package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/stack"
)

func TestNewReportIncludesDiagnosesAndRemediations(t *testing.T) {
	t.Parallel()

	result := Result{
		Findings: []Finding{
			{
				Severity:       SeverityWarning,
				Category:       CategorySecurity,
				RuleID:         "api-auth",
				TemplateName:   "fixture.template.json",
				ResourceID:     "JobsRoute",
				Message:        "API route does not configure an authorizer or non-public authorization type.",
				Recommendation: "Set AuthorizationType explicitly and attach an authorizer.",
			},
		},
	}
	diagnoses := diagnoseFindings(context.Background(), config.LLMConfig{Provider: "none"}, result)

	report := NewReport(result, diagnoses, Options{StackType: stack.TypeCDK, StackDir: "/tmp/app", StackName: "SmokeFixtureStack"})
	if report.Summary.TotalFindings != 1 {
		t.Fatalf("expected 1 total finding, got %d", report.Summary.TotalFindings)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 report finding, got %d", len(report.Findings))
	}
	if report.Findings[0].Diagnosis.Provider != "rulebook" {
		t.Fatalf("expected rulebook diagnosis provider, got %q", report.Findings[0].Diagnosis.Provider)
	}
	if len(report.Remediations) != 1 {
		t.Fatalf("expected 1 remediation, got %d", len(report.Remediations))
	}
	if report.Remediations[0].RuleID != "api-auth" {
		t.Fatalf("expected remediation for api-auth, got %q", report.Remediations[0].RuleID)
	}
}

func TestWriteJSONProducesMachineReadableFindings(t *testing.T) {
	t.Parallel()

	report := Report{
		Version: 1,
		Summary: ReportSummary{TotalFindings: 1},
		Findings: []ReportFinding{
			{
				Severity:     SeverityError,
				Category:     CategoryReliability,
				RuleID:       "sqs-redrive-policy",
				TemplateName: "fixture.template.json",
				ResourceID:   "JobsQueue",
				Message:      "Queue does not configure a dead-letter queue.",
				Diagnosis: ReportDiagnosis{
					Provider:    "rulebook",
					Explanation: "This leaves poison messages cycling forever.",
					Suggestion:  "Add RedrivePolicy.",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, report); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output was not valid JSON: %v", err)
	}
	findings, ok := decoded["findings"].([]any)
	if !ok || len(findings) != 1 {
		t.Fatalf("expected 1 machine-readable finding, got %#v", decoded["findings"])
	}
}

func TestWriteMarkdownIncludesRecommendedActions(t *testing.T) {
	t.Parallel()

	report := Report{
		Passed:    false,
		StackType: stack.TypeTerraform,
		Findings: []ReportFinding{
			{
				Severity:     SeverityWarning,
				Category:     CategoryObservability,
				RuleID:       "lambda-log-retention",
				TemplateName: "main.tf",
				ResourceID:   "aws_lambda_function.worker",
				Message:      "Lambda function does not configure explicit log retention.",
				Diagnosis: ReportDiagnosis{
					Provider:    "rulebook",
					Explanation: "This makes noisy workloads harder to operate.",
					Suggestion:  "Add aws_cloudwatch_log_group with retention_in_days.",
				},
			},
		},
		Remediations: []RemediationSummary{
			{
				RuleID:            "lambda-log-retention",
				Severity:          SeverityWarning,
				Category:          CategoryObservability,
				Title:             "Lambda function does not configure explicit log retention",
				WhyItMatters:      "This makes noisy workloads harder to operate.",
				SuggestedFix:      "Add aws_cloudwatch_log_group with retention_in_days.",
				AffectedResources: []string{"main.tf/aws_lambda_function.worker"},
			},
		},
		Summary: ReportSummary{
			Scores: []CategoryScore{
				{Category: CategoryObservability, Score: 88, Warnings: 1},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, report); err != nil {
		t.Fatalf("WriteMarkdown returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "### Recommended next actions") {
		t.Fatalf("expected markdown output to contain recommended actions section, got %q", output)
	}
	if !strings.Contains(output, "lambda-log-retention") {
		t.Fatalf("expected markdown output to mention remediation rule id, got %q", output)
	}
}
