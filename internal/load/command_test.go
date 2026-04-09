package load

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/config"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

type fakeClient struct {
	invokeURL  string
	resources  []awsclient.StackResource
	apis       []awsclient.APIDetail
	lambdaResp []byte
	lambdaErr  error
}

func (f fakeClient) CloudFormationStackStatus(context.Context, string) (string, error) {
	return "CREATE_COMPLETE", nil
}

func (f fakeClient) CloudFormationStackResources(context.Context, string) ([]awsclient.StackResource, error) {
	return f.resources, nil
}

func (f fakeClient) LambdaFunctionExists(context.Context, string) (bool, error) {
	return true, nil
}

func (f fakeClient) LambdaFunctionConfig(context.Context, string) (awsclient.LambdaConfig, error) {
	return awsclient.LambdaConfig{}, nil
}

func (f fakeClient) LambdaEventSourceMappings(context.Context, string) ([]awsclient.EventSourceMapping, error) {
	return nil, nil
}

func (f fakeClient) LambdaInvoke(context.Context, string, []byte) ([]byte, error) {
	return f.lambdaResp, f.lambdaErr
}

func (f fakeClient) SQSQueueExists(context.Context, string) (bool, error) {
	return true, nil
}

func (f fakeClient) SQSQueueURL(context.Context, string) (string, error) {
	return "https://example.test/queue", nil
}

func (f fakeClient) SQSSendMessage(context.Context, string, string) error {
	return nil
}

func (f fakeClient) S3BucketExists(context.Context, string) (bool, error) {
	return true, nil
}

func (f fakeClient) S3BucketNotificationTargets(context.Context, string) ([]awsclient.S3Notification, error) {
	return nil, nil
}

func (f fakeClient) IAMRoleExists(context.Context, string) (bool, error) {
	return true, nil
}

func (f fakeClient) IAMRolePolicies(context.Context, string) ([]awsclient.PolicyStatement, error) {
	return nil, nil
}

func (f fakeClient) APIGatewayV2Routes(context.Context, string) ([]awsclient.APIRoute, error) {
	return nil, nil
}

func (f fakeClient) APIGatewayV2APIs(context.Context) ([]awsclient.APIDetail, error) {
	return f.apis, nil
}

func (f fakeClient) APIGatewayV2InvokeURL(context.Context, string) (string, error) {
	return f.invokeURL, nil
}

func (f fakeClient) DynamoDBTableExists(context.Context, string) (bool, error) {
	return true, nil
}

func (f fakeClient) DynamoDBGetItem(context.Context, string, map[string]string) (map[string]string, error) {
	return nil, nil
}

func TestBuildHTTPChecksResolvesStackResources(t *testing.T) {
	t.Parallel()

	client := fakeClient{
		resources: []awsclient.StackResource{
			{LogicalID: "HttpApi", PhysicalID: "api-123", Type: "AWS::ApiGatewayV2::Api"},
			{LogicalID: "JobsHandler", PhysicalID: "jobs-handler-live", Type: "AWS::Lambda::Function"},
		},
		apis: []awsclient.APIDetail{
			{APIID: "api-123", Name: "fixture-http-api"},
		},
	}

	opts := Options{
		StackName: "SmokeFixtureStack",
		Config: config.Config{
			Assertions: config.AssertionsConfig{
				Behavioural: config.BehaviouralConfig{
					HTTP: []config.HTTPCheckConfig{
						{
							API:                 "HttpApi",
							IntegrationFunction: "JobsHandler",
							Method:              "post",
							Path:                "/jobs",
							ExpectedStatus:      http.StatusAccepted,
							Body:                `{"ok":true}`,
							Headers:             map[string]string{"X-Test": "1"},
						},
					},
				},
			},
		},
	}

	checks, err := buildHTTPChecks(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("buildHTTPChecks returned error: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}

	check := checks[0]
	if got := check.Name(); got != "apigw-http:POST api-123/jobs" {
		t.Fatalf("unexpected check name %q", got)
	}
}

func TestRunChecksAggregatesSuccessesAndFailures(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := fakeClient{invokeURL: server.URL}

	okCheck := assertions.NewAPIGatewayHTTPCheck("api-123", http.MethodGet, "/ok", nil, http.StatusOK)
	failCheck := assertions.NewAPIGatewayHTTPCheck("api-123", http.MethodGet, "/fail", nil, http.StatusOK)

	result, err := runChecks(context.Background(), client, []*assertions.APIGatewayHTTPCheck{okCheck, failCheck}, 3, 2)
	if err != nil {
		t.Fatalf("runChecks returned error: %v", err)
	}

	if result.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests, got %d", result.TotalRequests)
	}
	if result.Failures != 2 {
		t.Fatalf("expected 2 failures, got %d", result.Failures)
	}
	if len(result.ByCheck) != 2 {
		t.Fatalf("expected 2 grouped check results, got %d", len(result.ByCheck))
	}

	if result.ByCheck[0].Name != "apigw-http:GET api-123/fail" {
		t.Fatalf("expected sorted failing check first, got %q", result.ByCheck[0].Name)
	}
	if result.ByCheck[0].Failures != 2 {
		t.Fatalf("expected failing check to record 2 failures, got %d", result.ByCheck[0].Failures)
	}
	if result.ByCheck[1].Failures != 0 {
		t.Fatalf("expected passing check to record 0 failures, got %d", result.ByCheck[1].Failures)
	}
}

func TestAverageLatency(t *testing.T) {
	t.Parallel()

	got := averageLatency([]time.Duration{time.Second, 3 * time.Second, 2 * time.Second})
	if want := 2 * time.Second; got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}

	if got := averageLatency(nil); got != 0 {
		t.Fatalf("expected zero for empty slice, got %s", got)
	}
}

func TestPercentileLatency(t *testing.T) {
	t.Parallel()

	values := []time.Duration{
		500 * time.Millisecond,
		1500 * time.Millisecond,
		250 * time.Millisecond,
		2 * time.Second,
		750 * time.Millisecond,
	}

	if got, want := percentileLatency(values, 95), 1500*time.Millisecond; got != want {
		t.Fatalf("expected p95 %s, got %s", want, got)
	}

	if got := percentileLatency(nil, 95); got != 0 {
		t.Fatalf("expected zero for empty slice, got %s", got)
	}
}
