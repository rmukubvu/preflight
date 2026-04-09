package lint

import (
	"context"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
)

func TestDiagnoseFindingFallsBackToRulebook(t *testing.T) {
	finding := Finding{
		Severity:       SeverityWarning,
		Category:       CategoryReliability,
		RuleID:         "sqs-redrive-policy",
		TemplateName:   "App.template.json",
		ResourceID:     "Queue",
		Message:        "SQS queue has no dead-letter queue configured.",
		Recommendation: "Add RedrivePolicy so poison messages do not block consumers indefinitely.",
	}

	got := DiagnoseFinding(context.Background(), config.LLMConfig{Provider: "none"}, finding)
	if !got.Deterministic {
		t.Fatalf("expected deterministic diagnosis")
	}
	if got.Diagnosis.ProviderName != "rulebook" {
		t.Fatalf("want rulebook provider, got %q", got.Diagnosis.ProviderName)
	}
	if got.Diagnosis.Explanation == "" || got.Diagnosis.Suggestion == "" {
		t.Fatalf("expected populated diagnosis, got %#v", got.Diagnosis)
	}
}
