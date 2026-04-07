package config

import (
	"fmt"
	"strings"
)

// ValidationError describes a single configuration field violation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks cfg for logical consistency and returns all violations.
// Returns nil when cfg is valid. Callers should range over the slice;
// a nil slice means valid.
func Validate(cfg Config) []ValidationError {
	var errs []ValidationError

	add := func(field, msg string) {
		errs = append(errs, ValidationError{Field: field, Message: msg})
	}

	// Provider must be a known value.
	if !ValidProviders[cfg.LLM.Provider] {
		add("llm.provider", fmt.Sprintf("must be one of: auto, bedrock, claude, openai, ollama, none; got %q", cfg.LLM.Provider))
	}

	// Provider-specific required fields.
	switch cfg.LLM.Provider {
	case "claude":
		if cfg.LLM.Claude.APIKey == "" {
			add("llm.claude.api_key", "required when provider is claude")
		}
	case "openai":
		if cfg.LLM.OpenAI.APIKey == "" {
			add("llm.openai.api_key", "required when provider is openai")
		}
	case "bedrock":
		if cfg.LLM.Bedrock.Region == "" {
			add("llm.bedrock.region", "required when provider is bedrock")
		}
		if cfg.LLM.Bedrock.ModelID == "" {
			add("llm.bedrock.model_id", "required when provider is bedrock")
		}
	case "ollama":
		if cfg.LLM.Ollama.BaseURL == "" {
			add("llm.ollama.base_url", "required when provider is ollama")
		}
		if cfg.LLM.Ollama.Model == "" {
			add("llm.ollama.model", "required when provider is ollama")
		}
	}

	// Stack type must be a known value when set.
	if cfg.Stack.Type != "" && !ValidStackTypes[cfg.Stack.Type] {
		add("stack.type", fmt.Sprintf("must be cdk or terraform; got %q", cfg.Stack.Type))
	}

	// Floci port must be in the unprivileged range when set.
	if cfg.Floci.Port != 0 && (cfg.Floci.Port < 1024 || cfg.Floci.Port > 65535) {
		add("floci.port", fmt.Sprintf("must be between 1024 and 65535; got %d", cfg.Floci.Port))
	}

	for i, check := range cfg.Assertions.Behavioural.HTTP {
		prefix := fmt.Sprintf("assertions.behavioural.http[%d]", i)
		if strings.TrimSpace(check.API) == "" {
			add(prefix+".api", "required")
		}
		if strings.TrimSpace(check.Method) == "" {
			add(prefix+".method", "required")
		}
		if strings.TrimSpace(check.Path) == "" {
			add(prefix+".path", "required")
		}
		if check.ExpectedStatus <= 0 {
			add(prefix+".expected_status", "must be greater than 0")
		}
	}

	for i, check := range cfg.Assertions.Behavioural.SQSToLambdaToDynamo {
		prefix := fmt.Sprintf("assertions.behavioural.sqs_to_lambda_to_dynamodb[%d]", i)
		if strings.TrimSpace(check.Queue) == "" {
			add(prefix+".queue", "required")
		}
		if strings.TrimSpace(check.Table) == "" {
			add(prefix+".table", "required")
		}
		if strings.TrimSpace(check.MessageBody) == "" {
			add(prefix+".message_body", "required")
		}
		if len(check.ExpectedKey) == 0 {
			add(prefix+".expected_key", "must contain at least one key attribute")
		}
	}

	return errs
}
