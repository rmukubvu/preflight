package assertions_test

import (
	"context"
	"fmt"
	"testing"

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
