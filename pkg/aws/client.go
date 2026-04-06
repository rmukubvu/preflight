// Package aws provides an AWS SDK client pre-configured to target Floci
// instead of real AWS. All service calls go to localhost:4566 (or the
// configured port), making assertion code identical for local and real runs.
package aws

import "context"

// Client is the interface for AWS operations used by the assertion engine.
// Using an interface keeps assertions testable without a running Floci instance.
type Client interface {
	// S3BucketExists returns true if the named bucket exists in Floci.
	S3BucketExists(ctx context.Context, bucket string) (bool, error)

	// IAMRoleExists returns true if the named IAM role exists.
	IAMRoleExists(ctx context.Context, roleName string) (bool, error)

	// LambdaFunctionExists returns true if the named Lambda function exists.
	LambdaFunctionExists(ctx context.Context, functionName string) (bool, error)

	// SQSQueueExists returns true if a queue with the given name exists.
	SQSQueueExists(ctx context.Context, queueName string) (bool, error)
}

// FlociEndpoint is the default Floci base URL.
const FlociEndpoint = "http://localhost:4566"
