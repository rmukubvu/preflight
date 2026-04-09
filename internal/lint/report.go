package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/rmukubvu/preflight/internal/stack"
)

// Report is the machine-readable and markdown-friendly representation of a lint run.
type Report struct {
	Version      int                  `json:"version"`
	Timestamp    time.Time            `json:"timestamp"`
	StackType    stack.Type           `json:"stack_type"`
	StackDir     string               `json:"stack_dir,omitempty"`
	StackName    string               `json:"stack_name,omitempty"`
	Strict       bool                 `json:"strict"`
	Passed       bool                 `json:"passed"`
	Summary      ReportSummary        `json:"summary"`
	Findings     []ReportFinding      `json:"findings,omitempty"`
	Remediations []RemediationSummary `json:"remediations,omitempty"`
}

type ReportSummary struct {
	TotalFindings int              `json:"total_findings"`
	ErrorCount    int              `json:"error_count"`
	WarningCount  int              `json:"warning_count"`
	ByCategory    map[Category]int `json:"by_category"`
	Scores        []CategoryScore  `json:"scores"`
}

type ReportFinding struct {
	Severity       Severity        `json:"severity"`
	Category       Category        `json:"category"`
	RuleID         string          `json:"rule_id"`
	TemplateName   string          `json:"template_name"`
	ResourceID     string          `json:"resource_id"`
	Message        string          `json:"message"`
	Recommendation string          `json:"recommendation,omitempty"`
	Diagnosis      ReportDiagnosis `json:"diagnosis"`
}

type ReportDiagnosis struct {
	Provider      string  `json:"provider"`
	Deterministic bool    `json:"deterministic"`
	Confidence    float64 `json:"confidence"`
	Explanation   string  `json:"explanation"`
	Suggestion    string  `json:"suggestion,omitempty"`
}

type RemediationSummary struct {
	RuleID            string   `json:"rule_id"`
	Severity          Severity `json:"severity"`
	Category          Category `json:"category"`
	Title             string   `json:"title"`
	WhyItMatters      string   `json:"why_it_matters"`
	SuggestedFix      string   `json:"suggested_fix"`
	AffectedResources []string `json:"affected_resources"`
}

func NewReport(result Result, diagnoses []DiagnosedFinding, opts Options) Report {
	report := Report{
		Version:   1,
		Timestamp: time.Now().UTC(),
		StackType: opts.StackType,
		StackDir:  opts.StackDir,
		StackName: opts.StackName,
		Strict:    opts.Strict,
		Passed:    !result.HasErrors() && !(opts.Strict && result.HasFindings()),
		Summary: ReportSummary{
			TotalFindings: len(result.Findings),
			ByCategory:    result.CountsByCategory(),
			Scores:        result.Scores(),
		},
	}
	for _, finding := range result.Findings {
		if finding.Severity == SeverityError {
			report.Summary.ErrorCount++
		} else {
			report.Summary.WarningCount++
		}
	}

	diagnosisByKey := make(map[string]DiagnosedFinding, len(diagnoses))
	for _, diagnosis := range diagnoses {
		diagnosisByKey[reportFindingKey(diagnosis.Finding)] = diagnosis
	}

	for _, finding := range result.Findings {
		diagnosed := diagnosisByKey[reportFindingKey(finding)]
		report.Findings = append(report.Findings, ReportFinding{
			Severity:       finding.Severity,
			Category:       finding.Category,
			RuleID:         finding.RuleID,
			TemplateName:   finding.TemplateName,
			ResourceID:     finding.ResourceID,
			Message:        finding.Message,
			Recommendation: finding.Recommendation,
			Diagnosis: ReportDiagnosis{
				Provider:      diagnosed.Diagnosis.ProviderName,
				Deterministic: diagnosed.Deterministic,
				Confidence:    diagnosed.Diagnosis.Confidence,
				Explanation:   diagnosed.Diagnosis.Explanation,
				Suggestion:    diagnosed.Diagnosis.Suggestion,
			},
		})
	}
	report.Remediations = remediationSummaries(report.Findings)
	return report
}

func WriteJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func WriteMarkdown(w io.Writer, report Report) error {
	status := "✅ readiness checks passed"
	if !report.Passed {
		status = "❌ readiness issues detected"
	}

	fmt.Fprintf(w, "## preflight lint — %s\n\n", status)
	fmt.Fprintf(w, "**Stack type:** `%s`  \n", report.StackType)
	if report.StackName != "" {
		fmt.Fprintf(w, "**Stack name:** `%s`  \n", report.StackName)
	}
	fmt.Fprintf(w, "**Time:** %s\n\n", report.Timestamp.Format(time.RFC3339))

	fmt.Fprintf(w, "| Category | Score | Errors | Warnings |\n")
	fmt.Fprintf(w, "|----------|-------|--------|----------|\n")
	for _, score := range report.Summary.Scores {
		fmt.Fprintf(w, "| %s | %d | %d | %d |\n", score.Category, score.Score, score.Errors, score.Warnings)
	}
	fmt.Fprintln(w)

	if len(report.Remediations) > 0 {
		fmt.Fprintln(w, "### Recommended next actions")
		fmt.Fprintln(w)
		for _, remediation := range report.Remediations {
			fmt.Fprintf(w, "- **[%s] %s** `%s`\n", remediation.Category, remediation.Title, remediation.RuleID)
			fmt.Fprintf(w, "  - Why it matters: %s\n", remediation.WhyItMatters)
			fmt.Fprintf(w, "  - Suggested fix: %s\n", remediation.SuggestedFix)
			if len(remediation.AffectedResources) > 0 {
				fmt.Fprintf(w, "  - Affected resources: `%s`\n", strings.Join(remediation.AffectedResources, "`, `"))
			}
		}
		fmt.Fprintln(w)
	}

	if len(report.Findings) > 0 {
		fmt.Fprintln(w, "### Findings")
		fmt.Fprintln(w)
		for _, finding := range report.Findings {
			fmt.Fprintf(w, "- **[%s/%s]** `%s/%s` %s\n", finding.Severity, finding.Category, finding.TemplateName, finding.ResourceID, finding.Message)
			if strings.TrimSpace(finding.Diagnosis.Explanation) != "" {
				fmt.Fprintf(w, "  - Diagnosis: %s\n", finding.Diagnosis.Explanation)
			}
			if strings.TrimSpace(finding.Diagnosis.Suggestion) != "" {
				fmt.Fprintf(w, "  - Fix: %s\n", finding.Diagnosis.Suggestion)
			}
		}
	}

	return nil
}

func remediationSummaries(findings []ReportFinding) []RemediationSummary {
	type bucket struct {
		Severity          Severity
		Category          Category
		Title             string
		WhyItMatters      string
		SuggestedFix      string
		AffectedResources map[string]struct{}
	}

	byRule := make(map[string]*bucket)
	for _, finding := range findings {
		entry := byRule[finding.RuleID]
		if entry == nil {
			title := finding.Message
			if idx := strings.Index(title, "."); idx > 0 {
				title = title[:idx]
			}
			entry = &bucket{
				Severity:          finding.Severity,
				Category:          finding.Category,
				Title:             title,
				WhyItMatters:      firstNonEmpty(finding.Diagnosis.Explanation, finding.Message),
				SuggestedFix:      firstNonEmpty(finding.Diagnosis.Suggestion, finding.Recommendation),
				AffectedResources: map[string]struct{}{},
			}
			byRule[finding.RuleID] = entry
		}
		resourceRef := strings.Trim(strings.TrimSpace(finding.TemplateName+"/"+finding.ResourceID), "/")
		if resourceRef != "" {
			entry.AffectedResources[resourceRef] = struct{}{}
		}
	}

	keys := make([]string, 0, len(byRule))
	for key := range byRule {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := byRule[keys[i]]
		right := byRule[keys[j]]
		if left.Severity != right.Severity {
			return left.Severity == SeverityError
		}
		if left.Category != right.Category {
			return left.Category < right.Category
		}
		return keys[i] < keys[j]
	})

	out := make([]RemediationSummary, 0, len(keys))
	for _, key := range keys {
		entry := byRule[key]
		resources := make([]string, 0, len(entry.AffectedResources))
		for resource := range entry.AffectedResources {
			resources = append(resources, resource)
		}
		sort.Strings(resources)
		out = append(out, RemediationSummary{
			RuleID:            key,
			Severity:          entry.Severity,
			Category:          entry.Category,
			Title:             entry.Title,
			WhyItMatters:      entry.WhyItMatters,
			SuggestedFix:      entry.SuggestedFix,
			AffectedResources: resources,
		})
	}
	return out
}

func reportFindingKey(f Finding) string {
	return strings.Join([]string{string(f.Severity), string(f.Category), f.RuleID, f.TemplateName, f.ResourceID}, "::")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
