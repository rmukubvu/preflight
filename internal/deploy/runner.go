// Package deploy wraps CDK and Terraform deploy commands, redirecting them
// to Floci instead of real AWS.
package deploy

import "context"

// Runner deploys an IaC stack.
type Runner interface {
	// Deploy runs the deployment against the Floci endpoint.
	Deploy(ctx context.Context) error

	// StackName returns the identifier for the deployed stack.
	StackName() string
}
