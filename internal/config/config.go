package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// The json tags matter as much as the yaml ones: Echo binds request bodies through
// struct tags, so without them PUT /api/config would decode into a zero config.
type Config struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	LLM      LLMConfig      `yaml:"llm" json:"llm"`
}

type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Host string `yaml:"host" json:"host"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

type LLMConfig struct {
	Endpoint     string `yaml:"endpoint" json:"endpoint"`
	Protocol     string `yaml:"protocol" json:"protocol"`
	APIKey       string `yaml:"api_key" json:"api_key"`
	ExtractModel string `yaml:"extract_model" json:"extract_model"`
	AdviceModel  string `yaml:"advice_model" json:"advice_model"`
	// EmbedEndpoint and EmbedAPIKey are optional; they fall back to Endpoint and
	// APIKey so a single-provider setup still works.
	EmbedEndpoint string `yaml:"embed_endpoint" json:"embed_endpoint"`
	EmbedAPIKey   string `yaml:"embed_api_key" json:"embed_api_key"`
	EmbedModel    string `yaml:"embed_model" json:"embed_model"`
	EmbedDim      int    `yaml:"embed_dim" json:"embed_dim"`
	// MaxTokens bounds one completion. Leaving it unset lets providers apply their
	// own small default, which silently truncates long structured answers.
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
}

// ResolvedMaxTokens is the completion budget sent to chat models.
func (c *LLMConfig) ResolvedMaxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return 8000
}

func (c *LLMConfig) ResolvedEmbedEndpoint() string {
	if c.EmbedEndpoint != "" {
		return c.EmbedEndpoint
	}
	return c.Endpoint
}

func (c *LLMConfig) ResolvedEmbedAPIKey() string {
	if c.EmbedAPIKey != "" {
		return c.EmbedAPIKey
	}
	return c.APIKey
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "localhost",
		},
		Database: DatabaseConfig{
			Path: "data/relationship.db",
		},
		LLM: LLMConfig{
			Endpoint:     "http://localhost:11434",
			Protocol:     "openai",
			ExtractModel: "qwen3:8b",
			AdviceModel:  "deepseek-chat",
			EmbedModel:   "nomic-embed-text",
			EmbedDim:     768,
			MaxTokens:    8000,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
