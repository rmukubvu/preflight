package load

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/config"
	"github.com/rmukubvu/preflight/internal/emulator"
	"github.com/rmukubvu/preflight/internal/stack"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

var (
	stylePass   = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	styleFail   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Bold(true)
	styleMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	styleHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Bold(true)
	styleAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("#58a6ff"))
)

// NewCommand returns the cobra.Command for `preflight load`.
func NewCommand() *cobra.Command {
	var (
		stackType  string
		stackDir   string
		stackName  string
		vus        int
		iterations int
		skipDeploy bool
		runner     string
	)

	cmd := &cobra.Command{
		Use:   "load",
		Short: "Replay behavioural HTTP checks under concurrent load",
		Long: `Deploys the configured stack when needed, then replays HTTP behavioural
checks concurrently against the local emulator to surface latency, error rate,
and failure hotspots before AWS deployment.`,
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

			return Run(cmd.Context(), os.Stdout, Options{
				Config:     cfg,
				StackType:  st,
				StackDir:   stackDir,
				StackName:  stackName,
				VUs:        vus,
				Iterations: iterations,
				SkipDeploy: skipDeploy,
				Runner:     runner,
			})
		},
	}

	cmd.Flags().StringVar(&stackType, "stack-type", "", "Stack type: cdk or terraform (default: auto-detect)")
	cmd.Flags().StringVar(&stackDir, "dir", "", "Stack directory (default: current directory)")
	cmd.Flags().StringVar(&stackName, "stack-name", "", "Stack name used for resource resolution")
	cmd.Flags().IntVar(&vus, "vus", 4, "Concurrent virtual users")
	cmd.Flags().IntVar(&iterations, "iterations", 20, "Total requests per HTTP behavioural assertion")
	cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "Skip deploy and use the current emulator state")
	cmd.Flags().StringVar(&runner, "runner", "auto", "Load runner: auto, native, or k6")

	return cmd
}

// Options configures a load run.
type Options struct {
	Config     config.Config
	StackType  stack.Type
	StackDir   string
	StackName  string
	VUs        int
	Iterations int
	SkipDeploy bool
	Runner     string
}

// Result captures aggregate load output.
type Result struct {
	TotalRequests int
	Failures      int
	AvgLatency    time.Duration
	P95Latency    time.Duration
	ByCheck       []CheckResult
}

// CheckResult captures a single HTTP assertion scenario under load.
type CheckResult struct {
	Name       string
	Total      int
	Failures   int
	AvgLatency time.Duration
	P95Latency time.Duration
	LastError  string
}

// Run executes the load pass.
func Run(ctx context.Context, w io.Writer, opts Options) error {
	if opts.VUs <= 0 {
		opts.VUs = 1
	}
	if opts.Iterations <= 0 {
		opts.Iterations = 1
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n\n", styleAccent.Render("preflight load"))
	fmt.Fprintf(w, "  %s %s detected\n", stylePass.Render("✓"), opts.StackType)

	emulatorMgr := emulator.NewManager(opts.Config.Emulator)
	if _, err := emulatorMgr.EnsureRunning(ctx); err != nil {
		return fmt.Errorf("starting %s: %w", emulatorMgr.DisplayName(), err)
	}
	if emulatorMgr.StopOnExit() {
		defer func() { _ = emulatorMgr.Stop(context.Background()) }()
	}

	if !opts.SkipDeploy {
		if err := deployStack(ctx, opts, emulatorMgr.Endpoint()); err != nil {
			return err
		}
	}

	client, err := awsclient.NewEmulatorClient(emulatorMgr.Endpoint(), opts.Config.Emulator.Type)
	if err != nil {
		return fmt.Errorf("creating aws client: %w", err)
	}

	checks, err := buildHTTPChecks(ctx, client, opts)
	if err != nil {
		return err
	}
	scenarios, err := buildHTTPScenarios(ctx, client, opts)
	if err != nil {
		return err
	}
	if len(checks) == 0 {
		return fmt.Errorf("no behavioural HTTP assertions configured; add assertions.behavioural.http before using preflight load")
	}

	result, err := runHTTPLoad(ctx, client, checks, scenarios, opts)
	if err != nil {
		return err
	}
	printResults(w, result)
	if result.Failures > 0 {
		return fmt.Errorf("load run detected %d failed request(s)", result.Failures)
	}
	return nil
}

const (
	runnerAuto   = "auto"
	runnerNative = "native"
	runnerK6     = "k6"
)

type httpScenario struct {
	Name           string
	Method         string
	URL            string
	ExpectedStatus int
	Body           string
	Headers        map[string]string
}

func runHTTPLoad(ctx context.Context, client awsclient.Client, checks []*assertions.APIGatewayHTTPCheck, scenarios []httpScenario, opts Options) (Result, error) {
	switch normalizeRunner(opts.Runner) {
	case runnerNative:
		return runChecks(ctx, client, checks, opts.VUs, opts.Iterations)
	case runnerK6:
		return runK6(ctx, scenarios, opts.VUs, opts.Iterations)
	case runnerAuto:
		if hasK6Binary() {
			return runK6(ctx, scenarios, opts.VUs, opts.Iterations)
		}
		return runChecks(ctx, client, checks, opts.VUs, opts.Iterations)
	default:
		return Result{}, fmt.Errorf("unsupported load runner %q", opts.Runner)
	}
}

func normalizeRunner(runner string) string {
	runner = strings.ToLower(strings.TrimSpace(runner))
	if runner == "" {
		return runnerAuto
	}
	return runner
}

func printResults(w io.Writer, result Result) {
	status := stylePass.Render("✓")
	if result.Failures > 0 {
		status = styleFail.Render("✗")
	}
	fmt.Fprintf(w, "\n  %s total=%d failures=%d avg=%s p95=%s\n\n",
		status,
		result.TotalRequests,
		result.Failures,
		result.AvgLatency.Round(time.Millisecond),
		result.P95Latency.Round(time.Millisecond),
	)

	for _, check := range result.ByCheck {
		checkStatus := stylePass.Render("✓")
		if check.Failures > 0 {
			checkStatus = styleFail.Render("✗")
		}
		fmt.Fprintf(w, "  %s %s\n", checkStatus, styleHeader.Render(check.Name))
		fmt.Fprintf(w, "    %s total=%d failures=%d avg=%s p95=%s\n",
			styleMuted.Render("metrics:"),
			check.Total,
			check.Failures,
			check.AvgLatency.Round(time.Millisecond),
			check.P95Latency.Round(time.Millisecond),
		)
		if strings.TrimSpace(check.LastError) != "" {
			fmt.Fprintf(w, "    %s %s\n", styleMuted.Render("last error:"), check.LastError)
		}
	}
	fmt.Fprintln(w)
}

func deployStack(ctx context.Context, opts Options, endpoint string) error {
	switch opts.StackType {
	case stack.TypeCDK:
		return newCDKRunner(opts.StackDir, opts.StackName, endpoint, opts.Config.Stack.CDKApp).Deploy(ctx)
	case stack.TypeTerraform:
		return newTerraformRunner(opts.StackDir, opts.StackName, endpoint).Deploy(ctx)
	default:
		return fmt.Errorf("unsupported stack type %s", opts.StackType)
	}
}

type runner interface{ Deploy(context.Context) error }

func newCDKRunner(dir, stackName, endpoint, cdkApp string) runner {
	return cdkRunnerFactory(dir, stackName, endpoint, cdkApp)
}
func newTerraformRunner(dir, stackName, endpoint string) runner {
	return terraformRunnerFactory(dir, stackName, endpoint)
}

var (
	cdkRunnerFactory = func(dir, stackName, endpoint, cdkApp string) runner {
		return &cdkRunner{dir: dir, stackName: stackName, endpoint: endpoint, cdkApp: cdkApp}
	}
	terraformRunnerFactory = func(dir, stackName, endpoint string) runner {
		return &terraformRunner{dir: dir, stackName: stackName, endpoint: endpoint}
	}
)

type cdkRunner struct{ dir, stackName, endpoint, cdkApp string }
type terraformRunner struct{ dir, stackName, endpoint string }

func (r *cdkRunner) Deploy(ctx context.Context) error {
	return runCDKDeploy(ctx, r.dir, r.stackName, r.endpoint, r.cdkApp)
}
func (r *terraformRunner) Deploy(ctx context.Context) error {
	return runTerraformDeploy(ctx, r.dir, r.stackName, r.endpoint)
}

func buildHTTPChecks(ctx context.Context, client awsclient.Client, opts Options) ([]*assertions.APIGatewayHTTPCheck, error) {
	var resources []awsclient.StackResource
	var apis []awsclient.APIDetail
	if opts.StackName != "" {
		var err error
		resources, err = client.CloudFormationStackResources(ctx, opts.StackName)
		if err != nil {
			return nil, fmt.Errorf("discovering stack resources for %q: %w", opts.StackName, err)
		}
	}
	if len(opts.Config.Assertions.Behavioural.HTTP) > 0 {
		var err error
		apis, err = client.APIGatewayV2APIs(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing api gateway apis: %w", err)
		}
	}

	checks := make([]*assertions.APIGatewayHTTPCheck, 0, len(opts.Config.Assertions.Behavioural.HTTP))
	for _, cfg := range opts.Config.Assertions.Behavioural.HTTP {
		apiID := resolveAPIRef(resources, apis, cfg.API)
		check := assertions.NewAPIGatewayHTTPCheck(
			apiID,
			strings.ToUpper(strings.TrimSpace(cfg.Method)),
			cfg.Path,
			[]byte(cfg.Body),
			cfg.ExpectedStatus,
		).WithHeaders(cfg.Headers).WithIntegrationLambda(resolveLambdaRef(resources, cfg.IntegrationFunction))
		checks = append(checks, check)
	}
	return checks, nil
}

func buildHTTPScenarios(ctx context.Context, client awsclient.Client, opts Options) ([]httpScenario, error) {
	var resources []awsclient.StackResource
	var apis []awsclient.APIDetail
	if opts.StackName != "" {
		var err error
		resources, err = client.CloudFormationStackResources(ctx, opts.StackName)
		if err != nil {
			return nil, fmt.Errorf("discovering stack resources for %q: %w", opts.StackName, err)
		}
	}
	if len(opts.Config.Assertions.Behavioural.HTTP) > 0 {
		var err error
		apis, err = client.APIGatewayV2APIs(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing api gateway apis: %w", err)
		}
	}

	scenarios := make([]httpScenario, 0, len(opts.Config.Assertions.Behavioural.HTTP))
	for _, cfg := range opts.Config.Assertions.Behavioural.HTTP {
		apiID := resolveAPIRef(resources, apis, cfg.API)
		baseURL, err := client.APIGatewayV2InvokeURL(ctx, apiID)
		if err != nil {
			return nil, fmt.Errorf("getting invoke URL for API %q: %w", apiID, err)
		}
		scenarios = append(scenarios, httpScenario{
			Name:           fmt.Sprintf("apigw-http:%s %s%s", strings.ToUpper(strings.TrimSpace(cfg.Method)), apiID, normalizePath(cfg.Path)),
			Method:         strings.ToUpper(strings.TrimSpace(cfg.Method)),
			URL:            strings.TrimRight(baseURL, "/") + normalizePath(cfg.Path),
			ExpectedStatus: cfg.ExpectedStatus,
			Body:           cfg.Body,
			Headers:        cloneHeaders(cfg.Headers),
		})
	}
	return scenarios, nil
}

func normalizePath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func runChecks(ctx context.Context, client awsclient.Client, checks []*assertions.APIGatewayHTTPCheck, vus, iterations int) (Result, error) {
	type job struct {
		check *assertions.APIGatewayHTTPCheck
	}
	type sample struct {
		name    string
		latency time.Duration
		passed  bool
		errMsg  string
	}

	jobs := make(chan job)
	results := make(chan sample, len(checks)*iterations)
	var wg sync.WaitGroup
	for i := 0; i < vus; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				start := time.Now()
				result, err := job.check.Run(ctx, client)
				s := sample{name: job.check.Name(), latency: time.Since(start)}
				if err != nil {
					s.errMsg = err.Error()
				} else {
					s.passed = result.Passed
					if !result.Passed {
						s.errMsg = result.Message
					}
				}
				results <- s
			}
		}()
	}

	go func() {
		for _, check := range checks {
			for i := 0; i < iterations; i++ {
				jobs <- job{check: check}
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	byName := map[string]*CheckResult{}
	perCheckLatencies := map[string][]time.Duration{}
	var allLatencies []time.Duration
	total := 0
	failures := 0
	for sample := range results {
		total++
		allLatencies = append(allLatencies, sample.latency)
		entry := byName[sample.name]
		if entry == nil {
			entry = &CheckResult{Name: sample.name}
			byName[sample.name] = entry
		}
		entry.Total++
		entry.AvgLatency += sample.latency
		perCheckLatencies[sample.name] = append(perCheckLatencies[sample.name], sample.latency)
		if !sample.passed || sample.errMsg != "" {
			entry.Failures++
			failures++
			entry.LastError = sample.errMsg
		}
	}

	out := Result{TotalRequests: total, Failures: failures}
	if total > 0 {
		out.AvgLatency = averageLatency(allLatencies)
		out.P95Latency = percentileLatency(allLatencies, 95)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := byName[name]
		if entry.Total > 0 {
			entry.AvgLatency = time.Duration(int64(entry.AvgLatency) / int64(entry.Total))
			entry.P95Latency = percentileLatency(perCheckLatencies[name], 95)
		}
		out.ByCheck = append(out.ByCheck, *entry)
	}
	return out, nil
}

func averageLatency(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var total time.Duration
	for _, value := range values {
		total += value
	}
	return time.Duration(int64(total) / int64(len(values)))
}

func percentileLatency(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}
