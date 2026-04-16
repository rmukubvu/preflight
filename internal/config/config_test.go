package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
)

// ── Load ────────────────────────────────────────────────────────────────────

func TestLoad_FileNotExist_ReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLM.Provider != "auto" {
		t.Errorf("want provider auto, got %q", cfg.LLM.Provider)
	}
	if cfg.Emulator.Type != "stratus" {
		t.Errorf("want emulator type stratus, got %q", cfg.Emulator.Type)
	}
	if cfg.Emulator.Port != 4566 {
		t.Errorf("want emulator port 4566, got %d", cfg.Emulator.Port)
	}
	if cfg.Version != 1 {
		t.Errorf("want version 1, got %d", cfg.Version)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	// yaml.v3 rejects unclosed flow sequences.
	invalid := []byte("llm: {provider: [unclosed")
	if err := os.WriteFile(filepath.Join(dir, config.Filename), invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err == nil {
		t.Fatal("want error for invalid YAML, got nil")
	}
}

// ── Save + Load roundtrip ────────────────────────────────────────────────────

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()

	want := config.DefaultConfig()
	want.LLM.Provider = "claude"
	want.LLM.Claude.APIKey = "sk-ant-test"
	want.Stack.Type = "cdk"
	want.Stack.Dir = "./infra"
	want.Assertions.Behavioural.HTTP = []config.HTTPCheckConfig{
		{
			API:                 "JobsApi",
			IntegrationFunction: "JobsHandlerFunction",
			Method:              "POST",
			Path:                "/jobs",
			ExpectedStatus:      202,
			Body:                `{"id":"job-123"}`,
		},
	}

	if err := config.Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.LLM.Provider != want.LLM.Provider {
		t.Errorf("provider: want %q, got %q", want.LLM.Provider, got.LLM.Provider)
	}
	if got.LLM.Claude.APIKey != want.LLM.Claude.APIKey {
		t.Errorf("claude.api_key: want %q, got %q", want.LLM.Claude.APIKey, got.LLM.Claude.APIKey)
	}
	if got.Stack.Type != want.Stack.Type {
		t.Errorf("stack.type: want %q, got %q", want.Stack.Type, got.Stack.Type)
	}
	if len(got.Assertions.Behavioural.HTTP) != 1 {
		t.Fatalf("want 1 behavioural HTTP check, got %d", len(got.Assertions.Behavioural.HTTP))
	}
	if got.Assertions.Behavioural.HTTP[0].ExpectedStatus != 202 {
		t.Errorf("behavioural.http[0].expected_status: want 202, got %d", got.Assertions.Behavioural.HTTP[0].ExpectedStatus)
	}
	if got.Assertions.Behavioural.HTTP[0].IntegrationFunction != "JobsHandlerFunction" {
		t.Errorf("behavioural.http[0].integration_function: want %q, got %q", "JobsHandlerFunction", got.Assertions.Behavioural.HTTP[0].IntegrationFunction)
	}
}

func TestLoad_LegacyFlociConfig_MapsToEmulator(t *testing.T) {
	dir := t.TempDir()
	legacy := []byte("version: 1\nfloci:\n  image: hectorvent/floci:latest\n  port: 4666\n")
	if err := os.WriteFile(filepath.Join(dir, config.Filename), legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Emulator.Type != "floci" {
		t.Fatalf("want emulator type floci, got %q", cfg.Emulator.Type)
	}
	if cfg.Emulator.Port != 4666 {
		t.Fatalf("want emulator port 4666, got %d", cfg.Emulator.Port)
	}
	if cfg.Floci.Port != 4666 {
		t.Fatalf("want legacy floci port preserved in memory, got %d", cfg.Floci.Port)
	}
}

func TestSave_WritesHeaderComment(t *testing.T) {
	dir := t.TempDir()
	if err := config.Save(dir, config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, config.Filename))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}

	if !strings.Contains(string(data), "WARNING") {
		t.Error("saved file should contain WARNING comment about API keys")
	}
}

func TestSave_AtomicWrite_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := config.Save(dir, config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left on disk: %s", e.Name())
		}
	}
}

// ── Validate ─────────────────────────────────────────────────────────────────

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*config.Config)
		wantField string // empty means expect no errors
	}{
		{
			name:   "default config is valid",
			mutate: nil,
		},
		{
			name: "provider auto is valid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "auto"
			},
		},
		{
			name: "provider none is valid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "none"
			},
		},
		{
			name: "unknown provider is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "chatgpt"
			},
			wantField: "llm.provider",
		},
		{
			name: "claude with api key is valid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "claude"
				c.LLM.Claude.APIKey = "sk-ant-xxx"
			},
		},
		{
			name: "claude without api key is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "claude"
				c.LLM.Claude.APIKey = ""
			},
			wantField: "llm.claude.api_key",
		},
		{
			name: "openai without api key is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "openai"
				c.LLM.OpenAI.APIKey = ""
			},
			wantField: "llm.openai.api_key",
		},
		{
			name: "bedrock without region is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "bedrock"
				c.LLM.Bedrock.Region = ""
				c.LLM.Bedrock.ModelID = "amazon.nova-lite-v1:0"
			},
			wantField: "llm.bedrock.region",
		},
		{
			name: "bedrock without model_id is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "bedrock"
				c.LLM.Bedrock.Region = "us-east-1"
				c.LLM.Bedrock.ModelID = ""
			},
			wantField: "llm.bedrock.model_id",
		},
		{
			name: "ollama without base_url is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "ollama"
				c.LLM.Ollama.BaseURL = ""
				c.LLM.Ollama.Model = "llama3"
			},
			wantField: "llm.ollama.base_url",
		},
		{
			name: "ollama without model is invalid",
			mutate: func(c *config.Config) {
				c.LLM.Provider = "ollama"
				c.LLM.Ollama.BaseURL = "http://localhost:11434"
				c.LLM.Ollama.Model = ""
			},
			wantField: "llm.ollama.model",
		},
		{
			name: "pulumi stack type is valid",
			mutate: func(c *config.Config) {
				c.Stack.Type = "pulumi"
			},
		},
		{
			name: "default emulator type is valid",
			mutate: func(c *config.Config) {
				c.Emulator.Type = "stratus"
			},
		},
		{
			name: "unknown emulator type is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Type = "mystery"
			},
			wantField: "emulator.type",
		},
		{
			name: "cdk stack type is valid",
			mutate: func(c *config.Config) {
				c.Stack.Type = "cdk"
			},
		},
		{
			name: "emulator port below 1024 is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Port = 80
			},
			wantField: "emulator.port",
		},
		{
			name: "emulator port above 65535 is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Port = 70000
			},
			wantField: "emulator.port",
		},
		{
			name: "emulator port 0 is valid (use default)",
			mutate: func(c *config.Config) {
				c.Emulator.Port = 0
			},
		},
		{
			name: "stratus without command or endpoint is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Type = "stratus"
				c.Emulator.Command = ""
				c.Emulator.Endpoint = ""
			},
			wantField: "emulator.command",
		},
		{
			name: "floci without image or endpoint is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Type = "floci"
				c.Emulator.Image = ""
				c.Emulator.Endpoint = ""
			},
			wantField: "emulator.image",
		},
		{
			name: "custom without endpoint or command is invalid",
			mutate: func(c *config.Config) {
				c.Emulator.Type = "custom"
				c.Emulator.Command = ""
				c.Emulator.Endpoint = ""
			},
			wantField: "emulator.endpoint",
		},
		{
			name: "behavioural http check is valid",
			mutate: func(c *config.Config) {
				c.Assertions.Behavioural.HTTP = []config.HTTPCheckConfig{
					{
						API:            "JobsApi",
						Method:         "POST",
						Path:           "/jobs",
						ExpectedStatus: 202,
					},
				}
			},
		},
		{
			name: "behavioural http check requires api reference",
			mutate: func(c *config.Config) {
				c.Assertions.Behavioural.HTTP = []config.HTTPCheckConfig{
					{
						Method:         "GET",
						Path:           "/health",
						ExpectedStatus: 200,
					},
				}
			},
			wantField: "assertions.behavioural.http[0].api",
		},
		{
			name: "sqs behavioural check requires expected key",
			mutate: func(c *config.Config) {
				c.Assertions.Behavioural.SQSToLambdaToDynamo = []config.SQSToLambdaToDynamoDBConfig{
					{
						Queue:       "JobQueue",
						Table:       "JobsTable",
						MessageBody: `{"id":"job-123"}`,
					},
				}
			},
			wantField: "assertions.behavioural.sqs_to_lambda_to_dynamodb[0].expected_key",
		},
		{
			name: "sqs behavioural check is valid",
			mutate: func(c *config.Config) {
				c.Assertions.Behavioural.SQSToLambdaToDynamo = []config.SQSToLambdaToDynamoDBConfig{
					{
						Queue:       "JobQueue",
						Table:       "JobsTable",
						MessageBody: `{"id":"job-123"}`,
						ExpectedKey: map[string]string{"id": "job-123"},
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}

			errs := config.Validate(cfg)

			if tc.wantField == "" {
				if len(errs) != 0 {
					t.Errorf("want no errors, got %v", errs)
				}
				return
			}

			found := false
			for _, e := range errs {
				if e.Field == tc.wantField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("want error on field %q, got errors: %v", tc.wantField, errs)
			}
		})
	}
}
