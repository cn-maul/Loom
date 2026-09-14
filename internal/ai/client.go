package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cn-maul/rosetta"

	"relationship/internal/config"
)

// Client speaks to chat, embeddings and model listings through the unified
// rosetta SDK (v0.4.0 added the embeddings path). It holds two SDK clients:
// the main one for chat and embeddings, and a dedicated rerank client whose
// aux override points at the resolved rerank endpoint — rerank may live on a
// different provider than embeddings (Cohere-format POST /rerank), so it
// cannot simply share the embedding endpoint override.
//
// The settings page rewrites the shared LLMConfig in place, and the SDK bakes
// endpoint, protocol and keys into its client at construction — so both
// clients are rebuilt lazily whenever those settings change (see current).
type Client struct {
	cfg *config.LLMConfig

	mu        sync.Mutex
	llm       *rosetta.Client // chat + embeddings
	rerankLLM *rosetta.Client // rerank only
	llmErr    error
	built     clientSettings
}

// clientSettings is the subset of LLMConfig the rosetta clients capture at
// construction. Everything else (model ids, max tokens) is read per call.
type clientSettings struct {
	endpoint       string
	protocol       string
	apiKey         string
	embedEndpoint  string
	embedAPIKey    string
	rerankEndpoint string
	rerankAPIKey   string
}

func clientSettingsFrom(cfg *config.LLMConfig) clientSettings {
	// A local endpoint (Ollama, LM Studio) needs no credential, but the chat
	// library refuses to build a client with an empty one. A placeholder keeps
	// the out-of-the-box setup working instead of failing every call before it
	// is ever sent.
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = "local"
	}
	return clientSettings{
		endpoint:       cfg.Endpoint,
		protocol:       cfg.Protocol,
		apiKey:         apiKey,
		embedEndpoint:  cfg.EmbedEndpoint,
		embedAPIKey:    cfg.EmbedAPIKey,
		rerankEndpoint: cfg.RerankEndpoint,
		rerankAPIKey:   cfg.RerankAPIKey,
	}
}

func NewClient(cfg *config.LLMConfig) *Client {
	c := &Client{cfg: cfg}
	c.current()
	return c
}

// current returns the main and rerank rosetta clients for the live config,
// rebuilding them when endpoint/protocol/key settings changed since the last
// build. Model names and token budgets travel per request, so saving config
// applies immediately.
func (c *Client) current() (llm *rosetta.Client, rerank *rosetta.Client, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	snap := clientSettingsFrom(c.cfg)
	if c.llm != nil && snap == c.built {
		return c.llm, c.rerankLLM, c.llmErr
	}

	baseOpts := func() []rosetta.Option {
		opts := []rosetta.Option{
			rosetta.WithEndpoint(snap.endpoint),
			rosetta.WithAPIKey(snap.apiKey),
			// Advice generation on a reasoning model can take well over a minute.
			rosetta.WithTimeout(5 * time.Minute),
			// The configured gateway answers max_tokens, not the newer
			// max_completion_tokens; pinning skips one failed probe per client.
			rosetta.WithMaxTokensField("max_tokens"),
			// The SDK logs request payloads at debug level; those payloads carry
			// relationship text. Only warnings and errors may reach the log.
			rosetta.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))),
		}
		if snap.protocol == "anthropic" {
			opts = append(opts, rosetta.WithProtocol(rosetta.ProtoAnthropic))
		} else {
			opts = append(opts, rosetta.WithProtocol(rosetta.ProtoOpenAIChat))
		}
		return opts
	}

	// Main client: chat and embeddings. A dedicated embedding service (endpoint
	// or key differing from chat) rides rosetta's aux override; when unset,
	// rosetta falls back to the main endpoint and key — the same resolution
	// ResolvedEmbedEndpoint does.
	mainOpts := baseOpts()
	if snap.embedEndpoint != "" {
		mainOpts = append(mainOpts, rosetta.WithEmbeddingEndpoint(snap.embedEndpoint))
	}
	if snap.embedAPIKey != "" {
		mainOpts = append(mainOpts, rosetta.WithEmbeddingAPIKey(snap.embedAPIKey))
	}

	// Rerank client: a dedicated SDK client whose aux override points at the
	// resolved rerank endpoint (rerank → embedding → chat), so rerank can live
	// on a different provider than embeddings. The resolved endpoint and key
	// are always pinned, never inherited.
	rerankEndpoint := snap.rerankEndpoint
	if rerankEndpoint == "" {
		rerankEndpoint = snap.embedEndpoint
	}
	if rerankEndpoint == "" {
		rerankEndpoint = snap.endpoint
	}
	rerankKey := snap.rerankAPIKey
	if rerankKey == "" {
		rerankKey = snap.embedAPIKey
	}
	if rerankKey == "" {
		rerankKey = snap.apiKey
	}
	rerankOpts := append(baseOpts(),
		rosetta.WithAPIKey(rerankKey),
		rosetta.WithEmbeddingEndpoint(rerankEndpoint),
	)
	if snap.rerankAPIKey != "" {
		rerankOpts = append(rerankOpts, rosetta.WithEmbeddingAPIKey(snap.rerankAPIKey))
	}

	llm, err = rosetta.NewClient(mainOpts...)
	if err != nil {
		c.llm, c.rerankLLM, c.llmErr, c.built = nil, nil, err, snap
		return nil, nil, err
	}
	rerank, err = rosetta.NewClient(rerankOpts...)
	if err != nil {
		c.llm, c.rerankLLM, c.llmErr, c.built = llm, nil, err, snap
		return llm, nil, err
	}
	c.llm, c.rerankLLM, c.llmErr, c.built = llm, rerank, nil, snap
	return llm, rerank, nil
}

// ListModels fetches the endpoint's model catalog (rosetta probes the
// OpenAI-compatible /models list) and returns bare model ids. current() builds
// against the *current* config, so the settings page can change endpoint/key,
// save, and immediately list what that new endpoint serves.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	if err := c.checkBoundary(c.cfg.Endpoint); err != nil {
		return nil, err
	}
	llm, _, llmErr := c.current()
	if llmErr != nil {
		return nil, fmt.Errorf("llm client unavailable: %w", llmErr)
	}
	infos, err := llm.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.ID != "" {
			ids = append(ids, info.ID)
		}
	}
	return ids, nil
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
	if err := c.checkBoundary(c.cfg.Endpoint); err != nil {
		return "", err
	}
	llm, _, llmErr := c.current()
	if llmErr != nil {
		return "", fmt.Errorf("llm client unavailable: %w", llmErr)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := llm.Chat(ctx, c.buildRequest(model, messages, opts))
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

// Embed computes one embedding vector. The call rides rosetta's aux path
// (OpenAI-compatible POST /embeddings against the embedding override endpoint,
// else the main one); rosetta validates the reply shape, including the
// dimensionality against the requested Dimensions, and retries transient
// failures internally — no outer retry here.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	endpoint := c.cfg.ResolvedEmbedEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("no embedding endpoint configured")
	}
	if err := c.checkBoundary(endpoint); err != nil {
		return nil, err
	}
	llm, _, llmErr := c.current()
	if llmErr != nil {
		return nil, fmt.Errorf("llm client unavailable: %w", llmErr)
	}

	req := &rosetta.EmbeddingRequest{
		Model:      c.cfg.EmbedModel,
		Input:      []string{text},
		Dimensions: c.cfg.EmbedDim,
	}
	resp, err := llm.Embed(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	return resp.Data[0].Embedding, nil
}

// Rerank scores documents against the query, most relevant first, and returns
// their original indexes in that order. The call rides rosetta's aux path
// (Cohere-format POST /rerank against the embedding endpoint override, else
// the main one). It sends the document texts — record content — to the
// endpoint, so the allow_remote boundary applies here exactly as to Embed.
// rosetta retries transient failures internally; no outer retry here.
func (c *Client) Rerank(ctx context.Context, query string, documents []string, topN int) ([]int, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	if err := c.checkBoundary(c.cfg.ResolvedRerankEndpoint()); err != nil {
		return nil, err
	}
	_, rerankLLM, llmErr := c.current()
	if llmErr != nil {
		return nil, fmt.Errorf("llm client unavailable: %w", llmErr)
	}

	req := &rosetta.RerankRequest{
		Model:     c.cfg.RerankModel,
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}
	resp, err := rerankLLM.Rerank(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	order := make([]int, 0, len(resp.Results))
	for _, r := range resp.Results {
		order = append(order, r.Index)
	}
	return order, nil
}

func sleepIfRetry(ctx context.Context, attempt, seconds int) {
	if attempt == 0 {
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
		}
	}
}

// checkBoundary enforces llm.allow_remote: the kill switch that keeps record
// text on this machine. Extraction embeds the raw record in every prompt, so
// a non-loopback endpoint IS the data leaving the machine — when the switch
// is off, that call fails closed instead of silently leaking.
func (c *Client) checkBoundary(endpoint string) error {
	if c.cfg.AllowRemote || IsLoopbackEndpoint(endpoint) {
		return nil
	}
	return fmt.Errorf("数据边界：llm.allow_remote 为 false，已拒绝把记录内容发送到外部端点 %s；"+
		"如确认要发送，请在 config.yaml 中将 allow_remote 改为 true，或改用本地模型", endpoint)
}

// IsLoopbackEndpoint reports whether the endpoint host is this machine.
func IsLoopbackEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
