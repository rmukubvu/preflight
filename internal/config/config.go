// Package config defines the preflight configuration schema and persistence.
// Config is the single source of truth for all user settings; .preflight.yaml
// is merely its serialised form.
package config

// Config is the root configuration object persisted to .preflight.yaml.
type Config struct {
	Version int         `yaml:"version"`
	LLM     LLMConfig   `yaml:"llm"`
	Floci   FlociConfig `yaml:"floci"`
	Stack   StackConfig `yaml:"stack"`
}

// LLMConfig holds AI diagnosis provider settings.
type LLMConfig struct {
	// Provider is one of: auto, bedrock, claude, openai, ollama, none.
	Provider string        `yaml:"provider"`
	Bedrock  BedrockConfig `yaml:"bedrock,omitempty"`
	Claude   ClaudeConfig  `yaml:"claude,omitempty"`
	OpenAI   OpenAIConfig  `yaml:"openai,omitempty"`
	Ollama   OllamaConfig  `yaml:"ollama,omitempty"`
}

// BedrockConfig holds AWS Bedrock provider settings.
type BedrockConfig struct {
	Region  string `yaml:"region"`
	ModelID string `yaml:"model_id"`
}

// ClaudeConfig holds Anthropic Claude provider settings.
type ClaudeConfig struct {
	// APIKey is the Anthropic API key. Omitted from YAML when blank.
	APIKey string `yaml:"api_key,omitempty"`
	Model  string `yaml:"model,omitempty"`
}

// OpenAIConfig holds OpenAI (or Azure OpenAI) provider settings.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key. Omitted from YAML when blank.
	APIKey  string `yaml:"api_key,omitempty"`
	Model   string `yaml:"model,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"` // for Azure OpenAI endpoint override
}

// OllamaConfig holds Ollama local inference settings.
type OllamaConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

// FlociConfig holds settings for the local AWS emulator container.
type FlociConfig struct {
	Image   string `yaml:"image"`
	Port    int    `yaml:"port"`
	DataDir string `yaml:"data_dir,omitempty"`
}

// StackConfig identifies the IaC project being validated.
type StackConfig struct {
	// Type is "cdk" or "terraform".
	Type   string `yaml:"type,omitempty"`
	Dir    string `yaml:"dir,omitempty"`
	CDKApp string `yaml:"cdk_app,omitempty"` // e.g. "npx cdk"
}

// ValidProviders is the set of accepted values for LLMConfig.Provider.
var ValidProviders = map[string]bool{
	"auto":    true,
	"bedrock": true,
	"claude":  true,
	"openai":  true,
	"ollama":  true,
	"none":    true,
}

// ValidStackTypes is the set of accepted values for StackConfig.Type.
var ValidStackTypes = map[string]bool{
	"cdk":       true,
	"terraform": true,
}
