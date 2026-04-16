package deploy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/diagnosis"
	"github.com/rmukubvu/preflight/internal/emulator"
	"github.com/rmukubvu/preflight/internal/lint"
	"github.com/rmukubvu/preflight/internal/report"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// NewCommand returns the cobra.Command for `preflight deploy`.
func NewCommand() *cobra.Command {
	var (
		stackType              string
		stackDir               string
		stackName              string
		reportFile             string
		lintReportJSONFile     string
		lintReportMarkdownFile string
		noAI                   bool
		skipLint               bool
		lintStrict             bool
	)

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy your stack to a local AWS emulator and run assertions",
		Long: `Deploys your CDK or Terraform stack to the configured local AWS emulator,
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
			cfg = config.Normalize(cfg)

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
				cfg:                    cfg,
				stackType:              st,
				stackDir:               stackDir,
				stackName:              stackName,
				reportFile:             reportFile,
				lintReportJSONFile:     lintReportJSONFile,
				lintReportMarkdownFile: lintReportMarkdownFile,
				noAI:                   noAI,
				skipLint:               skipLint,
				lintStrict:             lintStrict,
			})
		},
	}

	cmd.Flags().StringVar(&stackType, "stack-type", "", "Stack type: cdk, pulumi, or terraform (default: auto-detect)")
	cmd.Flags().StringVar(&stackDir, "dir", "", "Stack directory (default: current directory)")
	cmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name for assertions")
	cmd.Flags().StringVar(&reportFile, "report", "", "Write JSON report to file")
	cmd.Flags().StringVar(&lintReportJSONFile, "lint-report-json", "", "Write the pre-deploy lint JSON report to file")
	cmd.Flags().StringVar(&lintReportMarkdownFile, "lint-report-markdown", "", "Write the pre-deploy lint markdown report to file")
	cmd.Flags().BoolVar(&noAI, "no-ai", false, "Disable AI diagnosis")
	cmd.Flags().BoolVar(&skipLint, "skip-lint", false, "Skip the static readiness lint pass before deploy")
	cmd.Flags().BoolVar(&lintStrict, "lint-strict", false, "Fail deploy when lint reports warnings as well as errors")

	return cmd
}

type runConfig struct {
	cfg                    config.Config
	stackType              StackType
	stackDir               string
	stackName              string
	reportFile             string
	lintReportJSONFile     string
	lintReportMarkdownFile string
	noAI                   bool
	skipLint               bool
	lintStrict             bool
}

func runDeploy(ctx context.Context, rc runConfig) error {
	w := os.Stdout

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", styleAccentText("preflight deploy"))

	// ── Detect stack type ──────────────────────────────────────────────────
	fmt.Fprintf(w, "  %s %s detected\n", stylePassText("✓"), rc.stackType)

	// ── Static readiness lint ──────────────────────────────────────────────
	if rc.skipLint {
		fmt.Fprintf(w, "  %s static readiness lint skipped\n", styleMutedText("•"))
	} else {
		report.PrintStep(w, "Running static readiness checks...", "")
		if _, err := lint.Run(ctx, w, lint.Options{
			StackType:          rc.stackType,
			StackDir:           rc.stackDir,
			StackName:          rc.stackName,
			CDKApp:             rc.cfg.Stack.CDKApp,
			Strict:             rc.lintStrict,
			LLM:                lint.EffectiveLLM(rc.cfg.LLM, rc.noAI),
			Diagnose:           true,
			ReportJSONFile:     rc.lintReportJSONFile,
			ReportMarkdownFile: rc.lintReportMarkdownFile,
		}); err != nil {
			return err
		}
	}

	// ── Start emulator ─────────────────────────────────────────────────────
	emulatorMgr := emulator.NewManager(rc.cfg.Emulator)

	report.PrintStep(w, fmt.Sprintf("Starting %s (local AWS)...", emulatorMgr.DisplayName()), "")
	elapsed, err := emulatorMgr.EnsureRunning(ctx)
	if err != nil {
		return fmt.Errorf("starting %s: %w", emulatorMgr.DisplayName(), err)
	}
	if elapsed == 0 {
		fmt.Fprintf(w, "  %s %s already running\n", stylePassText("✓"), emulatorMgr.DisplayName())
	} else {
		fmt.Fprintf(w, "    %s\n", styleMutedText(elapsed.Round(time.Millisecond).String()))
	}
	if emulatorMgr.StopOnExit() {
		defer func() {
			_ = emulatorMgr.Stop(context.Background())
		}()
	}

	// ── Deploy ─────────────────────────────────────────────────────────────
	runner := buildRunner(rc, emulatorMgr.Endpoint())

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

	awsClient, err := awsclient.NewEmulatorClient(emulatorMgr.Endpoint(), rc.cfg.Emulator.Type)
	if err != nil {
		return fmt.Errorf("creating AWS client: %w", err)
	}

	suite, err := buildAssertionSuite(ctx, awsClient, rc)
	if err != nil {
		return err
	}
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
	case StackTypePulumi:
		return NewPulumiRunner(rc.stackDir, rc.stackName, endpoint)
	case StackTypeTerraform:
		return NewTerraformRunner(rc.stackDir, rc.stackName, endpoint)
	default:
		// Should not be reached — validated before calling runDeploy.
		panic("unknown stack type: " + rc.stackType)
	}
}

func buildAssertionSuite(ctx context.Context, client awsclient.Client, rc runConfig) ([]assertions.Assertion, error) {
	var (
		suite     []assertions.Assertion
		resources []awsclient.StackResource
		apis      []awsclient.APIDetail
	)

	if rc.stackName != "" && rc.stackType != StackTypePulumi {
		var err error
		resources, err = client.CloudFormationStackResources(ctx, rc.stackName)
		if err != nil {
			return nil, fmt.Errorf("discovering stack resources for %q: %w", rc.stackName, err)
		}
		suite = append(suite, assertions.DiscoverSuiteFromResources(rc.stackName, resources)...)
	}

	if len(rc.cfg.Assertions.Behavioural.HTTP) > 0 {
		var err error
		apis, err = client.APIGatewayV2APIs(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing API Gateway APIs: %w", err)
		}
	}

	suite = append(suite, buildConfiguredBehaviouralAssertions(rc.cfg.Assertions.Behavioural, resources, apis)...)
	return suite, nil
}

func buildConfiguredBehaviouralAssertions(cfg config.BehaviouralConfig, resources []awsclient.StackResource, apis []awsclient.APIDetail) []assertions.Assertion {
	suite := make([]assertions.Assertion, 0, len(cfg.HTTP)+len(cfg.SQSToLambdaToDynamo))

	for _, check := range cfg.HTTP {
		apiID := resolveAPIRef(resources, apis, check.API)
		httpCheck := assertions.NewAPIGatewayHTTPCheck(
			apiID,
			strings.ToUpper(strings.TrimSpace(check.Method)),
			check.Path,
			[]byte(check.Body),
			check.ExpectedStatus,
		).WithHeaders(check.Headers)
		httpCheck = httpCheck.WithIntegrationLambda(resolveLambdaRef(resources, check.IntegrationFunction))
		suite = append(suite, httpCheck)
	}

	for _, check := range cfg.SQSToLambdaToDynamo {
		queueName := resolveQueueRef(resources, check.Queue)
		tableName := resolveTableRef(resources, check.Table)
		queueCheck := assertions.NewSQSToLambdaToDynamoDB(queueName, check.MessageBody, tableName, check.ExpectedKey).
			WithConsumerLambda(resolveLambdaRef(resources, check.ConsumerFunction))
		suite = append(suite, queueCheck)
	}

	return suite
}

func resolveAPIRef(resources []awsclient.StackResource, apis []awsclient.APIDetail, ref string) string {
	if resolved := resolvePhysicalResourceID(resources, "AWS::ApiGatewayV2::Api", ref); resolved != ref {
		for _, api := range apis {
			if api.APIID == resolved || api.Name == resolved {
				return api.APIID
			}
		}
	}

	ref = strings.TrimSpace(ref)
	for _, api := range apis {
		if api.APIID == ref || api.Name == ref || strings.HasPrefix(api.Name, ref) {
			return api.APIID
		}
	}

	return ref
}

func resolveQueueRef(resources []awsclient.StackResource, ref string) string {
	resolved := resolvePhysicalResourceID(resources, "AWS::SQS::Queue", ref)
	return sqsQueueName(resolved)
}

func resolveTableRef(resources []awsclient.StackResource, ref string) string {
	return resolvePhysicalResourceID(resources, "AWS::DynamoDB::Table", ref)
}

func resolveLambdaRef(resources []awsclient.StackResource, ref string) string {
	return resolvePhysicalResourceID(resources, "AWS::Lambda::Function", ref)
}

func resolvePhysicalResourceID(resources []awsclient.StackResource, resourceType, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}

	for _, resource := range resources {
		if resource.Type != resourceType {
			continue
		}
		if resource.LogicalID == ref || resource.PhysicalID == ref {
			return resource.PhysicalID
		}
		if strings.HasPrefix(resource.LogicalID, ref) {
			return resource.PhysicalID
		}
	}

	return ref
}

func sqsQueueName(ref string) string {
	if ref == "" {
		return ""
	}

	if parts := strings.Split(ref, "/"); len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != "" {
			return last
		}
	}

	if parts := strings.Split(ref, ":"); len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ref
}

// ─── minimal style helpers (avoids importing lipgloss in command layer) ───────

func styleAccentText(s string) string { return "\033[34m" + s + "\033[0m" }
func stylePassText(s string) string   { return "\033[32m" + s + "\033[0m" }
func styleMutedText(s string) string  { return "\033[90m" + s + "\033[0m" }
