// Package aws provides an AWS SDK client pre-configured to target a local AWS
// emulator instead of real AWS. Assertion code is identical
// for local and real deployments — only the endpoint changes.
package aws

import "context"

// Client is the interface for AWS operations used by the assertion engine.
// Every method targets the configured emulator endpoint at construction time.
// Using an interface keeps assertions testable without a running emulator instance.
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

	// SQSSendMessage sends a message body to the queue URL.
	// Used by behavioural assertions that exercise queue-driven flows.
	SQSSendMessage(ctx context.Context, queueURL, body string) error

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

	// APIGatewayV2APIs returns all API Gateway v2 APIs visible in the emulator.
	APIGatewayV2APIs(ctx context.Context) ([]APIDetail, error)

	// APIGatewayV2InvokeURL returns the HTTP base URL for invoking the API.
	// Used by behavioural assertions to make real HTTP calls against the emulator.
	APIGatewayV2InvokeURL(ctx context.Context, apiID string) (string, error)

	// ─── DynamoDB ─────────────────────────────────────────────────────────────

	// DynamoDBTableExists reports whether the named table exists.
	DynamoDBTableExists(ctx context.Context, tableName string) (bool, error)

	// DynamoDBGetItem retrieves a single item by its string-valued key attributes.
	// key maps attribute name to string value (e.g. {"id": "msg-123"}).
	// Returns the item as a string map, or nil when not found.
	DynamoDBGetItem(ctx context.Context, tableName string, key map[string]string) (map[string]string, error)
}

// DefaultEndpoint is the default local emulator base URL.
const DefaultEndpoint = "http://localhost:4566"
