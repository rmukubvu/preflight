// Package aws provides an AWS SDK client pre-configured to target Floci
// (the local AWS emulator) instead of real AWS. Assertion code is identical
// for local and real deployments — only the endpoint changes.
package aws

import "context"

// Client is the interface for AWS operations used by the assertion engine.
// Every method targets the Floci endpoint at construction time.
// Using an interface keeps assertions testable without a running Floci instance.
type Client interface {
	// ─── CloudFormation ──────────────────────────────────────────────────────

	// CloudFormationStackStatus returns the stack's status string,
	// e.g. "CREATE_COMPLETE", "ROLLBACK_COMPLETE", or "DOES_NOT_EXIST".
	CloudFormationStackStatus(ctx context.Context, stackName string) (string, error)

	// CloudFormationStackResources returns all resources in the named stack.
	CloudFormationStackResources(ctx context.Context, stackName string) ([]StackResource, error)

	// ─── Lambda ───────────────────────────────────────────────────────────────

	// LambdaFunctionExists reports whether the named function is deployed.
	LambdaFunctionExists(ctx context.Context, functionName string) (bool, error)

	// LambdaFunctionConfig returns the configuration of the named function.
	LambdaFunctionConfig(ctx context.Context, functionName string) (LambdaConfig, error)

	// LambdaEventSourceMappings returns all ESMs attached to the named function.
	LambdaEventSourceMappings(ctx context.Context, functionName string) ([]EventSourceMapping, error)

	// LambdaInvoke synchronously invokes the named function with payload
	// and returns the response body. Used by behavioural assertions.
	LambdaInvoke(ctx context.Context, functionName string, payload []byte) ([]byte, error)

	// ─── SQS ──────────────────────────────────────────────────────────────────

	// SQSQueueExists reports whether a queue with the given name exists.
	SQSQueueExists(ctx context.Context, queueName string) (bool, error)

	// SQSQueueURL returns the URL of the named queue.
	SQSQueueURL(ctx context.Context, queueName string) (string, error)

	// ─── S3 ───────────────────────────────────────────────────────────────────

	// S3BucketExists reports whether the named bucket exists.
	S3BucketExists(ctx context.Context, bucket string) (bool, error)

	// S3BucketNotificationTargets returns the notification configuration
	// for the named bucket.
	S3BucketNotificationTargets(ctx context.Context, bucket string) ([]S3Notification, error)

	// ─── IAM ──────────────────────────────────────────────────────────────────

	// IAMRoleExists reports whether the named IAM role exists.
	IAMRoleExists(ctx context.Context, roleName string) (bool, error)

	// IAMRolePolicies returns all policy statements attached to the role
	// (both inline and attached managed policies, flattened).
	IAMRolePolicies(ctx context.Context, roleName string) ([]PolicyStatement, error)

	// ─── API Gateway V2 ───────────────────────────────────────────────────────

	// APIGatewayV2Routes returns all routes in the named HTTP API.
	APIGatewayV2Routes(ctx context.Context, apiID string) ([]APIRoute, error)
}

// FlociEndpoint is the default Floci base URL.
const FlociEndpoint = "http://localhost:4566"
