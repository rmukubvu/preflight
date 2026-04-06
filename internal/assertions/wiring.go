package assertions

import (
	"context"
	"fmt"
	"strings"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// LambdaHasEventSource asserts that a Lambda function has an ESM connected
// to the expected event source ARN (or a source ARN containing the name).
type LambdaHasEventSource struct {
	functionName    string
	eventSourceName string // queue/stream name to match (partial ARN match)
}

func NewLambdaHasEventSource(functionName, eventSourceName string) *LambdaHasEventSource {
	return &LambdaHasEventSource{
		functionName:    functionName,
		eventSourceName: eventSourceName,
	}
}

func (a *LambdaHasEventSource) Name() string {
	return fmt.Sprintf("lambda-esm:%s→%s", a.functionName, a.eventSourceName)
}

func (a *LambdaHasEventSource) Category() Category { return CategoryWiring }

func (a *LambdaHasEventSource) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	esms, err := client.LambdaEventSourceMappings(ctx, a.functionName)
	if err != nil {
		return Result{}, err
	}

	for _, esm := range esms {
		if strings.Contains(esm.EventSourceARN, a.eventSourceName) && esm.State == "Enabled" {
			return Result{
				Name:       a.Name(),
				Category:   a.Category(),
				Passed:     true,
				Message:    fmt.Sprintf("Lambda %q has enabled ESM to %q", a.functionName, a.eventSourceName),
				ResourceID: a.functionName,
			}, nil
		}
	}

	// Check if ESM exists but is disabled.
	for _, esm := range esms {
		if strings.Contains(esm.EventSourceARN, a.eventSourceName) {
			return Result{
				Name:     a.Name(),
				Category: a.Category(),
				Passed:   false,
				Message:  fmt.Sprintf("Lambda %q has an ESM to %q but state is %q (expected Enabled)", a.functionName, a.eventSourceName, esm.State),
			}, nil
		}
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("Lambda %q has no event source mapping to %q", a.functionName, a.eventSourceName),
	}, nil
}

// APIGatewayHasRoutes asserts that an API Gateway V2 API has at least the
// expected routes (matched by RouteKey prefix, e.g. "GET /items").
type APIGatewayHasRoutes struct {
	apiID          string
	expectedRoutes []string
}

func NewAPIGatewayHasRoutes(apiID string, expectedRoutes []string) *APIGatewayHasRoutes {
	return &APIGatewayHasRoutes{apiID: apiID, expectedRoutes: expectedRoutes}
}

func (a *APIGatewayHasRoutes) Name() string {
	return "apigw-routes:" + a.apiID
}

func (a *APIGatewayHasRoutes) Category() Category { return CategoryWiring }

func (a *APIGatewayHasRoutes) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	routes, err := client.APIGatewayV2Routes(ctx, a.apiID)
	if err != nil {
		return Result{}, err
	}

	routeKeys := make(map[string]bool, len(routes))
	for _, r := range routes {
		routeKeys[r.RouteKey] = true
	}

	var missing []string
	for _, expected := range a.expectedRoutes {
		if !routeKeys[expected] {
			missing = append(missing, expected)
		}
	}

	if len(missing) == 0 {
		return Result{
			Name:       a.Name(),
			Category:   a.Category(),
			Passed:     true,
			Message:    fmt.Sprintf("API %q has all %d expected route(s)", a.apiID, len(a.expectedRoutes)),
			ResourceID: a.apiID,
		}, nil
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("API %q is missing route(s): %s", a.apiID, strings.Join(missing, ", ")),
	}, nil
}

// APIGatewayRouteHasIntegration asserts that every route in the API has
// a non-empty integration target (i.e. is wired to a Lambda function).
type APIGatewayRouteHasIntegration struct {
	apiID string
}

func NewAPIGatewayRouteHasIntegration(apiID string) *APIGatewayRouteHasIntegration {
	return &APIGatewayRouteHasIntegration{apiID: apiID}
}

func (a *APIGatewayRouteHasIntegration) Name() string {
	return "apigw-integrations:" + a.apiID
}

func (a *APIGatewayRouteHasIntegration) Category() Category { return CategoryWiring }

func (a *APIGatewayRouteHasIntegration) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	routes, err := client.APIGatewayV2Routes(ctx, a.apiID)
	if err != nil {
		return Result{}, err
	}

	var unwired []string
	for _, r := range routes {
		if r.Target == "" {
			unwired = append(unwired, r.RouteKey)
		}
	}

	if len(unwired) == 0 {
		return Result{
			Name:       a.Name(),
			Category:   a.Category(),
			Passed:     true,
			Message:    fmt.Sprintf("all %d route(s) in API %q have integrations", len(routes), a.apiID),
			ResourceID: a.apiID,
		}, nil
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("route(s) without integration in API %q: %s", a.apiID, strings.Join(unwired, ", ")),
	}, nil
}

// S3BucketHasNotificationTarget asserts that an S3 bucket has at least one
// notification pointing to either a Lambda or SQS target.
type S3BucketHasNotificationTarget struct {
	bucketName string
}

func NewS3BucketHasNotificationTarget(bucketName string) *S3BucketHasNotificationTarget {
	return &S3BucketHasNotificationTarget{bucketName: bucketName}
}

func (a *S3BucketHasNotificationTarget) Name() string {
	return "s3-notification-target:" + a.bucketName
}

func (a *S3BucketHasNotificationTarget) Category() Category { return CategoryWiring }

func (a *S3BucketHasNotificationTarget) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	notifications, err := client.S3BucketNotificationTargets(ctx, a.bucketName)
	if err != nil {
		return Result{}, err
	}

	for _, n := range notifications {
		if n.LambdaARN != "" || n.QueueARN != "" {
			return Result{
				Name:       a.Name(),
				Category:   a.Category(),
				Passed:     true,
				Message:    fmt.Sprintf("bucket %q has %d notification target(s)", a.bucketName, len(notifications)),
				ResourceID: a.bucketName,
			}, nil
		}
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("bucket %q has no notification targets configured", a.bucketName),
	}, nil
}
