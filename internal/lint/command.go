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
	"github.com/rmukubvu/preflight/internal/stack"
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

			st := stack.Type(stackType)
			if st == stack.TypeUnknown && cfg.Stack.Type != "" {
				st = stack.Type(cfg.Stack.Type)
			}
			if st == stack.TypeUnknown {
				st = stack.Detect(stackDir)
			}
			if st == stack.TypeUnknown {
				return fmt.Errorf("could not detect stack type — set stack.type in .preflight.yaml or use --stack-type")
			}

			_, err = Run(cmd.Context(), os.Stdout, Options{
				StackType: st,
				StackDir:  stackDir,
				StackName: stackName,
				CDKApp:    cfg.Stack.CDKApp,
				Strict:    strict,
			})
			return err
		},
	}

	cmd.Flags().StringVar(&stackType, "stack-type", "", "Stack type: cdk or terraform (default: auto-detect)")
	cmd.Flags().StringVar(&stackDir, "dir", "", "Stack directory (default: current directory)")
	cmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name to synthesize")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail on warnings as well as errors")

	return cmd
}

// Options configures a lint run.
type Options struct {
	StackType stack.Type
	StackDir  string
	StackName string
	CDKApp    string
	Strict    bool
}

// Run executes the static readiness pass and writes a terminal summary.
func Run(ctx context.Context, w io.Writer, opts Options) (Result, error) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", styleAccent.Render("preflight lint"))
	fmt.Fprintf(w, "  %s %s detected\n", stylePass.Render("✓"), opts.StackType)

	if opts.StackType != stack.TypeCDK {
		return Result{}, fmt.Errorf("static lint currently supports CDK/CloudFormation stacks only; detected %s", opts.StackType)
	}

	synthDir, err := synthCDK(ctx, opts.StackDir, opts.StackName, opts.CDKApp)
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(synthDir)
	fmt.Fprintf(w, "  %s synthesized templates %s\n", stylePass.Render("✓"), styleMuted.Render(synthDir))

	templates, err := LoadTemplates(synthDir)
	if err != nil {
		return Result{}, err
	}

	result := EvaluateTemplates(templates)
	printSummary(w, result)
	printFindings(w, result)

	switch {
	case result.HasErrors():
		return result, fmt.Errorf("lint found blocking readiness issues")
	case opts.Strict && result.HasFindings():
		return result, fmt.Errorf("lint found readiness warnings in strict mode")
	default:
		return result, nil
	}
}

func printSummary(w io.Writer, result Result) {
	counts := result.CountsByCategory()
	if len(result.Findings) == 0 {
		fmt.Fprintf(w, "  %s no readiness findings detected in the synthesized templates\n", stylePass.Render("✓"))
		for _, score := range result.Scores() {
			fmt.Fprintf(w, "  %s %-14s %s\n",
				stylePass.Render("100"),
				styleHeader.Render(string(score.Category)),
				styleMuted.Render("clear in current rule pack"),
			)
		}
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintln(w)
	for _, score := range result.Scores() {
		scoreText := stylePass.Render(fmt.Sprintf("%3d", score.Score))
		if score.Errors > 0 {
			scoreText = styleFail.Render(fmt.Sprintf("%3d", score.Score))
		} else if score.Warnings > 0 {
			scoreText = styleWarning.Render(fmt.Sprintf("%3d", score.Score))
		}
		fmt.Fprintf(w, "  %s %-14s %s\n",
			scoreText,
			styleHeader.Render(string(score.Category)),
			styleMuted.Render(fmt.Sprintf("%d finding(s)", counts[score.Category])),
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
