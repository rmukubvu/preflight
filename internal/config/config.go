// Package config defines the preflight configuration schema and persistence.
// Config is the single source of truth for all user settings; .preflight.yaml
// is merely its serialised form.
package config

// Config is the root configuration object persisted to .preflight.yaml.
type Config struct {
	Version    int              `yaml:"version"`
	LLM        LLMConfig        `yaml:"llm"`
	Emulator   EmulatorConfig   `yaml:"emulator,omitempty"`
	Floci      FlociConfig      `yaml:"floci,omitempty"`
	Stack      StackConfig      `yaml:"stack"`
	Assertions AssertionsConfig `yaml:"assertions,omitempty"`
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

// EmulatorConfig holds settings for the local AWS emulator runtime.
type EmulatorConfig struct {
	// Type is one of: stratus, floci, custom.
	Type string `yaml:"type,omitempty"`

	// Endpoint points at an already-running emulator and disables local startup.
	Endpoint string `yaml:"endpoint,omitempty"`

	// Command is a space-delimited local command used to start the emulator.
	Command string `yaml:"command,omitempty"`

	// Image is the Docker image for Docker-backed emulators such as Floci.
	Image string `yaml:"image,omitempty"`

	// Port is the host port exposed by the emulator.
	Port int `yaml:"port,omitempty"`

	// DataDir is an optional durable data directory for local runtimes.
	DataDir string `yaml:"data_dir,omitempty"`

	// HealthPath overrides the default backend health endpoint when needed.
	HealthPath string `yaml:"health_path,omitempty"`
}

// FlociConfig holds legacy settings for the Floci local AWS emulator container.
// It is retained for backward compatibility with older .preflight.yaml files.
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

// AssertionsConfig holds user-configured assertion scenarios.
type AssertionsConfig struct {
	Behavioural BehaviouralConfig `yaml:"behavioural,omitempty"`
}

// BehaviouralConfig holds end-to-end behavioural checks that require
// stack-specific inputs such as request payloads and expected side effects.
type BehaviouralConfig struct {
	HTTP                []HTTPCheckConfig             `yaml:"http,omitempty"`
	SQSToLambdaToDynamo []SQSToLambdaToDynamoDBConfig `yaml:"sqs_to_lambda_to_dynamodb,omitempty"`
}

// HTTPCheckConfig describes a real HTTP call that should be made against a
// deployed API Gateway route.
type HTTPCheckConfig struct {
	// API references the API Gateway resource by CloudFormation logical ID or
	// deployed API ID.
	API string `yaml:"api"`

	// IntegrationFunction optionally names the Lambda integration to invoke as
	// a local fallback when the emulator cannot expose the API Gateway route.
	IntegrationFunction string `yaml:"integration_function,omitempty"`

	Method         string            `yaml:"method"`
	Path           string            `yaml:"path"`
	ExpectedStatus int               `yaml:"expected_status"`
	Body           string            `yaml:"body,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
}

// SQSToLambdaToDynamoDBConfig describes a synthetic event that should flow
// through the queue consumer and leave proof of processing in DynamoDB.
type SQSToLambdaToDynamoDBConfig struct {
	// Queue references the SQS resource by CloudFormation logical ID, queue
	// name, queue ARN, or queue URL.
	Queue string `yaml:"queue"`

	// Table references the DynamoDB table by CloudFormation logical ID or table
	// name.
	Table string `yaml:"table"`

	// ConsumerFunction optionally names the Lambda consumer to invoke as a
	// local fallback when the emulator does not deliver SQS events via ESM.
	ConsumerFunction string `yaml:"consumer_function,omitempty"`

	MessageBody string            `yaml:"message_body"`
	ExpectedKey map[string]string `yaml:"expected_key"`
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

// ValidEmulatorTypes is the set of accepted values for EmulatorConfig.Type.
var ValidEmulatorTypes = map[string]bool{
	"stratus": true,
	"floci":   true,
	"custom":  true,
}
