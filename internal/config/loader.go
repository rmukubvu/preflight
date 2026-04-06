package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Filename is the config file name relative to the project directory.
const Filename = ".preflight.yaml"

const yamlHeader = `# preflight configuration
# WARNING: This file may contain sensitive API keys.
# Add .preflight.yaml to your .gitignore.
# Run 'preflight setup' to edit via browser UI.

`

// DefaultConfig returns a sensible zero-value configuration.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		LLM: LLMConfig{
			Provider: "auto",
			Bedrock: BedrockConfig{
				Region:  "us-east-1",
				ModelID: "amazon.nova-lite-v1:0",
			},
			Claude: ClaudeConfig{
				Model: "claude-haiku-4-5-20251001",
			},
			OpenAI: OpenAIConfig{
				Model: "gpt-4o-mini",
			},
			Ollama: OllamaConfig{
				BaseURL: "http://localhost:11434",
				Model:   "gemma3",
			},
		},
		Floci: FlociConfig{
			Image: "ghcr.io/floci/floci:latest",
			Port:  4566,
		},
	}
}

// Load reads the config file from dir. If the file does not exist,
// it returns DefaultConfig() with no error.
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, Filename)

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Save writes cfg to dir/.preflight.yaml atomically.
// It writes to a temporary file first, then renames to the final path,
// preventing a corrupt config on crash or concurrent writes.
func Save(dir string, cfg Config) error {
	path := filepath.Join(dir, Filename)
	tmp := path + ".tmp"

	data, err := marshalWithHeader(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

func marshalWithHeader(cfg Config) ([]byte, error) {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(yamlHeader)
	buf.Write(raw)
	return buf.Bytes(), nil
}
