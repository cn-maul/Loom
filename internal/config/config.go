package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// The json tags matter as much as the yaml ones: Echo binds request bodies through
// struct tags, so without them PUT /api/config would decode into a zero config.
type Config struct {
	Server      ServerConfig      `yaml:"server" json:"server"`
	Database    DatabaseConfig    `yaml:"database" json:"database"`
	LLM         LLMConfig         `yaml:"llm" json:"llm"`
	Backup      BackupConfig      `yaml:"backup" json:"backup"`
	Maintenance MaintenanceConfig `yaml:"maintenance" json:"maintenance"`
}

// MaintenanceConfig drives the periodic data-hygiene pass. AuditRetentionDays
// of 0 keeps audit rows forever — the safe default for security evidence.
type MaintenanceConfig struct {
	AuditRetentionDays int `yaml:"audit_retention_days" json:"audit_retention_days"`
}

type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Host string `yaml:"host" json:"host"`
	// AuthToken optionally guards every /api route. It is never returned to
	// the frontend; the settings page only learns whether it is set.
	AuthToken string `yaml:"auth_token" json:"-"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

// BackupConfig drives the scheduled snapshots. Dir is resolved relative to
// the working directory when relative; Keep is the retention count.
type BackupConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Dir           string `yaml:"dir" json:"dir"`
	IntervalHours int    `yaml:"interval_hours" json:"interval_hours"`
	Keep          int    `yaml:"keep" json:"keep"`
	// Passphrase encrypts snapshots with AES-256-GCM. It never leaves the
	// process: lose it and the encrypted backups are unreadable.
	Passphrase string `yaml:"passphrase" json:"-"`
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
	// AsyncExtract runs the record extraction pipeline in a background queue
	// instead of blocking POST /api/events. Defaults to true; set false to
	// restore the old synchronous behaviour (useful for tests and debugging).
	AsyncExtract bool `yaml:"async_extract" json:"async_extract"`
	// AllowRemote is the data-boundary kill switch: when false, every model
	// call to a non-loopback endpoint fails closed. Extraction always sends
	// the raw record text to the configured endpoint — that is how it works —
	// so pointing the endpoint at a cloud service means the text leaves the
	// machine. Defaults to true in config.Load.
	AllowRemote bool `yaml:"allow_remote" json:"allow_remote"`
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
		Backup: BackupConfig{
			Enabled:       true,
			Dir:           "backups",
			IntervalHours: 24,
			Keep:          14,
		},
		LLM: LLMConfig{
			Endpoint:     "http://localhost:11434",
			Protocol:     "openai",
			ExtractModel: "qwen3:8b",
			AdviceModel:  "deepseek-chat",
			EmbedModel:   "nomic-embed-text",
			EmbedDim:     768,
			MaxTokens:    8000,
			AsyncExtract: true,
			AllowRemote:  true,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
