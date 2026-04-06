package diagnosis

import (
	"context"
	"fmt"
)

// NoopProvider is the final fallback in the priority chain.
// It never requires configuration and always returns a structured error
// with a link to the docs rather than an AI-generated diagnosis.
type NoopProvider struct{}

var _ Provider = (*NoopProvider)(nil)

func (n *NoopProvider) Name() string { return "none" }

func (n *NoopProvider) Available(_ context.Context) bool { return true }

func (n *NoopProvider) Diagnose(_ context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	return DiagnoseResponse{
		Explanation:  fmt.Sprintf("Assertion %q failed: %s", req.AssertionName, req.FailureReason),
		Suggestion:   "Configure an LLM provider (`preflight setup`) for AI-powered diagnosis.",
		Confidence:   0,
		ProviderName: n.Name(),
	}, nil
}
