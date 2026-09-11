package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cn-maul/rosetta"

	"relationship/internal/config"
)

// Client speaks to chat providers through the unified rosetta SDK. Embeddings
// stay on a plain OpenAI-compatible /embeddings call: rosetta has no embed
// endpoint, and the Qwen3-Embedding-8B endpoint used here is one.
type Client struct {
	cfg    *config.LLMConfig
	llm    *rosetta.Client
	llmErr error
	// client backs the embeddings path only.
	client *http.Client
}

func NewClient(cfg *config.LLMConfig) *Client {
	opts := []rosetta.Option{
		rosetta.WithEndpoint(cfg.Endpoint),
		rosetta.WithAPIKey(cfg.APIKey),
		// Advice generation on a reasoning model can take well over a minute.
		rosetta.WithTimeout(5 * time.Minute),
		// The configured gateway answers max_tokens, not the newer
		// max_completion_tokens; pinning skips one failed probe per client.
		rosetta.WithMaxTokensField("max_tokens"),
		rosetta.WithLogger(slog.Default()),
	}
	if cfg.Protocol == "anthropic" {
		opts = append(opts, rosetta.WithProtocol(rosetta.ProtoAnthropic))
	} else {
		opts = append(opts, rosetta.WithProtocol(rosetta.ProtoOpenAIChat))
	}
	llm, err := rosetta.NewClient(opts...)
	return &Client{
		cfg:    cfg,
		llm:    llm,
		llmErr: err,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func System(content string) Message {
	return Message{Role: "system", Content: content}
}

func User(content string) Message {
	return Message{Role: "user", Content: content}
}

// ChatOptions tunes a single completion: JSON mode, sampling temperature
// (0 = provider default) and an explicit token budget (0 = config default).
type ChatOptions struct {
	JSONMode    bool
	Temperature float64
	MaxTokens   int
}

// Chat sends messages and returns the assistant text. Transient failures
// (transport errors, 408/429/5xx) are retried once: chat calls have no
// server-side side effects beyond the request itself.
func (c *Client) Chat(ctx context.Context, model string, messages []Message, opts ChatOptions) (string, error) {
	if c.llmErr != nil {
		return "", fmt.Errorf("llm client unavailable: %w", c.llmErr)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.llm.Chat(ctx, c.buildRequest(model, messages, opts))
		if err == nil {
			if resp.StopReason == rosetta.StopLength {
				return "", fmt.Errorf("回答被 max_tokens=%d 截断，可在配置中调大", c.resolveMaxTokens(opts))
			}
			if text := resp.Text(); text != "" {
				return text, nil
			}
			return "", fmt.Errorf("empty assistant reply")
		}
		lastErr = err
		if !retryableLLMErr(err) {
			break
		}
		sleepIfRetry(ctx, attempt, 1)
	}
	return "", fmt.Errorf("chat: %w", lastErr)
}

func (c *Client) buildRequest(model string, messages []Message, opts ChatOptions) *rosetta.ChatRequest {
	req := &rosetta.ChatRequest{
		Model:           model,
		MaxOutputTokens: c.resolveMaxTokens(opts),
	}
	for _, m := range messages {
		if m.Role == "system" {
			// rosetta merges the top-level System with system messages in
			// the list; hoisting keeps the Anthropic adapter happy.
			req.System += m.Content + "\n\n"
			continue
		}
		req.Messages = append(req.Messages, rosetta.Message{
			Role:   rosetta.Role(m.Role),
			Blocks: []rosetta.Block{{Type: rosetta.BlockText, Text: m.Content}},
		})
	}
	if opts.Temperature != 0 {
		req.Temperature = rosetta.Float(opts.Temperature)
	}
	if opts.JSONMode {
		if c.cfg.Protocol == "anthropic" {
			// Anthropic has no response_format; the system prompt is the
			// only cross-protocol lever for structured output.
			req.System += "Only output a JSON document. No prose, no code fences.\n\n"
		} else {
			req.Extra = map[string]any{"response_format": map[string]any{"type": "json_object"}}
		}
	}
	return req
}

func (c *Client) resolveMaxTokens(opts ChatOptions) int {
	if opts.MaxTokens > 0 {
		return opts.MaxTokens
	}
	return c.cfg.ResolvedMaxTokens()
}

// retryableLLMErr reports whether a rosetta error is worth one more attempt.
func retryableLLMErr(err error) bool {
	var apiErr *rosetta.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	var trErr *rosetta.TransportError
	return errors.As(err, &trErr)
}

type embedRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	endpoint := c.cfg.ResolvedEmbedEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("no embedding endpoint configured")
	}
	req := embedRequest{Model: c.cfg.EmbedModel, Input: text, Dimensions: c.cfg.EmbedDim}

	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.post(ctx, endpoint, c.cfg.ResolvedEmbedAPIKey(), "embeddings", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embeddings in response")
	}
	if c.cfg.EmbedDim > 0 && len(resp.Data[0].Embedding) != c.cfg.EmbedDim {
		return nil, fmt.Errorf("embedding dim mismatch: model returned %d, config expects %d", len(resp.Data[0].Embedding), c.cfg.EmbedDim)
	}
	return resp.Data[0].Embedding, nil
}

// post sends payload to {endpoint}/v1/embeddings. Endpoints are commonly
// written with or without the /v1 suffix, so join them without doubling it.
//
// Transport errors, 429s and 5xx are transient for embedding calls, so each
// is retried once after a short pause; 4xx responses are caller errors and
// pass through.
func (c *Client) post(ctx context.Context, endpoint, apiKey, path string, payload, out interface{}) error {
	url := apiURL(endpoint, path)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	var attemptErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.client.Do(httpReq)
		if err != nil {
			attemptErr = fmt.Errorf("%s request failed: %w", path, err)
			sleepIfRetry(ctx, attempt, 1)
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			attemptErr = fmt.Errorf("%s returned %s: %s", path, resp.Status, truncate(string(data), 400))
			sleepIfRetry(ctx, attempt, 1)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s returned %s: %s", path, resp.Status, truncate(string(data), 400))
		}
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unmarshal %s response: %w", path, err)
		}
		return nil
	}
	return attemptErr
}

func sleepIfRetry(ctx context.Context, attempt, seconds int) {
	if attempt == 0 {
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
		}
	}
}

func apiURL(endpoint, path string) string {
	base := strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/" + path
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
