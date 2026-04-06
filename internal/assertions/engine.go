package assertions

import (
	"context"
	"sync"

	"github.com/rmukubvu/preflight/pkg/aws"
)

// Engine runs a set of assertions in parallel and collects their results.
type Engine struct {
	assertions []Assertion
}

// NewEngine constructs an Engine with the given assertions.
func NewEngine(assertions []Assertion) *Engine {
	return &Engine{assertions: assertions}
}

// RunAll executes all registered assertions concurrently against client.
// It waits for all goroutines to finish before returning, even if ctx is
// cancelled — partial results are always returned alongside any error.
func (e *Engine) RunAll(ctx context.Context, client aws.Client) ([]Result, error) {
	results := make([]Result, len(e.assertions))
	errs := make([]error, len(e.assertions))

	var wg sync.WaitGroup
	wg.Add(len(e.assertions))

	for i, a := range e.assertions {
		i, a := i, a // capture loop vars
		go func() {
			defer wg.Done()
			results[i], errs[i] = a.Run(ctx, client)
		}()
	}

	wg.Wait()

	// Collect the first non-nil error to surface to the caller.
	// All results are returned regardless.
	var firstErr error
	for _, err := range errs {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}
