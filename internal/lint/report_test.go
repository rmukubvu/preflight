package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if len(report.Summary.ScoreDiagnoses) == 0 {
		t.Fatalf("expected score diagnoses to be populated")
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
			ScoreDiagnoses: []ScoreDiagnosisReport{
				{
					Category:     CategoryObservability,
					Score:        88,
					Status:       "watch",
					Explanation:  "This category is being pulled down by Lambda function does not configure explicit log retention.",
					SuggestedFix: "Add aws_cloudwatch_log_group with retention_in_days.",
					TopRuleIDs:   []string{"lambda-log-retention"},
				},
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
	if !strings.Contains(output, "### Score diagnoses") {
		t.Fatalf("expected markdown output to contain score diagnoses section, got %q", output)
	}
	if !strings.Contains(output, "lambda-log-retention") {
		t.Fatalf("expected markdown output to mention remediation rule id, got %q", output)
	}
}

func TestWriteArtifactsWritesJSONAndMarkdownFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "reports", "lint.json")
	mdPath := filepath.Join(tmpDir, "reports", "lint.md")

	report := Report{
		Version:   1,
		Passed:    false,
		StackType: stack.TypeCDK,
		Summary: ReportSummary{
			TotalFindings: 1,
			Scores: []CategoryScore{
				{Category: CategorySecurity, Score: 65, Warnings: 1},
			},
		},
		Findings: []ReportFinding{
			{
				Severity:     SeverityWarning,
				Category:     CategorySecurity,
				RuleID:       "api-auth",
				Message:      "API route does not configure an authorizer.",
				TemplateName: "fixture.template.json",
				ResourceID:   "JobsRoute",
				Diagnosis: ReportDiagnosis{
					Provider:    "rulebook",
					Explanation: "Public routes should be intentional.",
				},
			},
		},
	}

	if err := writeArtifacts(report, jsonPath, mdPath); err != nil {
		t.Fatalf("writeArtifacts returned error: %v", err)
	}

	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading JSON artifact: %v", err)
	}
	if !strings.Contains(string(jsonData), "\"rule_id\": \"api-auth\"") {
		t.Fatalf("expected JSON artifact to contain rule id, got %q", string(jsonData))
	}

	markdownData, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("reading markdown artifact: %v", err)
	}
	if !strings.Contains(string(markdownData), "preflight lint") {
		t.Fatalf("expected markdown artifact header, got %q", string(markdownData))
	}
}
