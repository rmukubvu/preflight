package aws

// StackResource is a resource declared in a CloudFormation stack.
type StackResource struct {
	LogicalID  string
	PhysicalID string
	Type       string // e.g. "AWS::Lambda::Function"
	Status     string // e.g. "CREATE_COMPLETE"
}

// LambdaConfig holds the essential configuration of a Lambda function.
type LambdaConfig struct {
	FunctionName string
	FunctionARN  string
	RoleARN      string
	Handler      string
	Runtime      string
	Environment  map[string]string
}

// EventSourceMapping describes a Lambda trigger from an event source.
type EventSourceMapping struct {
	UUID           string
	FunctionARN    string
	EventSourceARN string
	State          string // "Enabled" | "Disabled" | "Creating" | "Deleting"
}

// PolicyStatement is a single Allow/Deny statement from an IAM policy document.
type PolicyStatement struct {
	Effect    string   // "Allow" or "Deny"
	Actions   []string // e.g. ["sqs:ReceiveMessage", "sqs:DeleteMessage"]
	Resources []string // e.g. ["arn:aws:sqs:us-east-1:000000000000:MyQueue"]
}

// S3Notification is an event notification configured on an S3 bucket.
type S3Notification struct {
	Events    []string // e.g. ["s3:ObjectCreated:*"]
	LambdaARN string   // target Lambda ARN, if applicable
	QueueARN  string   // target SQS ARN, if applicable
}

// APIRoute is a route in an API Gateway V2 API.
type APIRoute struct {
	RouteID  string
	RouteKey string // e.g. "GET /items" or "$default"
	Target   string // e.g. "integrations/abc123"
}

// APIDetail holds metadata about an API Gateway V2 API.
type APIDetail struct {
	APIID       string
	Name        string
	Protocol    string // "HTTP" or "WEBSOCKET"
	Endpoint    string // the invoke URL base
}
