package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"relationship/internal/config"
)

// The default configuration names a local endpoint and no credential. Building
// the chat client has to succeed in that case, otherwise every extraction fails
// with "api key is required" before a request is ever sent.
func TestNewClientAcceptsMissingAPIKey(t *testing.T) {
	client := NewClient(&config.LLMConfig{Endpoint: "http://127.0.0.1:1", Protocol: "openai"})
	if client.llmErr != nil {
		t.Fatalf("client without an api key did not build: %v", client.llmErr)
	}
	if client.llm == nil {
		t.Fatal("client without an api key built no chat client")
	}
}

const embedReply = `{"data":[{"index":0,"embedding":[0.1]}]}`

// Saving new endpoint settings rewrites the shared LLMConfig in place; the
// client must follow without a restart. This is the regression for
// "embeddings kept hitting the old endpoint after a settings save".
func TestClientFollowsEndpointChange(t *testing.T) {
	newServer := func(hits *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(embedReply))
		}))
	}
	var hitsA, hitsB atomic.Int32
	serverA := newServer(&hitsA)
	defer serverA.Close()
	serverB := newServer(&hitsB)
	defer serverB.Close()

	cfg := &config.LLMConfig{Endpoint: serverA.URL, Protocol: "openai", EmbedModel: "test"}
	client := NewClient(cfg)
	if _, err := client.Embed(context.Background(), "q"); err != nil {
		t.Fatalf("embed against the initial endpoint failed: %v", err)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 0 {
		t.Fatalf("first embed hit A=%d B=%d, want A=1 B=0", hitsA.Load(), hitsB.Load())
	}

	// What PUT /api/config does under the hood: rewrite the shared config.
	cfg.Endpoint = serverB.URL
	if _, err := client.Embed(context.Background(), "q"); err != nil {
		t.Fatalf("embed after an endpoint change failed: %v", err)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Fatalf("after the change embed hit A=%d B=%d, want A=1 B=1", hitsA.Load(), hitsB.Load())
	}
}

// A rerank provider that is not the embedding provider gets its own endpoint:
// /rerank calls must go there and never leak onto the main endpoint.
func TestClientRoutesRerankToItsOwnEndpoint(t *testing.T) {
	var embedHits, rerankOnMain, rerankHits atomic.Int32
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/rerank" {
			rerankOnMain.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		embedHits.Add(1)
		_, _ = w.Write([]byte(embedReply))
	}))
	defer embedSrv.Close()
	rerankSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rerankHits.Add(1)
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.9}]}`))
	}))
	defer rerankSrv.Close()

	cfg := &config.LLMConfig{
		Endpoint: embedSrv.URL, Protocol: "openai", EmbedModel: "e",
		RerankEndpoint: rerankSrv.URL, RerankModel: "r",
	}
	client := NewClient(cfg)
	order, err := client.Rerank(context.Background(), "q", []string{"d1"}, 1)
	if err != nil {
		t.Fatalf("rerank against the dedicated endpoint failed: %v", err)
	}
	if len(order) != 1 || order[0] != 0 {
		t.Fatalf("rerank order = %v, want [0]", order)
	}
	if rerankHits.Load() != 1 || rerankOnMain.Load() != 0 {
		t.Fatalf("rerank hits: dedicated=%d main=%d, want dedicated=1 main=0", rerankHits.Load(), rerankOnMain.Load())
	}
}
