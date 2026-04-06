package assertions

import (
	"context"
	"fmt"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// LambdaInvocable asserts that a Lambda function can be invoked without error.
// It sends a minimal test payload and checks for a non-error response.
type LambdaInvocable struct {
	functionName string
	payload      []byte
}

// NewLambdaInvocable constructs a LambdaInvocable assertion.
// If payload is nil, a minimal JSON event is used.
func NewLambdaInvocable(functionName string, payload []byte) *LambdaInvocable {
	if payload == nil {
		payload = []byte(`{"preflight":"ping"}`)
	}
	return &LambdaInvocable{functionName: functionName, payload: payload}
}

func (a *LambdaInvocable) Name() string     { return "lambda-invocable:" + a.functionName }
func (a *LambdaInvocable) Category() Category { return CategoryBehavioural }

func (a *LambdaInvocable) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	resp, err := client.LambdaInvoke(ctx, a.functionName, a.payload)
	if err != nil {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  fmt.Sprintf("Lambda %q invocation failed: %v", a.functionName, err),
		}, nil
	}

	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     true,
		Message:    fmt.Sprintf("Lambda %q invoked successfully (%d bytes returned)", a.functionName, len(resp)),
		ResourceID: a.functionName,
	}, nil
}
