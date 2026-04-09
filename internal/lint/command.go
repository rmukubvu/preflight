package lint

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/deploy"
)

var (
	stylePass    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	styleFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	styleHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Bold(true)
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922")).Bold(true)
)

// NewCommand returns the cobra.Command for `preflight lint`.
func NewCommand() *cobra.Command {
	var (
		stackType string
		stackDir  string
		stackName string
		strict    bool
	)

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run static readiness checks against your stack definition",
		Long: `Synthesizes your CDK stack and checks for missing conditions that usually
matter for scalability, reliability, observability, and security before you
deploy anything to AWS.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			cfg = config.Normalize(cfg)

			if stackDir == "" {
				stackDir = workDir
				if cfg.Stack.Dir != "" {
					stackDir = cfg.Stack.Dir
				}
			}

			st := deploy.StackType(stackType)
			if st == deploy.StackTypeUnknown && cfg.Stack.Type != "" {
				st = deploy.StackType(cfg.Stack.Type)
			}
			if st == deploy.StackTypeUnknown {
				st = deploy.DetectStackType(stackDir)
			}
			if st == deploy.StackTypeUnknown {
				return fmt.Errorf("could not detect stack type — set stack.type in .preflight.yaml or use --stack-type")
			}

			rc := runConfig{
				cfg:       cfg,
				stackType: st,
				stackDir:  stackDir,
				stackName: stackName,
				strict:    strict,
			}
			return runLint(cmd.Context(), rc)
		},
	}

	cmd.Flags().StringVar(&stackType, "stack-type", "", "Stack type: cdk or terraform (default: auto-detect)")
	cmd.Flags().StringVar(&stackDir, "dir", "", "Stack directory (default: current directory)")
	cmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name to synthesize")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail on warnings as well as errors")

	return cmd
}

type runConfig struct {
	cfg       config.Config
	stackType deploy.StackType
	stackDir  string
	stackName string
	strict    bool
}

func runLint(ctx context.Context, rc runConfig) error {
	w := os.Stdout

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", styleAccent.Render("preflight lint"))
	fmt.Fprintf(w, "  %s %s detected\n", stylePass.Render("✓"), rc.stackType)

	if rc.stackType != deploy.StackTypeCDK {
		return fmt.Errorf("static lint currently supports CDK/CloudFormation stacks only; detected %s", rc.stackType)
	}

	synthDir, err := synthCDK(ctx, rc.stackDir, rc.stackName, rc.cfg.Stack.CDKApp)
	if err != nil {
		return err
	}
	defer os.RemoveAll(synthDir)
	fmt.Fprintf(w, "  %s synthesized templates %s\n", stylePass.Render("✓"), styleMuted.Render(synthDir))

	templates, err := LoadTemplates(synthDir)
	if err != nil {
		return err
	}

	result := EvaluateTemplates(templates)
	printSummary(w, result)
	printFindings(w, result)

	switch {
	case result.HasErrors():
		return fmt.Errorf("lint found blocking readiness issues")
	case rc.strict && result.HasFindings():
		return fmt.Errorf("lint found readiness warnings in strict mode")
	default:
		return nil
	}
}

func printSummary(w io.Writer, result Result) {
	counts := result.CountsByCategory()
	if len(result.Findings) == 0 {
		fmt.Fprintf(w, "  %s no readiness findings detected in the synthesized templates\n\n", stylePass.Render("✓"))
		return
	}

	order := []Category{
		CategorySecurity,
		CategoryReliability,
		CategoryObservability,
		CategoryScalability,
	}
	fmt.Fprintln(w)
	for _, category := range order {
		if counts[category] == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s %-14s %s\n",
			styleWarning.Render("•"),
			styleHeader.Render(string(category)),
			styleMuted.Render(fmt.Sprintf("%d finding(s)", counts[category])),
		)
	}
	fmt.Fprintln(w)
}

func printFindings(w io.Writer, result Result) {
	if len(result.Findings) == 0 {
		return
	}

	findings := make([]Finding, len(result.Findings))
	copy(findings, result.Findings)
	sort.SliceStable(findings, func(i, j int) bool {
		return findings[i].RuleID < findings[j].RuleID
	})

	for _, finding := range findings {
		icon := styleWarning.Render("WARN")
		if finding.Severity == SeverityError {
			icon = styleFail.Render("ERROR")
		}
		fmt.Fprintf(w, "  %s  %s  %s\n",
			icon,
			styleHeader.Render(fmt.Sprintf("%s/%s", finding.TemplateName, finding.ResourceID)),
			finding.Message,
		)
		if strings.TrimSpace(finding.Recommendation) != "" {
			fmt.Fprintf(w, "        %s %s\n", styleMuted.Render("fix:"), finding.Recommendation)
		}
	}
	fmt.Fprintln(w)
}
