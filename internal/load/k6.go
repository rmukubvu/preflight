package load

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type k6Metric struct {
	Values map[string]float64 `json:"values"`
}

type k6Summary struct {
	Metrics map[string]k6Metric `json:"metrics"`
}

type k6ScriptScenario struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	ExpectedStatus int               `json:"expectedStatus"`
	Body           string            `json:"body,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	MetricPrefix   string            `json:"metricPrefix"`
}

var lookPath = exec.LookPath

func hasK6Binary() bool {
	_, err := lookPath("k6")
	return err == nil
}

func runK6(ctx context.Context, scenarios []httpScenario, vus, iterations int) (Result, error) {
	if len(scenarios) == 0 {
		return Result{}, fmt.Errorf("no behavioural HTTP assertions configured for k6 execution")
	}
	k6Path, err := lookPath("k6")
	if err != nil {
		return Result{}, fmt.Errorf("k6 runner requested but k6 binary is not installed")
	}

	workDir, err := os.MkdirTemp("", "preflight-k6-*")
	if err != nil {
		return Result{}, fmt.Errorf("creating k6 temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	scriptPath := filepath.Join(workDir, "scenario.js")
	summaryPath := filepath.Join(workDir, "summary.json")
	script, err := renderK6Script(scenarios)
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return Result{}, fmt.Errorf("writing k6 script: %w", err)
	}

	cmd := exec.CommandContext(
		ctx,
		k6Path,
		"run",
		"--quiet",
		"--vus", strconv.Itoa(vus),
		"--iterations", strconv.Itoa(iterations),
		"--summary-export", summaryPath,
		scriptPath,
	)
	output, runErr := cmd.CombinedOutput()

	result, parseErr := parseK6Summary(summaryPath, scenarios, iterations)
	if parseErr != nil {
		if runErr != nil {
			return Result{}, fmt.Errorf("running k6: %v\n%s", runErr, strings.TrimSpace(string(output)))
		}
		return Result{}, parseErr
	}
	if runErr != nil {
		return result, fmt.Errorf("k6 load run detected failed checks: %s", strings.TrimSpace(string(output)))
	}
	return result, nil
}

func renderK6Script(scenarios []httpScenario) (string, error) {
	payload := make([]k6ScriptScenario, 0, len(scenarios))
	for index, scenario := range scenarios {
		payload = append(payload, k6ScriptScenario{
			Name:           scenario.Name,
			Method:         scenario.Method,
			URL:            scenario.URL,
			ExpectedStatus: scenario.ExpectedStatus,
			Body:           scenario.Body,
			Headers:        scenario.Headers,
			MetricPrefix:   fmt.Sprintf("preflight_%02d", index),
		})
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshalling k6 scenarios: %w", err)
	}

	return fmt.Sprintf(`import http from 'k6/http';
import { check } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const scenarios = %s;
const metrics = {};

for (const scenario of scenarios) {
  metrics[scenario.metricPrefix] = {
    latency: new Trend(scenario.metricPrefix + '_latency'),
    failures: new Counter(scenario.metricPrefix + '_failures'),
  };
}

export const options = {
  thresholds: {
    checks: ['rate==1.0'],
  },
};

export default function () {
  for (const scenario of scenarios) {
    const params = { headers: scenario.headers || {} };
    const response = http.request(scenario.method, scenario.url, scenario.body || null, params);
    const passed = check(response, {
      ['status ' + scenario.expectedStatus + ' for ' + scenario.name]: (r) => r.status === scenario.expectedStatus,
    });
    metrics[scenario.metricPrefix].latency.add(response.timings.duration);
    if (!passed) {
      metrics[scenario.metricPrefix].failures.add(1);
    }
  }
}
`, string(data)), nil
}

func parseK6Summary(path string, scenarios []httpScenario, iterations int) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("reading k6 summary: %w", err)
	}

	var summary k6Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return Result{}, fmt.Errorf("parsing k6 summary: %w", err)
	}

	result := Result{
		TotalRequests: len(scenarios) * iterations,
	}
	if metric, ok := summary.Metrics["http_req_duration"]; ok {
		result.AvgLatency = metricDuration(metric.Values["avg"])
		result.P95Latency = metricDuration(metric.Values["p(95)"])
	}
	if metric, ok := summary.Metrics["checks"]; ok {
		rate := metric.Values["rate"]
		if rate < 1 {
			result.Failures = int((1 - rate) * float64(result.TotalRequests))
			if result.Failures == 0 {
				result.Failures = 1
			}
		}
	}

	nameByPrefix := make(map[string]string, len(scenarios))
	for index, scenario := range scenarios {
		nameByPrefix[fmt.Sprintf("preflight_%02d", index)] = scenario.Name
	}
	prefixes := make([]string, 0)
	for name := range summary.Metrics {
		if strings.HasPrefix(name, "preflight_") && strings.HasSuffix(name, "_latency") {
			prefixes = append(prefixes, strings.TrimSuffix(name, "_latency"))
		}
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		check := CheckResult{
			Name:       nameByPrefix[prefix],
			Total:      iterations,
			AvgLatency: metricDuration(summary.Metrics[prefix+"_latency"].Values["avg"]),
			P95Latency: metricDuration(summary.Metrics[prefix+"_latency"].Values["p(95)"]),
			Failures:   int(summary.Metrics[prefix+"_failures"].Values["count"]),
		}
		result.ByCheck = append(result.ByCheck, check)
	}
	return result, nil
}

func metricDuration(value float64) time.Duration {
	return time.Duration(value * float64(time.Millisecond))
}
