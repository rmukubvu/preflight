package assertions_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmukubvu/preflight/internal/assertions"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

var ctx = context.Background()

// ── Engine ───────────────────────────────────────────────────────────────────

func TestEngine_RunAll_ReturnsResultPerAssertion(t *testing.T) {
	client := &awsclient.MockClient{}
	engine := assertions.NewEngine([]assertions.Assertion{
		assertions.NewStackDeployed("my-stack"),
		assertions.NewLambdaExists("MyFunction"),
	})

	results, err := engine.RunAll(ctx, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("want 2 results, got %d", len(results))
	}
}

func TestEngine_RunAll_AllPassOnHappyPath(t *testing.T) {
	client := &awsclient.MockClient{}
	engine := assertions.NewEngine([]assertions.Assertion{
		assertions.NewStackDeployed("my-stack"),
		assertions.NewSQSQueueExists("MyQueue"),
		assertions.NewLambdaExists("MyFunction"),
	})

	results, _ := engine.RunAll(ctx, client)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("assertion %q should pass with default mock, got: %s", r.Name, r.Message)
		}
	}
}

// ── Structural ────────────────────────────────────────────────────────────────

func TestStackDeployed_Pass_WhenCreateComplete(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackStatusFn: func(_ context.Context, _ string) (string, error) {
			return "CREATE_COMPLETE", nil
		},
	}
	result, err := assertions.NewStackDeployed("my-stack").Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
}

func TestStackDeployed_Fail_WhenRollback(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackStatusFn: func(_ context.Context, _ string) (string, error) {
			return "ROLLBACK_COMPLETE", nil
		},
	}
	result, err := assertions.NewStackDeployed("my-stack").Run(ctx, client)
	assertNoError(t, err)
	assertFailed(t, result)
}

func TestStackDeployed_Fail_WhenDoesNotExist(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackStatusFn: func(_ context.Context, _ string) (string, error) {
			return "DOES_NOT_EXIST", nil
		},
	}
	result, _ := assertions.NewStackDeployed("my-stack").Run(ctx, client)
	assertFailed(t, result)
}

func TestResourcesCreateComplete_Pass_WhenAllComplete(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackResourcesFn: func(_ context.Context, _ string) ([]awsclient.StackResource, error) {
			return []awsclient.StackResource{
				{LogicalID: "MyLambda", Type: "AWS::Lambda::Function", Status: "CREATE_COMPLETE"},
				{LogicalID: "MyQueue", Type: "AWS::SQS::Queue", Status: "CREATE_COMPLETE"},
			}, nil
		},
	}
	result, _ := assertions.NewResourcesCreateComplete("my-stack").Run(ctx, client)
	assertPassed(t, result)
}

func TestResourcesCreateComplete_Fail_WhenResourceFailed(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackResourcesFn: func(_ context.Context, _ string) ([]awsclient.StackResource, error) {
			return []awsclient.StackResource{
				{LogicalID: "MyLambda", Type: "AWS::Lambda::Function", Status: "CREATE_FAILED"},
				{LogicalID: "MyQueue", Type: "AWS::SQS::Queue", Status: "CREATE_COMPLETE"},
			}, nil
		},
	}
	result, _ := assertions.NewResourcesCreateComplete("my-stack").Run(ctx, client)
	assertFailed(t, result)
}

func TestLambdaExists_Pass(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionExistsFn: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	result, _ := assertions.NewLambdaExists("MyFunction").Run(ctx, client)
	assertPassed(t, result)
}

func TestLambdaExists_Fail_WhenNotFound(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionExistsFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
	}
	result, _ := assertions.NewLambdaExists("MyFunction").Run(ctx, client)
	assertFailed(t, result)
}

// ── Wiring ────────────────────────────────────────────────────────────────────

func TestLambdaHasEventSource_Pass_WhenEnabledESMExists(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaEventSourceMappingsFn: func(_ context.Context, _ string) ([]awsclient.EventSourceMapping, error) {
			return []awsclient.EventSourceMapping{
				{
					UUID:           "abc-123",
					FunctionARN:    "arn:aws:lambda:us-east-1:000:function:Worker",
					EventSourceARN: "arn:aws:sqs:us-east-1:000:JobQueue",
					State:          "Enabled",
				},
			}, nil
		},
	}
	result, _ := assertions.NewLambdaHasEventSource("Worker", "JobQueue").Run(ctx, client)
	assertPassed(t, result)
}

func TestLambdaHasEventSource_Fail_WhenNoESM(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaEventSourceMappingsFn: func(_ context.Context, _ string) ([]awsclient.EventSourceMapping, error) {
			return nil, nil
		},
	}
	result, _ := assertions.NewLambdaHasEventSource("Worker", "JobQueue").Run(ctx, client)
	assertFailed(t, result)
}

func TestLambdaHasEventSource_Fail_WhenESMDisabled(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaEventSourceMappingsFn: func(_ context.Context, _ string) ([]awsclient.EventSourceMapping, error) {
			return []awsclient.EventSourceMapping{
				{
					EventSourceARN: "arn:aws:sqs:us-east-1:000:JobQueue",
					State:          "Disabled",
				},
			}, nil
		},
	}
	result, _ := assertions.NewLambdaHasEventSource("Worker", "JobQueue").Run(ctx, client)
	assertFailed(t, result)
}

func TestESMEnabled_Pass_WhenProviderReturnsNoMappings(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaEventSourceMappingsFn: func(_ context.Context, _ string) ([]awsclient.EventSourceMapping, error) {
			return nil, nil
		},
	}

	result, err := assertions.NewESMEnabled("MyMapping", "uuid-123").Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
}

func TestAPIGatewayHasRoutes_Pass_WhenAllPresent(t *testing.T) {
	client := &awsclient.MockClient{
		APIGatewayV2RoutesFn: func(_ context.Context, _ string) ([]awsclient.APIRoute, error) {
			return []awsclient.APIRoute{
				{RouteKey: "GET /items", Target: "integrations/abc"},
				{RouteKey: "POST /items", Target: "integrations/def"},
			}, nil
		},
	}
	result, _ := assertions.NewAPIGatewayHasRoutes("api-id", []string{"GET /items", "POST /items"}).Run(ctx, client)
	assertPassed(t, result)
}

func TestAPIGatewayHasRoutes_Fail_WhenMissingRoute(t *testing.T) {
	client := &awsclient.MockClient{
		APIGatewayV2RoutesFn: func(_ context.Context, _ string) ([]awsclient.APIRoute, error) {
			return []awsclient.APIRoute{
				{RouteKey: "GET /items", Target: "integrations/abc"},
			}, nil
		},
	}
	result, _ := assertions.NewAPIGatewayHasRoutes("api-id", []string{"GET /items", "DELETE /items"}).Run(ctx, client)
	assertFailed(t, result)
}

// ── IAM ───────────────────────────────────────────────────────────────────────

func TestLambdaRoleHasPermission_Pass_WhenActionPresent(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionConfigFn: func(_ context.Context, _ string) (awsclient.LambdaConfig, error) {
			return awsclient.LambdaConfig{
				FunctionName: "Worker",
				RoleARN:      "arn:aws:iam::000:role/WorkerRole",
			}, nil
		},
		IAMRolePoliciesFn: func(_ context.Context, _ string) ([]awsclient.PolicyStatement, error) {
			return []awsclient.PolicyStatement{
				{
					Effect:    "Allow",
					Actions:   []string{"sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"},
					Resources: []string{"arn:aws:sqs:us-east-1:000:JobQueue"},
				},
			}, nil
		},
	}
	result, _ := assertions.NewLambdaRoleHasPermission("Worker", "sqs:ReceiveMessage", "JobQueue").Run(ctx, client)
	assertPassed(t, result)
}

func TestLambdaRoleHasPermission_Pass_WhenServiceWildcard(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionConfigFn: func(_ context.Context, _ string) (awsclient.LambdaConfig, error) {
			return awsclient.LambdaConfig{RoleARN: "arn:aws:iam::000:role/WorkerRole"}, nil
		},
		IAMRolePoliciesFn: func(_ context.Context, _ string) ([]awsclient.PolicyStatement, error) {
			return []awsclient.PolicyStatement{
				{
					Effect:    "Allow",
					Actions:   []string{"sqs:*"},
					Resources: []string{"*"},
				},
			}, nil
		},
	}
	result, _ := assertions.NewLambdaRoleHasPermission("Worker", "sqs:ReceiveMessage", "").Run(ctx, client)
	assertPassed(t, result)
}

func TestLambdaRoleHasPermission_Fail_WhenMissing(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionConfigFn: func(_ context.Context, _ string) (awsclient.LambdaConfig, error) {
			return awsclient.LambdaConfig{RoleARN: "arn:aws:iam::000:role/WorkerRole"}, nil
		},
		IAMRolePoliciesFn: func(_ context.Context, _ string) ([]awsclient.PolicyStatement, error) {
			return []awsclient.PolicyStatement{
				{
					Effect:    "Allow",
					Actions:   []string{"s3:GetObject"},
					Resources: []string{"*"},
				},
			}, nil
		},
	}
	result, _ := assertions.NewLambdaRoleHasPermission("Worker", "sqs:ReceiveMessage", "").Run(ctx, client)
	assertFailed(t, result)
}

func TestNoWildcardResourcePolicy_Pass_WhenNoWildcards(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionConfigFn: func(_ context.Context, _ string) (awsclient.LambdaConfig, error) {
			return awsclient.LambdaConfig{RoleARN: "arn:aws:iam::000:role/WorkerRole"}, nil
		},
		IAMRolePoliciesFn: func(_ context.Context, _ string) ([]awsclient.PolicyStatement, error) {
			return []awsclient.PolicyStatement{
				{
					Effect:    "Allow",
					Actions:   []string{"sqs:ReceiveMessage"},
					Resources: []string{"arn:aws:sqs:us-east-1:000:JobQueue"},
				},
			}, nil
		},
	}
	result, _ := assertions.NewNoWildcardResourcePolicy("Worker").Run(ctx, client)
	assertPassed(t, result)
}

func TestNoWildcardResourcePolicy_Fail_WhenWildcardPresent(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaFunctionConfigFn: func(_ context.Context, _ string) (awsclient.LambdaConfig, error) {
			return awsclient.LambdaConfig{RoleARN: "arn:aws:iam::000:role/WorkerRole"}, nil
		},
		IAMRolePoliciesFn: func(_ context.Context, _ string) ([]awsclient.PolicyStatement, error) {
			return []awsclient.PolicyStatement{
				{
					Effect:    "Allow",
					Actions:   []string{"dynamodb:*"},
					Resources: []string{"*"},
				},
			}, nil
		},
	}
	result, _ := assertions.NewNoWildcardResourcePolicy("Worker").Run(ctx, client)
	assertFailed(t, result)
}

// ── Behavioural ───────────────────────────────────────────────────────────────

func TestLambdaInvocable_Pass_WhenNoError(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaInvokeFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
			return []byte(`{"statusCode":200}`), nil
		},
	}
	result, _ := assertions.NewLambdaInvocable("MyFunction", nil).Run(ctx, client)
	assertPassed(t, result)
}

func TestLambdaInvocable_Fail_WhenInvocationErrors(t *testing.T) {
	client := &awsclient.MockClient{
		LambdaInvokeFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
			return nil, fmt.Errorf("function error: Runtime.ExitError")
		},
	}
	result, _ := assertions.NewLambdaInvocable("MyFunction", nil).Run(ctx, client)
	assertFailed(t, result)
}

func TestAPIGatewayHTTPCheck_Pass_WhenExpectedStatusMatches(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("want POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-Preflight-Test"); got != "true" {
			t.Fatalf("want X-Preflight-Test=true, got %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("accepted")),
			Header:     make(http.Header),
		}, nil
	})}

	client := &awsclient.MockClient{
		APIGatewayV2InvokeURLFn: func(_ context.Context, _ string) (string, error) {
			return "http://apigw.local", nil
		},
	}

	result, err := assertions.NewAPIGatewayHTTPCheck("api-id", http.MethodPost, "/jobs", []byte(`{"id":"job-123"}`), http.StatusAccepted).
		WithHeaders(map[string]string{"X-Preflight-Test": "true"}).
		WithHTTPClient(httpClient).
		Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
}

func TestAPIGatewayHTTPCheck_Fail_WhenStatusMismatches(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("bad request")),
			Header:     make(http.Header),
		}, nil
	})}

	client := &awsclient.MockClient{
		APIGatewayV2InvokeURLFn: func(_ context.Context, _ string) (string, error) {
			return "http://apigw.local", nil
		},
	}

	result, err := assertions.NewAPIGatewayHTTPCheck("api-id", http.MethodGet, "/health", nil, http.StatusOK).
		WithHTTPClient(httpClient).
		Run(ctx, client)
	assertNoError(t, err)
	assertFailed(t, result)
}

func TestAPIGatewayHTTPCheck_FallsBackToIntegrationLambdaOnInvokeURLFailure(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body: io.NopCloser(strings.NewReader(
				`<?xml version="1.0"?><Error><Code>InvalidArgument</Code><Message>POST requires either ?uploads</Message></Error>`,
			)),
			Header: make(http.Header),
		}, nil
	})}

	client := &awsclient.MockClient{
		APIGatewayV2InvokeURLFn: func(_ context.Context, _ string) (string, error) {
			return "http://apigw.local", nil
		},
		LambdaInvokeFn: func(_ context.Context, functionName string, payload []byte) ([]byte, error) {
			if functionName != "handler-fn" {
				t.Fatalf("want fallback function handler-fn, got %q", functionName)
			}
			if !strings.Contains(string(payload), `"rawPath":"/jobs"`) {
				t.Fatalf("want API Gateway event payload, got %s", string(payload))
			}
			return []byte(`{"statusCode":202}`), nil
		},
	}

	result, err := assertions.NewAPIGatewayHTTPCheck("api-id", http.MethodPost, "/jobs", []byte(`{"id":"job-123"}`), http.StatusAccepted).
		WithIntegrationLambda("handler-fn").
		WithHTTPClient(httpClient).
		Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
}

func TestSQSToLambdaToDynamoDB_Pass_WhenItemAppears(t *testing.T) {
	var sends int
	var polls int

	client := &awsclient.MockClient{
		SQSQueueURLFn: func(_ context.Context, queueName string) (string, error) {
			return "http://localhost:4566/000000000000/" + queueName, nil
		},
		SQSSendMessageFn: func(_ context.Context, queueURL, body string) error {
			sends++
			if queueURL == "" || body == "" {
				t.Fatalf("queue send should receive queue URL and body")
			}
			return nil
		},
		DynamoDBGetItemFn: func(_ context.Context, tableName string, key map[string]string) (map[string]string, error) {
			polls++
			if tableName != "JobsTable" {
				t.Fatalf("want table JobsTable, got %q", tableName)
			}
			if polls < 2 {
				return nil, nil
			}
			return map[string]string{"id": key["id"]}, nil
		},
	}

	result, err := assertions.NewSQSToLambdaToDynamoDB(
		"JobQueue",
		`{"id":"job-123"}`,
		"JobsTable",
		map[string]string{"id": "job-123"},
	).WithPolling(5*time.Millisecond, 50*time.Millisecond).Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
	if sends != 1 {
		t.Fatalf("want 1 SQS send, got %d", sends)
	}
}

func TestSQSToLambdaToDynamoDB_Fail_WhenSendFails(t *testing.T) {
	client := &awsclient.MockClient{
		SQSQueueURLFn: func(_ context.Context, queueName string) (string, error) {
			return "http://localhost:4566/000000000000/" + queueName, nil
		},
		SQSSendMessageFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("access denied")
		},
	}

	result, err := assertions.NewSQSToLambdaToDynamoDB(
		"JobQueue",
		`{"id":"job-123"}`,
		"JobsTable",
		map[string]string{"id": "job-123"},
	).WithPolling(5*time.Millisecond, 20*time.Millisecond).Run(ctx, client)
	assertNoError(t, err)
	assertFailed(t, result)
}

func TestSQSToLambdaToDynamoDB_Fail_WhenItemNeverAppears(t *testing.T) {
	client := &awsclient.MockClient{
		SQSQueueURLFn: func(_ context.Context, queueName string) (string, error) {
			return "http://localhost:4566/000000000000/" + queueName, nil
		},
		SQSSendMessageFn: func(_ context.Context, _, _ string) error {
			return nil
		},
		DynamoDBGetItemFn: func(_ context.Context, _ string, _ map[string]string) (map[string]string, error) {
			return nil, nil
		},
	}

	result, err := assertions.NewSQSToLambdaToDynamoDB(
		"JobQueue",
		`{"id":"job-123"}`,
		"JobsTable",
		map[string]string{"id": "job-123"},
	).WithPolling(5*time.Millisecond, 20*time.Millisecond).Run(ctx, client)
	assertNoError(t, err)
	assertFailed(t, result)
}

func TestSQSToLambdaToDynamoDB_FallsBackToConsumerLambdaWhenESMDoesNotDeliver(t *testing.T) {
	var invokes int

	client := &awsclient.MockClient{
		SQSQueueURLFn: func(_ context.Context, queueName string) (string, error) {
			return "http://localhost:4566/000000000000/" + queueName, nil
		},
		SQSSendMessageFn: func(_ context.Context, _, _ string) error {
			return nil
		},
		DynamoDBGetItemFn: func(_ context.Context, _ string, _ map[string]string) (map[string]string, error) {
			if invokes == 0 {
				return nil, nil
			}
			return map[string]string{"id": "job-123"}, nil
		},
		LambdaInvokeFn: func(_ context.Context, functionName string, payload []byte) ([]byte, error) {
			invokes++
			if functionName != "worker-fn" {
				t.Fatalf("want worker-fn, got %q", functionName)
			}
			if !strings.Contains(string(payload), `"Records"`) {
				t.Fatalf("want SQS event payload, got %s", string(payload))
			}
			return []byte(`{"processed":1}`), nil
		},
	}

	result, err := assertions.NewSQSToLambdaToDynamoDB(
		"JobQueue",
		`{"id":"job-123"}`,
		"JobsTable",
		map[string]string{"id": "job-123"},
	).WithConsumerLambda("worker-fn").WithPolling(5*time.Millisecond, 20*time.Millisecond).Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
	if invokes != 1 {
		t.Fatalf("want 1 fallback invoke, got %d", invokes)
	}
}

func TestSQSToLambdaToDynamoDB_RetriesConsumerFallbackWhenFirstInvokeFails(t *testing.T) {
	var invokes int

	client := &awsclient.MockClient{
		SQSQueueURLFn: func(_ context.Context, queueName string) (string, error) {
			return "http://localhost:4566/000000000000/" + queueName, nil
		},
		SQSSendMessageFn: func(_ context.Context, _, _ string) error {
			return nil
		},
		DynamoDBGetItemFn: func(_ context.Context, _ string, _ map[string]string) (map[string]string, error) {
			if invokes < 2 {
				return nil, nil
			}
			return map[string]string{"id": "job-123"}, nil
		},
		LambdaInvokeFn: func(_ context.Context, functionName string, payload []byte) ([]byte, error) {
			invokes++
			if functionName != "worker-fn" {
				t.Fatalf("want worker-fn, got %q", functionName)
			}
			if !strings.Contains(string(payload), `"Records"`) {
				t.Fatalf("want SQS event payload, got %s", string(payload))
			}
			if invokes == 1 {
				return nil, fmt.Errorf("function error: Unhandled")
			}
			return []byte(`{"processed":1}`), nil
		},
	}

	result, err := assertions.NewSQSToLambdaToDynamoDB(
		"JobQueue",
		`{"id":"job-123"}`,
		"JobsTable",
		map[string]string{"id": "job-123"},
	).WithConsumerLambda("worker-fn").WithPolling(5*time.Millisecond, 20*time.Millisecond).Run(ctx, client)
	assertNoError(t, err)
	assertPassed(t, result)
	if invokes != 2 {
		t.Fatalf("want 2 fallback invokes, got %d", invokes)
	}
}

func TestDiscoverSuiteFromResources_SkipsIncompleteResourcesWithoutPhysicalIDs(t *testing.T) {
	suite := assertions.DiscoverSuiteFromResources("my-stack", []awsclient.StackResource{
		{LogicalID: "JobsApiFE3EF639", PhysicalID: "api-123", Type: "AWS::ApiGatewayV2::Api", Status: "CREATE_COMPLETE"},
		{LogicalID: "JobsHandlerFunctionE48D6095", PhysicalID: "", Type: "AWS::Lambda::Function", Status: "CREATE_FAILED"},
		{LogicalID: "WorkerFunctionACE6A4B0", PhysicalID: "", Type: "AWS::Lambda::Function", Status: "CREATE_FAILED"},
		{LogicalID: "JobsTable1970BC16", PhysicalID: "jobs-table", Type: "AWS::DynamoDB::Table", Status: "CREATE_COMPLETE"},
	})

	assertContainsAssertionWithPrefix(t, suite, "cfn-stack-deployed")
	assertContainsAssertionWithPrefix(t, suite, "dynamodb-table-exists:jobs-table")
	assertNoAssertionWithPrefix(t, suite, "lambda-exists:")
	assertNoAssertionWithPrefix(t, suite, "lambda-invocable:")
	assertNoAssertionWithPrefix(t, suite, "esm-enabled:")
}

func TestDiscoverSuiteFromResources_DoesNotAssumeAllLambdasConsumeSQS(t *testing.T) {
	suite := assertions.DiscoverSuiteFromResources("my-stack", []awsclient.StackResource{
		{LogicalID: "JobQueue", PhysicalID: "http://localhost:4566/000000000000/JobQueue", Type: "AWS::SQS::Queue", Status: "CREATE_COMPLETE"},
		{LogicalID: "SenderFunction", PhysicalID: "sender-fn", Type: "AWS::Lambda::Function", Status: "CREATE_COMPLETE"},
		{LogicalID: "WorkerFunction", PhysicalID: "worker-fn", Type: "AWS::Lambda::Function", Status: "CREATE_COMPLETE"},
	})

	assertNoAssertionWithPrefix(t, suite, "iam-role-permission:")
}

func TestDiscoverSuiteFromResources_DoesNotAddGenericLambdaInvocations(t *testing.T) {
	suite := assertions.DiscoverSuiteFromResources("my-stack", []awsclient.StackResource{
		{LogicalID: "SenderFunction", PhysicalID: "sender-fn", Type: "AWS::Lambda::Function", Status: "CREATE_COMPLETE"},
	})

	assertNoAssertionWithPrefix(t, suite, "lambda-invocable:")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertPassed(t *testing.T, r assertions.Result) {
	t.Helper()
	if !r.Passed {
		t.Errorf("assertion %q should pass, but failed: %s", r.Name, r.Message)
	}
}

func assertFailed(t *testing.T, r assertions.Result) {
	t.Helper()
	if r.Passed {
		t.Errorf("assertion %q should fail, but passed: %s", r.Name, r.Message)
	}
}

func assertContainsAssertionWithPrefix(t *testing.T, suite []assertions.Assertion, want string) {
	t.Helper()
	for _, assertion := range suite {
		if assertion.Name() == want || strings.HasPrefix(assertion.Name(), want) {
			return
		}
	}
	t.Fatalf("assertion with prefix %q not found", want)
}

func assertNoAssertionWithPrefix(t *testing.T, suite []assertions.Assertion, prefix string) {
	t.Helper()
	for _, assertion := range suite {
		if strings.HasPrefix(assertion.Name(), prefix) {
			t.Fatalf("unexpected assertion %q found", assertion.Name())
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
