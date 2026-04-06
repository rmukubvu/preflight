package assertions

import (
	"context"
	"fmt"
	"strings"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// LambdaRoleHasPermission asserts that a Lambda function's execution role
// includes the specified IAM action on the named resource.
type LambdaRoleHasPermission struct {
	functionName   string
	requiredAction string // e.g. "sqs:ReceiveMessage"
	resourceHint   string // partial ARN or name to match in the resource list
}

func NewLambdaRoleHasPermission(functionName, action, resourceHint string) *LambdaRoleHasPermission {
	return &LambdaRoleHasPermission{
		functionName:   functionName,
		requiredAction: action,
		resourceHint:   resourceHint,
	}
}

func (a *LambdaRoleHasPermission) Name() string {
	return fmt.Sprintf("iam-role-permission:%s/%s", a.functionName, a.requiredAction)
}

func (a *LambdaRoleHasPermission) Category() Category { return CategoryIAM }

func (a *LambdaRoleHasPermission) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	cfg, err := client.LambdaFunctionConfig(ctx, a.functionName)
	if err != nil {
		return Result{}, fmt.Errorf("getting function config: %w", err)
	}

	roleName := roleNameFromARN(cfg.RoleARN)
	stmts, err := client.IAMRolePolicies(ctx, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("getting role policies: %w", err)
	}

	for _, stmt := range stmts {
		if stmt.Effect != "Allow" {
			continue
		}
		if !actionMatches(stmt.Actions, a.requiredAction) {
			continue
		}
		if a.resourceHint == "" || resourceMatches(stmt.Resources, a.resourceHint) {
			return Result{
				Name:       a.Name(),
				Category:   a.Category(),
				Passed:     true,
				Message:    fmt.Sprintf("role %q allows %q", roleName, a.requiredAction),
				ResourceID: a.functionName,
			}, nil
		}
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("Lambda %q execution role %q missing %q permission", a.functionName, roleName, a.requiredAction),
	}, nil
}

// NoWildcardResourcePolicy asserts that a Lambda function's execution role
// does not have any policy statements with a wildcard (*) resource.
// Wildcard resource policies are a common security misconfiguration.
type NoWildcardResourcePolicy struct {
	functionName string
}

func NewNoWildcardResourcePolicy(functionName string) *NoWildcardResourcePolicy {
	return &NoWildcardResourcePolicy{functionName: functionName}
}

func (a *NoWildcardResourcePolicy) Name() string {
	return "iam-no-wildcard-resource:" + a.functionName
}

func (a *NoWildcardResourcePolicy) Category() Category { return CategoryIAM }

func (a *NoWildcardResourcePolicy) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	cfg, err := client.LambdaFunctionConfig(ctx, a.functionName)
	if err != nil {
		return Result{}, fmt.Errorf("getting function config: %w", err)
	}

	roleName := roleNameFromARN(cfg.RoleARN)
	stmts, err := client.IAMRolePolicies(ctx, roleName)
	if err != nil {
		return Result{}, fmt.Errorf("getting role policies: %w", err)
	}

	var wildcards []string
	for _, stmt := range stmts {
		if stmt.Effect != "Allow" {
			continue
		}
		for _, r := range stmt.Resources {
			if r == "*" {
				wildcards = append(wildcards, fmt.Sprintf("Action=%v", stmt.Actions))
				break
			}
		}
	}

	if len(wildcards) == 0 {
		return Result{
			Name:       a.Name(),
			Category:   a.Category(),
			Passed:     true,
			Message:    fmt.Sprintf("role %q has no wildcard resource policies", roleName),
			ResourceID: a.functionName,
		}, nil
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  fmt.Sprintf("role %q has wildcard (*) resource policies: %s", roleName, strings.Join(wildcards, "; ")),
	}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// roleNameFromARN extracts the role name from an ARN like
// "arn:aws:iam::123456789012:role/MyRole".
func roleNameFromARN(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return arn
}

// actionMatches returns true if the given action is in the actions list,
// accounting for wildcard patterns like "sqs:*" or "*".
func actionMatches(actions []string, want string) bool {
	want = strings.ToLower(want)
	for _, a := range actions {
		a = strings.ToLower(a)
		if a == "*" || a == want {
			return true
		}
		// Handle service wildcards like "sqs:*"
		if strings.HasSuffix(a, ":*") {
			prefix := strings.TrimSuffix(a, "*")
			if strings.HasPrefix(want, prefix) {
				return true
			}
		}
	}
	return false
}

// resourceMatches returns true if any resource in the list contains hint
// or is a wildcard.
func resourceMatches(resources []string, hint string) bool {
	for _, r := range resources {
		if r == "*" || strings.Contains(r, hint) {
			return true
		}
	}
	return false
}
