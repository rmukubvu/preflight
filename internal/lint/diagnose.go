package lint

import (
	"context"
	"fmt"
	"strings"

	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/diagnosis"
)

// DiagnosedFinding joins a lint finding with an explanation.
type DiagnosedFinding struct {
	Finding       Finding
	Diagnosis     diagnosis.DiagnoseResponse
	Deterministic bool
}

// DiagnoseFinding explains a lint finding. It falls back to deterministic
// rulebook guidance when no AI provider is configured or available.
func DiagnoseFinding(ctx context.Context, llm config.LLMConfig, finding Finding) DiagnosedFinding {
	baseline := deterministicDiagnosis(finding)
	engine := diagnosis.NewEngine(llm)
	if engine.ProviderName(ctx) == "none" {
		return DiagnosedFinding{
			Finding:       finding,
			Diagnosis:     baseline,
			Deterministic: true,
		}
	}

	resp, err := engine.Diagnose(ctx, diagnosis.DiagnoseRequest{
		AssertionName: "lint/" + finding.RuleID,
		FailureReason: finding.Message,
		ResourceType:  finding.ResourceID,
		ResourceDiff:  finding.Recommendation,
	})
	if err != nil {
		return DiagnosedFinding{
			Finding:       finding,
			Diagnosis:     baseline,
			Deterministic: true,
		}
	}

	if strings.TrimSpace(resp.Explanation) == "" {
		resp.Explanation = baseline.Explanation
	}
	if strings.TrimSpace(resp.Suggestion) == "" {
		resp.Suggestion = baseline.Suggestion
	}

	return DiagnosedFinding{
		Finding:       finding,
		Diagnosis:     resp,
		Deterministic: false,
	}
}

func deterministicDiagnosis(f Finding) diagnosis.DiagnoseResponse {
	impact := map[Category]string{
		CategorySecurity:      "This weakens the safety boundary around the workload and can turn a local success into a production exposure.",
		CategoryReliability:   "This leaves the stack without a clear recovery path when downstream systems fail or messages go bad.",
		CategoryObservability: "This reduces your ability to understand failures quickly once the stack is under real traffic.",
		CategoryScalability:   "This leaves scaling behavior implicit, which usually shows up as throttling, backlog growth, or unstable latency.",
	}[f.Category]

	explanation := fmt.Sprintf("%s %s", impact, strings.TrimSpace(f.Message))
	if impact == "" {
		explanation = f.Message
	}

	suggestion := strings.TrimSpace(f.Recommendation)
	if suggestion == "" {
		suggestion = "Make the relevant infrastructure setting explicit before promoting the stack."
	}

	return diagnosis.DiagnoseResponse{
		Explanation:  explanation,
		Suggestion:   suggestion,
		Confidence:   0.66,
		ProviderName: "rulebook",
	}
}
