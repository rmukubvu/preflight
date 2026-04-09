// Package assertions contains the parallel assertion engine and individual
// infrastructure checks. After a local emulator deployment, the engine runs
// all registered assertions concurrently and aggregates the results.
package assertions

import (
	"context"

	"github.com/rmukubvu/preflight/pkg/aws"
)

// Category groups related assertions for reporting.
type Category string

const (
	CategoryStructural  Category = "STRUCTURAL"
	CategoryWiring      Category = "WIRING"
	CategoryIAM         Category = "IAM"
	CategoryBehavioural Category = "BEHAVIOURAL"
)

// Result is the outcome of a single assertion run.
type Result struct {
	// Name is the assertion's short identifier, e.g. "s3-bucket-exists".
	Name string

	// Category classifies the assertion for grouped reporting.
	Category Category

	// Passed is true when the assertion's condition is met.
	Passed bool

	// Message describes what was checked and, on failure, what was found.
	Message string

	// ResourceID is the AWS resource involved, if applicable.
	ResourceID string
}

// Assertion is the interface for a single infrastructure check.
// All implementations must be safe to call concurrently.
type Assertion interface {
	// Name returns the short identifier used in reports and diagnosis requests.
	Name() string

	// Category returns the assertion's classification.
	Category() Category

	// Run executes the check against the emulator-backed AWS client.
	Run(ctx context.Context, client aws.Client) (Result, error)
}
