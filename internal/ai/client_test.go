package ai

import (
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
