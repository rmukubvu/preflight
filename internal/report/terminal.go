// Package report renders assertion results to the terminal and writes
// structured JSON output for CI integration.
package report

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmukubvu/preflight/internal/assertions"
)

var (
	stylePassed = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	styleFailed = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true)
	styleMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	styleHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Bold(true)
)

// PrintSummary writes a coloured assertion summary to w.
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

		passed := 0
		for _, r := range catResults {
			if r.Passed {
				passed++
			}
		}
		total := len(catResults)

		status := stylePassed.Render("✓")
		if passed < total {
			status = styleFailed.Render("✗")
		}

		fmt.Fprintf(w, "  %s %s %s\n",
			status,
			styleHeader.Render(fmt.Sprintf("%-14s", string(cat))),
			styleMuted.Render(fmt.Sprintf("%d/%d", passed, total)),
		)

		for _, r := range catResults {
			if !r.Passed {
				fmt.Fprintf(w, "     %s %s\n",
					styleFailed.Render("✗"),
					r.Message,
				)
			}
		}
	}
}

func groupByCategory(results []assertions.Result) map[assertions.Category][]assertions.Result {
	m := make(map[assertions.Category][]assertions.Result)
	for _, r := range results {
		m[r.Category] = append(m[r.Category], r)
	}
	return m
}
