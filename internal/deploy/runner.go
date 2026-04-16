// Package deploy wraps CDK and Terraform deploy commands, redirecting them
// to the local emulator instead of real AWS.
package deploy

import (
	"context"

	"github.com/rmukubvu/preflight/internal/stack"
)

// Runner deploys an IaC stack.
type Runner interface {
	// Deploy runs the deployment against the emulator endpoint.
	Deploy(ctx context.Context) error

	// StackName returns the identifier for the deployed stack.
	StackName() string
}

// StackType is retained as an alias for deploy-facing code while stack type
// detection lives in the shared internal/stack package.
type StackType = stack.Type

const (
	StackTypeCDK       = stack.TypeCDK
	StackTypePulumi    = stack.TypePulumi
	StackTypeTerraform = stack.TypeTerraform
	StackTypeUnknown   = stack.TypeUnknown
)

// DetectStackType preserves the original deploy package entrypoint.
func DetectStackType(dir string) StackType {
	return stack.Detect(dir)
}
