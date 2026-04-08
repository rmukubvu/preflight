package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
		Emulator: EmulatorConfig{
			Type:    "stratus",
			Command: "stratus",
			Port:    4566,
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

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	return Normalize(cfg), nil
}

// Save writes cfg to dir/.preflight.yaml atomically.
// It writes to a temporary file first, then renames to the final path,
// preventing a corrupt config on crash or concurrent writes.
func Save(dir string, cfg Config) error {
	path := filepath.Join(dir, Filename)
	tmp := path + ".tmp"

	normalized := Normalize(cfg)
	normalized.Floci = FlociConfig{}

	data, err := marshalWithHeader(normalized)
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

// Normalize applies defaults and maps legacy config into the emulator model.
func Normalize(cfg Config) Config {
	defaults := DefaultConfig()

	if cfg.Version == 0 {
		cfg.Version = defaults.Version
	}

	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = defaults.LLM.Provider
	}
	if cfg.LLM.Bedrock.Region == "" {
		cfg.LLM.Bedrock.Region = defaults.LLM.Bedrock.Region
	}
	if cfg.LLM.Bedrock.ModelID == "" {
		cfg.LLM.Bedrock.ModelID = defaults.LLM.Bedrock.ModelID
	}
	if cfg.LLM.Claude.Model == "" {
		cfg.LLM.Claude.Model = defaults.LLM.Claude.Model
	}
	if cfg.LLM.OpenAI.Model == "" {
		cfg.LLM.OpenAI.Model = defaults.LLM.OpenAI.Model
	}
	if cfg.LLM.Ollama.BaseURL == "" {
		cfg.LLM.Ollama.BaseURL = defaults.LLM.Ollama.BaseURL
	}
	if cfg.LLM.Ollama.Model == "" {
		cfg.LLM.Ollama.Model = defaults.LLM.Ollama.Model
	}

	if !hasExplicitEmulatorConfig(cfg.Emulator) && hasLegacyFlociConfig(cfg.Floci) {
		cfg.Emulator = EmulatorConfig{
			Type:    "floci",
			Image:   cfg.Floci.Image,
			Port:    cfg.Floci.Port,
			DataDir: cfg.Floci.DataDir,
		}
	}

	if cfg.Emulator.Type == "" {
		cfg.Emulator.Type = defaults.Emulator.Type
	}
	if cfg.Emulator.Port == 0 {
		cfg.Emulator.Port = defaults.Emulator.Port
	}

	switch cfg.Emulator.Type {
	case "stratus":
		if cfg.Emulator.Command == "" && cfg.Emulator.Endpoint == "" {
			cfg.Emulator.Command = defaults.Emulator.Command
		}
	case "floci":
		if cfg.Emulator.Image == "" {
			cfg.Emulator.Image = "hectorvent/floci:latest"
		}
	}

	if raw := os.Getenv("PREFLIGHT_EMULATOR_TYPE"); raw != "" {
		cfg.Emulator.Type = raw
	}
	if raw := os.Getenv("PREFLIGHT_EMULATOR_ENDPOINT"); raw != "" {
		cfg.Emulator.Endpoint = raw
		cfg.Emulator.Command = ""
	}
	if raw := os.Getenv("PREFLIGHT_EMULATOR_COMMAND"); raw != "" {
		cfg.Emulator.Command = raw
	}
	if raw := os.Getenv("PREFLIGHT_EMULATOR_PORT"); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			cfg.Emulator.Port = port
		}
	}
	if raw := os.Getenv("PREFLIGHT_EMULATOR_DATA_DIR"); raw != "" {
		cfg.Emulator.DataDir = raw
	}
	if raw := os.Getenv("PREFLIGHT_EMULATOR_HEALTH_PATH"); raw != "" {
		cfg.Emulator.HealthPath = raw
	}

	return cfg
}

func hasExplicitEmulatorConfig(cfg EmulatorConfig) bool {
	return cfg.Type != "" ||
		cfg.Endpoint != "" ||
		cfg.Command != "" ||
		cfg.Image != "" ||
		cfg.Port != 0 ||
		cfg.DataDir != "" ||
		cfg.HealthPath != ""
}

func hasLegacyFlociConfig(cfg FlociConfig) bool {
	return cfg.Image != "" || cfg.Port != 0 || cfg.DataDir != ""
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
