package assertions

import (
	"context"
	"fmt"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// StackDeployed asserts that the CloudFormation stack reached CREATE_COMPLETE.
type StackDeployed struct {
	stackName string
}

func NewStackDeployed(stackName string) *StackDeployed {
	return &StackDeployed{stackName: stackName}
}

func (a *StackDeployed) Name() string     { return "cfn-stack-deployed" }
func (a *StackDeployed) Category() Category { return CategoryStructural }

func (a *StackDeployed) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	status, err := client.CloudFormationStackStatus(ctx, a.stackName)
	if err != nil {
		return Result{}, err
	}
	passed := status == "CREATE_COMPLETE" || status == "UPDATE_COMPLETE"
	msg := fmt.Sprintf("stack %q status: %s", a.stackName, status)
	if passed {
		msg = fmt.Sprintf("stack %q is %s", a.stackName, status)
	}
	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     passed,
		Message:    msg,
		ResourceID: a.stackName,
	}, nil
}

// ResourcesCreateComplete asserts that every resource in the stack
// reached *_COMPLETE status (no failures or in-progress resources).
type ResourcesCreateComplete struct {
	stackName string
}

func NewResourcesCreateComplete(stackName string) *ResourcesCreateComplete {
	return &ResourcesCreateComplete{stackName: stackName}
}

func (a *ResourcesCreateComplete) Name() string     { return "cfn-all-resources-complete" }
func (a *ResourcesCreateComplete) Category() Category { return CategoryStructural }

func (a *ResourcesCreateComplete) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	resources, err := client.CloudFormationStackResources(ctx, a.stackName)
	if err != nil {
		return Result{}, err
	}

	var failed []string
	for _, r := range resources {
		if !isCompleteStatus(r.Status) {
			failed = append(failed, fmt.Sprintf("%s (%s) → %s", r.LogicalID, r.Type, r.Status))
		}
	}

	if len(failed) == 0 {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   true,
			Message:  fmt.Sprintf("all %d resources in %q are complete", len(resources), a.stackName),
		}, nil
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("%d resource(s) not complete: %v", len(failed), failed),
	}, nil
}

// LambdaExists asserts that a specific Lambda function was created.
type LambdaExists struct {
	functionName string
}

func NewLambdaExists(functionName string) *LambdaExists {
	return &LambdaExists{functionName: functionName}
}

func (a *LambdaExists) Name() string     { return "lambda-exists:" + a.functionName }
func (a *LambdaExists) Category() Category { return CategoryStructural }

func (a *LambdaExists) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	exists, err := client.LambdaFunctionExists(ctx, a.functionName)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     exists,
		Message:    fmt.Sprintf("Lambda function %q exists: %v", a.functionName, exists),
		ResourceID: a.functionName,
	}, nil
}

// SQSQueueExists asserts that a specific SQS queue was created.
type SQSQueueExists struct {
	queueName string
}

func NewSQSQueueExists(queueName string) *SQSQueueExists {
	return &SQSQueueExists{queueName: queueName}
}

func (a *SQSQueueExists) Name() string     { return "sqs-queue-exists:" + a.queueName }
func (a *SQSQueueExists) Category() Category { return CategoryStructural }

func (a *SQSQueueExists) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	exists, err := client.SQSQueueExists(ctx, a.queueName)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     exists,
		Message:    fmt.Sprintf("SQS queue %q exists: %v", a.queueName, exists),
		ResourceID: a.queueName,
	}, nil
}

// S3BucketExists asserts that a specific S3 bucket was created.
type S3BucketExists struct {
	bucketName string
}

func NewS3BucketExists(bucketName string) *S3BucketExists {
	return &S3BucketExists{bucketName: bucketName}
}

func (a *S3BucketExists) Name() string     { return "s3-bucket-exists:" + a.bucketName }
func (a *S3BucketExists) Category() Category { return CategoryStructural }

func (a *S3BucketExists) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	exists, err := client.S3BucketExists(ctx, a.bucketName)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     exists,
		Message:    fmt.Sprintf("S3 bucket %q exists: %v", a.bucketName, exists),
		ResourceID: a.bucketName,
	}, nil
}

// isCompleteStatus returns true when a CloudFormation resource status
// represents a successful terminal state.
func isCompleteStatus(status string) bool {
	switch status {
	case "CREATE_COMPLETE", "UPDATE_COMPLETE", "IMPORT_COMPLETE":
		return true
	}
	return false
}
