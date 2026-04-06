package deploy

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/diagnosis"
	"github.com/rmukubvu/preflight/internal/floci"
	"github.com/rmukubvu/preflight/internal/report"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// NewCommand returns the cobra.Command for `preflight deploy`.
func NewCommand() *cobra.Command {
	var (
		stackType  string
		stackDir   string
		stackName  string
		reportFile string
		noAI       bool
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy your stack to Floci and run assertions",
		Long: `Deploys your CDK or Terraform stack to the Floci local AWS emulator,
then runs structural, wiring, IAM, and behavioural assertions to validate
your infrastructure before it touches real AWS.

Exit code 0: all assertions passed.
Exit code 1: one or more assertions failed.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(workDir)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if stackDir == "" {
				stackDir = workDir
				if cfg.Stack.Dir != "" {
					stackDir = cfg.Stack.Dir
				}
			}

			// Resolve stack type.
			st := StackType(stackType)
			if st == StackTypeUnknown && cfg.Stack.Type != "" {
				st = StackType(cfg.Stack.Type)
			}
			if st == StackTypeUnknown {
				st = DetectStackType(stackDir)
			}
			if st == StackTypeUnknown {
				return fmt.Errorf("could not detect stack type — set stack.type in .preflight.yaml or use --stack-type")
			}

			return runDeploy(cmd.Context(), runConfig{
				cfg:        cfg,
				stackType:  st,
				stackDir:   stackDir,
				stackName:  stackName,
				reportFile: reportFile,
				noAI:       noAI,
			})
		},
	}

	cmd.Flags().StringVar(&stackType, "stack-type", "", "Stack type: cdk or terraform (default: auto-detect)")
	cmd.Flags().StringVar(&stackDir, "dir", "", "Stack directory (default: current directory)")
	cmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name for assertions")
	cmd.Flags().StringVar(&reportFile, "report", "", "Write JSON report to file")
	cmd.Flags().BoolVar(&noAI, "no-ai", false, "Disable AI diagnosis")

	return cmd
}

type runConfig struct {
	cfg        config.Config
	stackType  StackType
	stackDir   string
	stackName  string
	reportFile string
	noAI       bool
}

func runDeploy(ctx context.Context, rc runConfig) error {
	w := os.Stdout

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", styleAccentText("preflight deploy"))

	// ── Detect stack type ──────────────────────────────────────────────────
	fmt.Fprintf(w, "  %s %s detected\n", stylePassText("✓"), rc.stackType)

	// ── Start Floci ────────────────────────────────────────────────────────
	flociMgr := floci.NewManager(rc.cfg.Floci.Image, rc.cfg.Floci.Port)

	report.PrintStep(w, "Starting Floci (local AWS)...", "")
	flociStart := time.Now()
	elapsed, err := flociMgr.EnsureRunning(ctx)
	if err != nil {
		return fmt.Errorf("starting Floci: %w", err)
	}
	if elapsed == 0 {
		fmt.Fprintf(w, "  %s Floci already running\n", stylePassText("✓"))
	} else {
		fmt.Fprintf(w, "    %s\n", styleMutedText(elapsed.Round(time.Millisecond).String()))
	}
	_ = flociStart

	// ── Deploy ─────────────────────────────────────────────────────────────
	runner := buildRunner(rc, flociMgr.Endpoint())

	stepName := fmt.Sprintf("Deploying %s stack...", rc.stackType)
	report.PrintStep(w, stepName, "")

	deployStart := time.Now()
	if err := runner.Deploy(ctx); err != nil {
		return err
	}
	fmt.Fprintf(w, "    %s\n", styleMutedText(time.Since(deployStart).Round(time.Millisecond).String()))

	// ── Assert ─────────────────────────────────────────────────────────────
	report.PrintStep(w, "Running assertion suite (parallel)...", "")
	fmt.Fprintln(w)

	awsClient, err := awsclient.NewFlociClient(flociMgr.Endpoint())
	if err != nil {
		return fmt.Errorf("creating AWS client: %w", err)
	}

	suite := buildAssertionSuite(rc)
	engine := assertions.NewEngine(suite)

	results, _ := engine.RunAll(ctx, awsClient)
	report.PrintSummary(w, results)
	report.PrintFailures(w, results)

	// ── Diagnose failures ──────────────────────────────────────────────────
	diagnoses := make(map[string]diagnosis.DiagnoseResponse)
	if !rc.noAI {
		diagEngine := diagnosis.NewEngine(rc.cfg.LLM)
		for _, r := range results {
			if !r.Passed {
				resp, err := diagEngine.Diagnose(ctx, diagnosis.DiagnoseRequest{
					AssertionName: r.Name,
					FailureReason: r.Message,
					ResourceType:  r.ResourceID,
				})
				if err == nil {
					diagnoses[r.Name] = resp
					report.PrintDiagnosis(w, resp)
				}
			}
		}
	}

	// ── Write report ───────────────────────────────────────────────────────
	if rc.reportFile != "" {
		rpt := report.NewReport(rc.stackName, results, diagnoses)
		f, err := os.Create(rc.reportFile)
		if err != nil {
			fmt.Fprintf(w, "  warning: could not write report: %v\n", err)
		} else {
			defer f.Close()
			_ = report.WriteJSON(f, rpt)
		}
	}

	// ── Exit status ────────────────────────────────────────────────────────
	passed := true
	for _, r := range results {
		if !r.Passed {
			passed = false
			break
		}
	}

	if passed {
		report.PrintSuccess(w)
		return nil
	}
	return fmt.Errorf("assertion suite failed — fix the issues above before deploying to AWS")
}

func buildRunner(rc runConfig, endpoint string) Runner {
	switch rc.stackType {
	case StackTypeCDK:
		return NewCDKRunner(rc.stackDir, rc.stackName, endpoint, rc.cfg.Stack.CDKApp)
	case StackTypeTerraform:
		return NewTerraformRunner(rc.stackDir, rc.stackName, endpoint)
	default:
		// Should not be reached — validated before calling runDeploy.
		panic("unknown stack type: " + rc.stackType)
	}
}

func buildAssertionSuite(rc runConfig) []assertions.Assertion {
	var suite []assertions.Assertion

	// Always include the stack-level structural checks if a stack name is known.
	if rc.stackName != "" {
		suite = append(suite,
			assertions.NewStackDeployed(rc.stackName),
			assertions.NewResourcesCreateComplete(rc.stackName),
		)
	}

	return suite
}

// ─── minimal style helpers (avoids importing lipgloss in command layer) ───────

func styleAccentText(s string) string  { return "\033[34m" + s + "\033[0m" }
func stylePassText(s string) string    { return "\033[32m" + s + "\033[0m" }
func styleMutedText(s string) string   { return "\033[90m" + s + "\033[0m" }
