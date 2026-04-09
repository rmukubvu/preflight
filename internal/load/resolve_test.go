package load

import (
	"testing"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

func TestResolvePhysicalResourceID(t *testing.T) {
	t.Parallel()

	resources := []awsclient.StackResource{
		{LogicalID: "HttpApi", PhysicalID: "api-123", Type: "AWS::ApiGatewayV2::Api"},
		{LogicalID: "JobsHandler", PhysicalID: "jobs-handler-live", Type: "AWS::Lambda::Function"},
	}

	if got := resolvePhysicalResourceID(resources, "AWS::ApiGatewayV2::Api", "HttpApi"); got != "api-123" {
		t.Fatalf("expected logical id resolution, got %q", got)
	}
	if got := resolvePhysicalResourceID(resources, "AWS::Lambda::Function", "Jobs"); got != "jobs-handler-live" {
		t.Fatalf("expected logical prefix resolution, got %q", got)
	}
	if got := resolvePhysicalResourceID(resources, "AWS::Lambda::Function", "jobs-handler-live"); got != "jobs-handler-live" {
		t.Fatalf("expected physical id passthrough, got %q", got)
	}
	if got := resolvePhysicalResourceID(resources, "AWS::Lambda::Function", "missing"); got != "missing" {
		t.Fatalf("expected unknown ref passthrough, got %q", got)
	}
}

func TestResolveAPIRef(t *testing.T) {
	t.Parallel()

	resources := []awsclient.StackResource{
		{LogicalID: "HttpApi", PhysicalID: "api-123", Type: "AWS::ApiGatewayV2::Api"},
	}
	apis := []awsclient.APIDetail{
		{APIID: "api-123", Name: "fixture-http-api"},
		{APIID: "api-456", Name: "fixture-admin"},
	}

	if got := resolveAPIRef(resources, apis, "HttpApi"); got != "api-123" {
		t.Fatalf("expected stack resource resolution, got %q", got)
	}
	if got := resolveAPIRef(resources, apis, "fixture-admin"); got != "api-456" {
		t.Fatalf("expected api name resolution, got %q", got)
	}
	if got := resolveAPIRef(resources, apis, "fixture"); got != "api-123" {
		t.Fatalf("expected prefix name resolution, got %q", got)
	}
}
