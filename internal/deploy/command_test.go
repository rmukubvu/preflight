package deploy

import (
	"context"
	"testing"

	"github.com/rmukubvu/preflight/internal/assertions"
	"github.com/rmukubvu/preflight/internal/config"
	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

func TestBuildAssertionSuite_ResolvesConfiguredBehaviouralChecksFromStackResources(t *testing.T) {
	client := &awsclient.MockClient{
		CloudFormationStackResourcesFn: func(_ context.Context, _ string) ([]awsclient.StackResource, error) {
			return []awsclient.StackResource{
				{LogicalID: "JobsApi", PhysicalID: "api-123", Type: "AWS::ApiGatewayV2::Api", Status: "CREATE_COMPLETE"},
				{LogicalID: "JobQueue", PhysicalID: "http://localhost:4566/000000000000/JobQueue", Type: "AWS::SQS::Queue", Status: "CREATE_COMPLETE"},
				{LogicalID: "JobsTable", PhysicalID: "jobs-table", Type: "AWS::DynamoDB::Table", Status: "CREATE_COMPLETE"},
				{LogicalID: "WorkerFunction", PhysicalID: "worker-fn", Type: "AWS::Lambda::Function", Status: "CREATE_COMPLETE"},
			}, nil
		},
		APIGatewayV2APIsFn: func(_ context.Context) ([]awsclient.APIDetail, error) {
			return []awsclient.APIDetail{
				{APIID: "api-123", Name: "JobsApi"},
			}, nil
		},
	}

	rc := runConfig{
		stackName: "my-stack",
		cfg: config.Config{
			Assertions: config.AssertionsConfig{
				Behavioural: config.BehaviouralConfig{
					HTTP: []config.HTTPCheckConfig{
						{
							API:                 "JobsApi",
							IntegrationFunction: "WorkerFunction",
							Method:              "post",
							Path:                "jobs",
							ExpectedStatus:      202,
						},
					},
					SQSToLambdaToDynamo: []config.SQSToLambdaToDynamoDBConfig{
						{
							Queue:            "JobQueue",
							Table:            "JobsTable",
							ConsumerFunction: "WorkerFunction",
							MessageBody:      `{"id":"job-123"}`,
							ExpectedKey:      map[string]string{"id": "job-123"},
						},
					},
				},
			},
		},
	}

	suite, err := buildAssertionSuite(context.Background(), client, rc)
	if err != nil {
		t.Fatalf("buildAssertionSuite: %v", err)
	}

	assertContainsAssertionName(t, suite, "cfn-stack-deployed")
	assertContainsAssertionName(t, suite, "apigw-http:POST api-123/jobs")
	assertContainsAssertionName(t, suite, "e2e-sqs-lambda-ddb:JobQueue→jobs-table")
}

func TestBuildConfiguredBehaviouralAssertions_UsesLiteralRefsWithoutStackDiscovery(t *testing.T) {
	suite := buildConfiguredBehaviouralAssertions(
		config.BehaviouralConfig{
			HTTP: []config.HTTPCheckConfig{
				{
					API:                 "api-789",
					IntegrationFunction: "handler-fn",
					Method:              "GET",
					Path:                "/health",
					ExpectedStatus:      200,
				},
			},
			SQSToLambdaToDynamo: []config.SQSToLambdaToDynamoDBConfig{
				{
					Queue:            "jobs-queue",
					Table:            "jobs-table",
					ConsumerFunction: "worker-fn",
					MessageBody:      `{"id":"job-123"}`,
					ExpectedKey:      map[string]string{"id": "job-123"},
				},
			},
		},
		[]awsclient.StackResource{
			{LogicalID: "handler-fn", PhysicalID: "handler-fn", Type: "AWS::Lambda::Function"},
			{LogicalID: "worker-fn", PhysicalID: "worker-fn", Type: "AWS::Lambda::Function"},
		},
		nil,
	)

	assertContainsAssertionName(t, suite, "apigw-http:GET api-789/health")
	assertContainsAssertionName(t, suite, "e2e-sqs-lambda-ddb:jobs-queue→jobs-table")
}

func TestBuildConfiguredBehaviouralAssertions_ResolvesAPIsByGeneratedLogicalIDAndName(t *testing.T) {
	suite := buildConfiguredBehaviouralAssertions(
		config.BehaviouralConfig{
			HTTP: []config.HTTPCheckConfig{
				{
					API:            "JobsApi",
					Method:         "POST",
					Path:           "/jobs",
					ExpectedStatus: 202,
				},
			},
		},
		[]awsclient.StackResource{
			{LogicalID: "JobsApiFE3EF639", PhysicalID: "JobsApiFE3EF639", Type: "AWS::ApiGatewayV2::Api"},
		},
		[]awsclient.APIDetail{
			{APIID: "real-api-id", Name: "JobsApi"},
		},
	)

	assertContainsAssertionName(t, suite, "apigw-http:POST real-api-id/jobs")
}

func assertContainsAssertionName(t *testing.T, suite []assertions.Assertion, want string) {
	t.Helper()
	for _, assertion := range suite {
		if assertion.Name() == want {
			return
		}
	}
	t.Fatalf("assertion %q not found", want)
}
