// Package report renders assertion results to the terminal and writes
// structured JSON/Markdown output for CI integration.
package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/diagnosis"
)

var (
	stylePass    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	styleFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	styleHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Bold(true)
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
)

const (
	barFilled = "█"
	barEmpty  = "░"
	barWidth  = 12
)

// PrintSummary writes the assertion suite summary to w with lipgloss styling.
func PrintSummary(w io.Writer, results []assertions.Result) {
	byCategory := groupByCategory(results)

	for _, cat := range []assertions.Category{
		assertions.CategoryStructural,
		assertions.CategoryWiring,
		assertions.CategoryIAM,
		assertions.CategoryBehavioural,
	} {
		catResults, ok := byCategory[cat]
		if !ok {
			continue
		}
		printCategoryLine(w, cat, catResults)
	}
}

// PrintFailures writes detailed failure messages to w.
func PrintFailures(w io.Writer, results []assertions.Result) {
	failures := failedResults(results)
	if len(failures) == 0 {
		return
	}

	fmt.Fprintf(w, "\n%s\n\n", styleHeader.Render(fmt.Sprintf("%d issue(s) found before reaching AWS", len(failures))))
	for _, r := range failures {
		fmt.Fprintf(w, "  %s %s  %s\n",
			styleFail.Render("✗"),
			styleWarning.Render(string(r.Category)),
			r.Message,
		)
	}
}

// PrintDiagnosis writes a diagnosis result to w.
func PrintDiagnosis(w io.Writer, dr diagnosis.DiagnoseResponse) {
	fmt.Fprintf(w, "\n%s %s\n\n",
		styleAccent.Render("◆ Diagnosis"),
		styleMuted.Render(fmt.Sprintf("via %s", dr.ProviderName)),
	)
	fmt.Fprintf(w, "%s\n", dr.Explanation)
	if dr.Suggestion != "" {
		fmt.Fprintf(w, "\n%s\n%s\n",
			styleMuted.Render("Suggested fix:"),
			dr.Suggestion,
		)
	}
}

// PrintStep writes a named step with its elapsed time (e.g. "◆ Starting Floci... 24ms").
func PrintStep(w io.Writer, name, detail string) {
	if detail != "" {
		fmt.Fprintf(w, "  %s %s %s\n",
			styleAccent.Render("◆"),
			name,
			styleMuted.Render(detail),
		)
	} else {
		fmt.Fprintf(w, "  %s %s\n", styleAccent.Render("◆"), name)
	}
}

// PrintSuccess writes the final pass message.
func PrintSuccess(w io.Writer) {
	fmt.Fprintf(w, "\n  %s  All assertions passed. Safe to deploy to AWS.\n\n",
		stylePass.Render("✓"),
	)
}

// ─── private helpers ─────────────────────────────────────────────────────────

func printCategoryLine(w io.Writer, cat assertions.Category, results []assertions.Result) {
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	total := len(results)
	allPass := passed == total

	bar := buildBar(passed, total)
	status := stylePass.Render("✓")
	if !allPass {
		status = styleFail.Render("✗")
	}

	fmt.Fprintf(w, "  %s %-14s %s %s\n",
		status,
		styleHeader.Render(string(cat)),
		bar,
		styleMuted.Render(fmt.Sprintf("%d/%d", passed, total)),
	)
}

func buildBar(passed, total int) string {
	if total == 0 {
		return strings.Repeat(barEmpty, barWidth)
	}
	filled := (passed * barWidth) / total
	return stylePass.Render(strings.Repeat(barFilled, filled)) +
		styleMuted.Render(strings.Repeat(barEmpty, barWidth-filled))
}

func groupByCategory(results []assertions.Result) map[assertions.Category][]assertions.Result {
	m := make(map[assertions.Category][]assertions.Result)
	for _, r := range results {
		m[r.Category] = append(m[r.Category], r)
	}
	return m
}

func failedResults(results []assertions.Result) []assertions.Result {
	var out []assertions.Result
	for _, r := range results {
		if !r.Passed {
			out = append(out, r)
		}
	}
	return out
}
