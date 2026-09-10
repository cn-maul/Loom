package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	LLM      LLMConfig      `yaml:"llm"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LLMConfig struct {
	Endpoint     string `yaml:"endpoint"`
	Protocol     string `yaml:"protocol"`
	APIKey       string `yaml:"api_key"`
	ExtractModel string `yaml:"extract_model"`
	AdviceModel  string `yaml:"advice_model"`
	EmbedModel   string `yaml:"embed_model"`
	EmbedDim     int    `yaml:"embed_dim"`
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
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
