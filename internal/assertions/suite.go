package assertions

import (
	"context"
	"strings"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// DiscoverSuite queries a deployed CloudFormation stack and builds the full
// assertion suite automatically — no manual configuration required.
//
// It reads every resource in the stack, groups them by type, and constructs
// the appropriate Structural / Wiring / IAM / Behavioural checks.
func DiscoverSuite(ctx context.Context, client awsclient.Client, stackName string) ([]Assertion, error) {
	resources, err := client.CloudFormationStackResources(ctx, stackName)
	if err != nil {
		return nil, err
	}

	return DiscoverSuiteFromResources(stackName, resources), nil
}

// DiscoverSuiteFromResources builds the generic assertion suite from stack
// resources that have already been listed by the caller.
func DiscoverSuiteFromResources(stackName string, resources []awsclient.StackResource) []Assertion {
	// Always start with the stack-level checks.
	suite := []Assertion{
		NewStackDeployed(stackName),
		NewResourcesCreateComplete(stackName),
	}

	completeResources := successfulResources(resources)

	// Group resources by their CloudFormation type.
	byType := groupByType(completeResources)

	// ── Structural assertions ─────────────────────────────────────────────────

	for _, r := range byType["AWS::Lambda::Function"] {
		suite = append(suite, NewLambdaExists(r.PhysicalID))
	}
	for _, r := range byType["AWS::SQS::Queue"] {
		suite = append(suite, NewSQSQueueExists(queueName(r.PhysicalID)))
	}
	for _, r := range byType["AWS::S3::Bucket"] {
		suite = append(suite, NewS3BucketExists(r.PhysicalID))
	}
	for _, r := range byType["AWS::DynamoDB::Table"] {
		suite = append(suite, NewDynamoDBTableExists(r.PhysicalID))
	}

	// ── Wiring assertions ─────────────────────────────────────────────────────

	// For each Lambda ESM, verify the mapping is enabled.
	if len(byType["AWS::Lambda::Function"]) > 0 {
		for _, r := range byType["AWS::Lambda::EventSourceMapping"] {
			// PhysicalID is the ESM UUID; we discover the function+source via the API.
			suite = append(suite, NewESMEnabled(r.LogicalID, r.PhysicalID))
		}
	}

	// For each API Gateway, verify all routes have integrations.
	for _, r := range byType["AWS::ApiGatewayV2::Api"] {
		suite = append(suite, NewAPIGatewayRouteHasIntegration(r.PhysicalID))
	}

	// ── IAM assertions ────────────────────────────────────────────────────────

	for _, r := range byType["AWS::Lambda::Function"] {
		suite = append(suite, NewNoWildcardResourcePolicy(r.PhysicalID))
	}

	return suite
}

// groupByType indexes resources by their CloudFormation type.
func groupByType(resources []awsclient.StackResource) map[string][]awsclient.StackResource {
	m := make(map[string][]awsclient.StackResource)
	for _, r := range resources {
		m[r.Type] = append(m[r.Type], r)
	}
	return m
}

// queueName extracts the queue name from a full SQS queue URL or ARN.
// Emulator CloudFormation PhysicalIDs for SQS can be either format.
func queueName(physicalID string) string {
	// URL: http://localhost:4566/000000000000/MyQueue → "MyQueue"
	// ARN: arn:aws:sqs:us-east-1:000000000000:MyQueue → "MyQueue"
	parts := strings.Split(physicalID, "/")
	if last := parts[len(parts)-1]; last != "" {
		return last
	}
	arnParts := strings.Split(physicalID, ":")
	return arnParts[len(arnParts)-1]
}

func successfulResources(resources []awsclient.StackResource) []awsclient.StackResource {
	filtered := make([]awsclient.StackResource, 0, len(resources))
	for _, resource := range resources {
		if !isCompleteStatus(resource.Status) || resource.PhysicalID == "" {
			continue
		}
		filtered = append(filtered, resource)
	}
	return filtered
}
