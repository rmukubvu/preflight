package aws

import "testing"

func TestAPIGatewayV2InvokeURLFromDetails(t *testing.T) {
	apis := []APIDetail{
		{
			APIID:    "api-123",
			Name:     "jobs-api",
			Endpoint: "https://api-123.execute-api.us-east-1.amazonaws.com",
		},
	}

	if got := apiGatewayV2InvokeURLFromDetails(apis, "api-123"); got != "https://api-123.execute-api.us-east-1.amazonaws.com" {
		t.Fatalf("want endpoint by id, got %q", got)
	}
	if got := apiGatewayV2InvokeURLFromDetails(apis, "jobs-api"); got != "https://api-123.execute-api.us-east-1.amazonaws.com" {
		t.Fatalf("want endpoint by name, got %q", got)
	}
	if got := apiGatewayV2InvokeURLFromDetails(apis, "missing"); got != "" {
		t.Fatalf("want empty string for missing API, got %q", got)
	}
}
