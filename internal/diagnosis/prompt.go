package diagnosis

import (
	"fmt"
	"strings"
)

// systemPrompt is prepended to diagnosis requests where the API supports it.
const systemPrompt = `You are an AWS infrastructure expert specialising in CDK and Terraform.
When given an infrastructure assertion failure, explain WHY it causes a real-world problem
and provide an exact code fix. Be concise (3-5 sentences max).`

// buildPrompt constructs the user-facing prompt from an assertion failure.
func buildPrompt(req DiagnoseRequest) string {
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("## Assertion Failure\n\n"))
	sb.WriteString(fmt.Sprintf("**Assertion:** %s\n", req.AssertionName))
	sb.WriteString(fmt.Sprintf("**Resource type:** %s\n", req.ResourceType))
	sb.WriteString(fmt.Sprintf("**Failure:** %s\n", req.FailureReason))
	if req.ResourceDiff != "" {
		sb.WriteString(fmt.Sprintf("**Resource diff (expected vs actual):**\n```json\n%s\n```\n", req.ResourceDiff))
	}
	sb.WriteString("\nProvide:\n")
	sb.WriteString("1. A plain-English explanation of why this fails on real AWS.\n")
	sb.WriteString("2. An exact code fix (CDK TypeScript or Terraform HCL as appropriate).\n")
	sb.WriteString("3. The filename and function/resource to edit, if determinable.\n")
	return sb.String()
}

// parseStructuredResponse extracts the explanation and suggestion from a
// free-text LLM response. It does a best-effort split on common patterns.
func parseStructuredResponse(text, providerName string) DiagnoseResponse {
	text = strings.TrimSpace(text)
	if text == "" {
		return DiagnoseResponse{
			Explanation:  "No diagnosis available.",
			ProviderName: providerName,
		}
	}

	// Split on blank lines: first paragraph is explanation, rest is suggestion.
	parts := strings.SplitN(text, "\n\n", 2)
	explanation := strings.TrimSpace(parts[0])
	suggestion := ""
	if len(parts) > 1 {
		suggestion = strings.TrimSpace(parts[1])
	}

	return DiagnoseResponse{
		Explanation:  explanation,
		Suggestion:   suggestion,
		Confidence:   0.8, // providers don't expose confidence; use a reasonable default
		ProviderName: providerName,
	}
}
