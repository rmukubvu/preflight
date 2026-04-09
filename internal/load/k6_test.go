package load

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderK6ScriptIncludesScenarioDetails(t *testing.T) {
	t.Parallel()

	script, err := renderK6Script([]httpScenario{{
		Name:           "apigw-http:POST api-123/jobs",
		Method:         "POST",
		URL:            "http://127.0.0.1:4566/_aws/execute-api/api-123/jobs",
		ExpectedStatus: 202,
		Body:           `{"ok":true}`,
		Headers:        map[string]string{"Content-Type": "application/json"},
	}})
	if err != nil {
		t.Fatalf("renderK6Script returned error: %v", err)
	}
	if !strings.Contains(script, "apigw-http:POST api-123/jobs") {
		t.Fatalf("expected script to contain scenario name, got %q", script)
	}
	if !strings.Contains(script, "scenario.metricPrefix + '_latency'") {
		t.Fatalf("expected script to contain per-scenario latency metric, got %q", script)
	}
}

func TestParseK6Summary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	data := `{
  "metrics": {
    "http_req_duration": { "values": { "avg": 12.5, "p(95)": 20.1 } },
    "checks": { "values": { "rate": 0.75 } },
    "preflight_00_latency": { "values": { "avg": 10, "p(95)": 15 } },
    "preflight_00_failures": { "values": { "count": 1 } }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write summary file: %v", err)
	}

	result, err := parseK6Summary(path, []httpScenario{{
		Name: "apigw-http:GET api-123/jobs",
	}}, 4)
	if err != nil {
		t.Fatalf("parseK6Summary returned error: %v", err)
	}
	if result.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests, got %d", result.TotalRequests)
	}
	if result.Failures != 1 {
		t.Fatalf("expected 1 failure, got %d", result.Failures)
	}
	if len(result.ByCheck) != 1 || result.ByCheck[0].Name != "apigw-http:GET api-123/jobs" {
		t.Fatalf("unexpected by-check output: %#v", result.ByCheck)
	}
}
