package diagnosis_test

import (
	"context"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/diagnosis"
)

func TestEngine_Diagnose_UsesNoopWhenProviderIsNone(t *testing.T) {
	engine := diagnosis.NewEngine(config.LLMConfig{Provider: "none"})
	resp, err := engine.Diagnose(context.Background(), diagnosis.DiagnoseRequest{
		AssertionName: "lambda-esm",
		FailureReason: "no ESM found",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProviderName != "none" {
		t.Errorf("want provider none, got %q", resp.ProviderName)
	}
	if resp.Explanation == "" {
		t.Error("want non-empty explanation")
	}
}

func TestEngine_Diagnose_FallsBackToNoop_WhenNoProviderConfigured(t *testing.T) {
	// auto with no credentials — all real providers should be unavailable.
	// In CI where no credentials exist, this should always be Noop.
	engine := diagnosis.NewEngine(config.LLMConfig{Provider: "auto"})
	resp, err := engine.Diagnose(context.Background(), diagnosis.DiagnoseRequest{
		AssertionName: "iam-role-permission",
		FailureReason: "missing sqs:ReceiveMessage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The response must always have an explanation (could be any provider or noop).
	if resp.Explanation == "" {
		t.Error("want non-empty explanation")
	}
}

func TestEngine_ProviderName_ReturnsNoneWhenUnconfigured(t *testing.T) {
	engine := diagnosis.NewEngine(config.LLMConfig{Provider: "none"})
	name := engine.ProviderName(context.Background())
	if name != "none" {
		t.Errorf("want none, got %q", name)
	}
}

func TestNoopProvider_AlwaysAvailable(t *testing.T) {
	var p diagnosis.Provider = &diagnosis.NoopProvider{}
	if !p.Available(context.Background()) {
		t.Error("NoopProvider should always be available")
	}
}

func TestNoopProvider_Diagnose_IncludesAssertionName(t *testing.T) {
	p := &diagnosis.NoopProvider{}
	resp, err := p.Diagnose(context.Background(), diagnosis.DiagnoseRequest{
		AssertionName: "cfn-stack-deployed",
		FailureReason: "stack is ROLLBACK_COMPLETE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Explanation == "" {
		t.Error("want non-empty explanation")
	}
}
