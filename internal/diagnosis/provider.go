// Package diagnosis provides AI-powered analysis of assertion failures.
// Each LLM backend implements Provider; the Engine auto-selects the best
// available one using the priority chain defined in the design spec.
package diagnosis

import "context"

// Provider is satisfied by all LLM backends.
// Implementations: BedrockProvider, ClaudeProvider, OpenAIProvider,
// OllamaProvider, NoopProvider.
type Provider interface {
	// Name returns the provider identifier (e.g. "bedrock", "claude").
	Name() string

	// Available reports whether this provider can be used right now —
	// the API key is set, the endpoint is reachable, etc.
	Available(ctx context.Context) bool

	// Diagnose takes an assertion failure and returns a human-readable
	// explanation with a concrete code fix.
	Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error)
}

// DiagnoseRequest carries the failure context for a Provider to analyse.
type DiagnoseRequest struct {
	// AssertionName is the short identifier of the failed check,
	// e.g. "iam-role-has-permission".
	AssertionName string

	// FailureReason is the human-readable description of what failed.
	FailureReason string

	// ResourceType is the AWS resource type involved, e.g. "AWS::Lambda::Function".
	ResourceType string

	// ResourceDiff is a JSON diff between expected and actual resource config.
	ResourceDiff string
}

// DiagnoseResponse is the AI provider's analysis of a failure.
type DiagnoseResponse struct {
	// Explanation is a 2–3 sentence plain-English description of why this
	// failure will cause problems on real AWS.
	Explanation string

	// Suggestion is an exact code or config fix (file path + snippet when possible).
	Suggestion string

	// Confidence is the provider's self-assessed confidence: 0.0–1.0.
	Confidence float64

	// ProviderName is stamped by the engine so callers know which backend ran.
	ProviderName string
}
