package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/diagnosis"
)

// Report is the machine-readable output written to JSON and Markdown.
// CI gates read this file to determine pass/fail.
type Report struct {
	Version   int            `json:"version"`
	Timestamp time.Time      `json:"timestamp"`
	StackName string         `json:"stack_name"`
	Passed    bool           `json:"passed"`
	Summary   CategorySummary `json:"summary"`
	Results   []ResultEntry  `json:"results"`
	Diagnoses []DiagnosisEntry `json:"diagnoses,omitempty"`
}

// CategorySummary holds the pass/fail counts per assertion category.
type CategorySummary struct {
	Structural  CategoryCount `json:"structural"`
	Wiring      CategoryCount `json:"wiring"`
	IAM         CategoryCount `json:"iam"`
	Behavioural CategoryCount `json:"behavioural"`
}

// CategoryCount is a pass/total pair.
type CategoryCount struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

// ResultEntry is a single assertion result in the JSON report.
type ResultEntry struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	Passed     bool   `json:"passed"`
	Message    string `json:"message"`
	ResourceID string `json:"resource_id,omitempty"`
}

// DiagnosisEntry captures an AI diagnosis for a specific failed assertion.
type DiagnosisEntry struct {
	AssertionName string `json:"assertion_name"`
	Provider      string `json:"provider"`
	Explanation   string `json:"explanation"`
	Suggestion    string `json:"suggestion,omitempty"`
}

// NewReport constructs a Report from assertion results and optional diagnoses.
func NewReport(stackName string, results []assertions.Result, diagnoses map[string]diagnosis.DiagnoseResponse) Report {
	r := Report{
		Version:   1,
		Timestamp: time.Now().UTC(),
		StackName: stackName,
		Passed:    allPassed(results),
		Results:   make([]ResultEntry, 0, len(results)),
	}

	for _, res := range results {
		r.Results = append(r.Results, ResultEntry{
			Name:       res.Name,
			Category:   string(res.Category),
			Passed:     res.Passed,
			Message:    res.Message,
			ResourceID: res.ResourceID,
		})
		switch res.Category {
		case assertions.CategoryStructural:
			r.Summary.Structural.Total++
			if res.Passed {
				r.Summary.Structural.Passed++
			}
		case assertions.CategoryWiring:
			r.Summary.Wiring.Total++
			if res.Passed {
				r.Summary.Wiring.Passed++
			}
		case assertions.CategoryIAM:
			r.Summary.IAM.Total++
			if res.Passed {
				r.Summary.IAM.Passed++
			}
		case assertions.CategoryBehavioural:
			r.Summary.Behavioural.Total++
			if res.Passed {
				r.Summary.Behavioural.Passed++
			}
		}
	}

	for name, d := range diagnoses {
		r.Diagnoses = append(r.Diagnoses, DiagnosisEntry{
			AssertionName: name,
			Provider:      d.ProviderName,
			Explanation:   d.Explanation,
			Suggestion:    d.Suggestion,
		})
	}

	return r
}

// WriteJSON encodes the report as indented JSON to w.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteMarkdown writes a human-readable Markdown summary to w.
// This is the format used in PR comments.
func WriteMarkdown(w io.Writer, r Report) error {
	status := "✅ All assertions passed"
	if !r.Passed {
		status = "❌ Assertion failures detected"
	}

	fmt.Fprintf(w, "## preflight — %s\n\n", status)
	fmt.Fprintf(w, "**Stack:** `%s`  \n", r.StackName)
	fmt.Fprintf(w, "**Time:** %s\n\n", r.Timestamp.Format(time.RFC3339))

	fmt.Fprintf(w, "| Category | Passed | Total |\n")
	fmt.Fprintf(w, "|----------|--------|-------|\n")
	s := r.Summary
	fmt.Fprintf(w, "| Structural | %d | %d |\n", s.Structural.Passed, s.Structural.Total)
	fmt.Fprintf(w, "| Wiring | %d | %d |\n", s.Wiring.Passed, s.Wiring.Total)
	fmt.Fprintf(w, "| IAM | %d | %d |\n", s.IAM.Passed, s.IAM.Total)
	fmt.Fprintf(w, "| Behavioural | %d | %d |\n\n", s.Behavioural.Passed, s.Behavioural.Total)

	var failures []ResultEntry
	for _, res := range r.Results {
		if !res.Passed {
			failures = append(failures, res)
		}
	}

	if len(failures) > 0 {
		fmt.Fprintf(w, "### Failures\n\n")
		for _, f := range failures {
			fmt.Fprintf(w, "- **[%s]** %s\n", f.Category, f.Message)
		}
		fmt.Fprintln(w)
	}

	if len(r.Diagnoses) > 0 {
		fmt.Fprintf(w, "### AI Diagnosis\n\n")
		for _, d := range r.Diagnoses {
			fmt.Fprintf(w, "**%s** *(via %s)*\n\n%s\n\n", d.AssertionName, d.Provider, d.Explanation)
			if d.Suggestion != "" {
				fmt.Fprintf(w, "```\n%s\n```\n\n", d.Suggestion)
			}
		}
	}

	return nil
}

func allPassed(results []assertions.Result) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}
