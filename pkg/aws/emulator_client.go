package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/smithy-go"
)

// EmulatorClient implements Client, pointing all service calls at the local
// AWS emulator. Credentials are fixed test values because local emulators
// generally do not enforce authentication.
type EmulatorClient struct {
	cfn     *cloudformation.Client
	lambda  *awslambda.Client
	sqsSvc  *sqs.Client
	s3Svc   *s3.Client
	iamSvc  *iam.Client
	apigw   *apigatewayv2.Client
	ddb     *dynamodb.Client
	baseURL string // e.g. "http://localhost:4566" — used to build invoke URLs
	kind    string
}

// NewEmulatorClient constructs an EmulatorClient targeting baseURL
// (e.g. "http://localhost:4566").
func NewEmulatorClient(baseURL, kind string) (*EmulatorClient, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid emulator base URL %q: %w", baseURL, err)
	}

	// Local emulators do not enforce authentication; use fixed test credentials.
	creds := credentials.NewStaticCredentialsProvider("test", "test", "")

	endpoint := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, _ ...any) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               baseURL,
				HostnameImmutable: true,
				SigningRegion:     region,
			}, nil
		},
	)

	base := aws.Config{
		Region:                      "us-east-1",
		Credentials:                 creds,
		EndpointResolverWithOptions: endpoint, //nolint:staticcheck // required for emulator compatibility
	}

	return &EmulatorClient{
		cfn:     cloudformation.NewFromConfig(base),
		lambda:  awslambda.NewFromConfig(base),
		sqsSvc:  sqs.NewFromConfig(base),
		s3Svc:   s3.NewFromConfig(base),
		iamSvc:  iam.NewFromConfig(base),
		apigw:   apigatewayv2.NewFromConfig(base),
		ddb:     dynamodb.NewFromConfig(base),
		baseURL: baseURL,
		kind:    kind,
	}, nil
}

var _ Client = (*EmulatorClient)(nil) // compile-time interface check

// ─── CloudFormation ──────────────────────────────────────────────────────────

func (c *EmulatorClient) CloudFormationStackStatus(ctx context.Context, stackName string) (string, error) {
	out, err := c.cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ValidationError" {
			return "DOES_NOT_EXIST", nil
		}
		return "", fmt.Errorf("describing stack %q: %w", stackName, err)
	}
	if len(out.Stacks) == 0 {
		return "DOES_NOT_EXIST", nil
	}
	return string(out.Stacks[0].StackStatus), nil
}

func (c *EmulatorClient) CloudFormationStackResources(ctx context.Context, stackName string) ([]StackResource, error) {
	out, err := c.cfn.ListStackResources(ctx, &cloudformation.ListStackResourcesInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing resources for stack %q: %w", stackName, err)
	}

	resources := make([]StackResource, 0, len(out.StackResourceSummaries))
	for _, r := range out.StackResourceSummaries {
		resources = append(resources, StackResource{
			LogicalID:  aws.ToString(r.LogicalResourceId),
			PhysicalID: aws.ToString(r.PhysicalResourceId),
			Type:       aws.ToString(r.ResourceType),
			Status:     string(r.ResourceStatus),
		})
	}
	return resources, nil
}

// ─── Lambda ───────────────────────────────────────────────────────────────────

func (c *EmulatorClient) LambdaFunctionExists(ctx context.Context, functionName string) (bool, error) {
	_, err := c.lambda.GetFunction(ctx, &awslambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ResourceNotFoundException" {
			return false, nil
		}
		return false, fmt.Errorf("getting function %q: %w", functionName, err)
	}
	return true, nil
}

func (c *EmulatorClient) LambdaFunctionConfig(ctx context.Context, functionName string) (LambdaConfig, error) {
	out, err := c.lambda.GetFunctionConfiguration(ctx, &awslambda.GetFunctionConfigurationInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		return LambdaConfig{}, fmt.Errorf("getting config for %q: %w", functionName, err)
	}

	env := make(map[string]string)
	if out.Environment != nil {
		for k, v := range out.Environment.Variables {
			env[k] = v
		}
	}

	return LambdaConfig{
		FunctionName: aws.ToString(out.FunctionName),
		FunctionARN:  aws.ToString(out.FunctionArn),
		RoleARN:      aws.ToString(out.Role),
		Handler:      aws.ToString(out.Handler),
		Runtime:      string(out.Runtime),
		Environment:  env,
	}, nil
}

func (c *EmulatorClient) LambdaEventSourceMappings(ctx context.Context, functionName string) ([]EventSourceMapping, error) {
	input := &awslambda.ListEventSourceMappingsInput{}
	if functionName != "" {
		input.FunctionName = aws.String(functionName)
	}

	out, err := c.lambda.ListEventSourceMappings(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("listing ESMs for %q: %w", functionName, err)
	}

	mappings := make([]EventSourceMapping, 0, len(out.EventSourceMappings))
	for _, m := range out.EventSourceMappings {
		mappings = append(mappings, EventSourceMapping{
			UUID:           aws.ToString(m.UUID),
			FunctionARN:    aws.ToString(m.FunctionArn),
			EventSourceARN: aws.ToString(m.EventSourceArn),
			State:          aws.ToString(m.State),
		})
	}
	return mappings, nil
}

func (c *EmulatorClient) LambdaInvoke(ctx context.Context, functionName string, payload []byte) ([]byte, error) {
	out, err := c.lambda.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName: aws.String(functionName),
		Payload:      payload,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking %q: %w", functionName, err)
	}
	if out.FunctionError != nil {
		return out.Payload, fmt.Errorf("function error: %s", aws.ToString(out.FunctionError))
	}
	return out.Payload, nil
}

// ─── SQS ──────────────────────────────────────────────────────────────────────

func (c *EmulatorClient) SQSQueueExists(ctx context.Context, queueName string) (bool, error) {
	_, err := c.sqsSvc.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "AWS.SimpleQueueService.NonExistentQueue" {
			return false, nil
		}
		return false, fmt.Errorf("checking queue %q: %w", queueName, err)
	}
	return true, nil
}

func (c *EmulatorClient) SQSQueueURL(ctx context.Context, queueName string) (string, error) {
	out, err := c.sqsSvc.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return "", fmt.Errorf("getting URL for queue %q: %w", queueName, err)
	}
	return aws.ToString(out.QueueUrl), nil
}

func (c *EmulatorClient) SQSSendMessage(ctx context.Context, queueURL, body string) error {
	_, err := c.sqsSvc.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(body),
	})
	if err != nil {
		return fmt.Errorf("sending message to queue %q: %w", queueURL, err)
	}
	return nil
}

// ─── S3 ───────────────────────────────────────────────────────────────────────

func (c *EmulatorClient) S3BucketExists(ctx context.Context, bucket string) (bool, error) {
	_, err := c.s3Svc.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && (ae.ErrorCode() == "NotFound" || ae.ErrorCode() == "NoSuchBucket") {
			return false, nil
		}
		return false, fmt.Errorf("checking bucket %q: %w", bucket, err)
	}
	return true, nil
}

func (c *EmulatorClient) S3BucketNotificationTargets(ctx context.Context, bucket string) ([]S3Notification, error) {
	out, err := c.s3Svc.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return nil, fmt.Errorf("getting notifications for bucket %q: %w", bucket, err)
	}

	var notifications []S3Notification

	for _, lc := range out.LambdaFunctionConfigurations {
		events := make([]string, len(lc.Events))
		for i, e := range lc.Events {
			events[i] = string(e)
		}
		notifications = append(notifications, S3Notification{
			Events:    events,
			LambdaARN: aws.ToString(lc.LambdaFunctionArn),
		})
	}

	for _, qc := range out.QueueConfigurations {
		events := make([]string, len(qc.Events))
		for i, e := range qc.Events {
			events[i] = string(e)
		}
		notifications = append(notifications, S3Notification{
			Events:   events,
			QueueARN: aws.ToString(qc.QueueArn),
		})
	}

	return notifications, nil
}

// ─── IAM ──────────────────────────────────────────────────────────────────────

func (c *EmulatorClient) IAMRoleExists(ctx context.Context, roleName string) (bool, error) {
	_, err := c.iamSvc.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "NoSuchEntity" {
			return false, nil
		}
		return false, fmt.Errorf("checking role %q: %w", roleName, err)
	}
	return true, nil
}

// IAMRolePolicies collects all policy statements for the role:
// inline policies (GetRolePolicy) + attached managed policies (GetPolicy + GetPolicyVersion).
func (c *EmulatorClient) IAMRolePolicies(ctx context.Context, roleName string) ([]PolicyStatement, error) {
	var stmts []PolicyStatement

	// Inline policies
	inline, err := c.iamSvc.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing inline policies for %q: %w", roleName, err)
	}
	for _, policyName := range inline.PolicyNames {
		out, err := c.iamSvc.GetRolePolicy(ctx, &iam.GetRolePolicyInput{
			RoleName:   aws.String(roleName),
			PolicyName: aws.String(policyName),
		})
		if err != nil {
			continue
		}
		decoded, _ := url.QueryUnescape(aws.ToString(out.PolicyDocument))
		stmts = append(stmts, parseStatements(decoded)...)
	}

	// Attached managed policies
	attached, err := c.iamSvc.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing attached policies for %q: %w", roleName, err)
	}
	for _, p := range attached.AttachedPolicies {
		pv, err := c.iamSvc.GetPolicy(ctx, &iam.GetPolicyInput{
			PolicyArn: p.PolicyArn,
		})
		if err != nil {
			continue
		}
		pvOut, err := c.iamSvc.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
			PolicyArn: p.PolicyArn,
			VersionId: pv.Policy.DefaultVersionId,
		})
		if err != nil {
			continue
		}
		decoded, _ := url.QueryUnescape(aws.ToString(pvOut.PolicyVersion.Document))
		stmts = append(stmts, parseStatements(decoded)...)
	}

	return stmts, nil
}

// parseStatements decodes a JSON IAM policy document into PolicyStatements.
func parseStatements(document string) []PolicyStatement {
	var doc struct {
		Statement []struct {
			Effect   string          `json:"Effect"`
			Action   json.RawMessage `json:"Action"`
			Resource json.RawMessage `json:"Resource"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		return nil
	}

	stmts := make([]PolicyStatement, 0, len(doc.Statement))
	for _, s := range doc.Statement {
		stmts = append(stmts, PolicyStatement{
			Effect:    s.Effect,
			Actions:   unmarshalStringOrSlice(s.Action),
			Resources: unmarshalStringOrSlice(s.Resource),
		})
	}
	return stmts
}

// unmarshalStringOrSlice handles IAM JSON where Action can be a string or []string.
func unmarshalStringOrSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}
	var multi []string
	_ = json.Unmarshal(raw, &multi)
	return multi
}

// ─── API Gateway V2 ───────────────────────────────────────────────────────────

func (c *EmulatorClient) APIGatewayV2Routes(ctx context.Context, apiID string) ([]APIRoute, error) {
	out, err := c.apigw.GetRoutes(ctx, &apigatewayv2.GetRoutesInput{
		ApiId: aws.String(apiID),
	})
	if err != nil {
		return nil, fmt.Errorf("getting routes for API %q: %w", apiID, err)
	}

	routes := make([]APIRoute, 0, len(out.Items))
	for _, r := range out.Items {
		routes = append(routes, APIRoute{
			RouteID:  aws.ToString(r.RouteId),
			RouteKey: aws.ToString(r.RouteKey),
			Target:   aws.ToString(r.Target),
		})
	}
	return routes, nil
}

func (c *EmulatorClient) APIGatewayV2APIs(ctx context.Context) ([]APIDetail, error) {
	out, err := c.apigw.GetApis(ctx, &apigatewayv2.GetApisInput{})
	if err != nil {
		return nil, fmt.Errorf("getting API Gateway APIs: %w", err)
	}

	apis := make([]APIDetail, 0, len(out.Items))
	for _, api := range out.Items {
		apis = append(apis, APIDetail{
			APIID:    aws.ToString(api.ApiId),
			Name:     aws.ToString(api.Name),
			Protocol: string(api.ProtocolType),
			Endpoint: aws.ToString(api.ApiEndpoint),
		})
	}

	return apis, nil
}

// APIGatewayV2InvokeURL returns the invoke URL for the API. When the emulator
// does not report an explicit API endpoint, it falls back to the backend's
// local execute-api convention.
func (c *EmulatorClient) APIGatewayV2InvokeURL(ctx context.Context, apiID string) (string, error) {
	apis, err := c.APIGatewayV2APIs(ctx)
	if err == nil {
		if invokeURL := apiGatewayV2InvokeURLFromDetails(apis, apiID); invokeURL != "" {
			return invokeURL, nil
		}
	}

	switch c.kind {
	case "stratus":
		return fmt.Sprintf("%s/_aws/execute-api/%s", c.baseURL, apiID), nil
	default:
		return fmt.Sprintf("%s/restapis/%s/test/_user_request_", c.baseURL, apiID), nil
	}
}

func apiGatewayV2InvokeURLFromDetails(apis []APIDetail, ref string) string {
	for _, api := range apis {
		if api.APIID == ref || api.Name == ref {
			return api.Endpoint
		}
	}
	return ""
}

// ─── DynamoDB ─────────────────────────────────────────────────────────────────

func (c *EmulatorClient) DynamoDBTableExists(ctx context.Context, tableName string) (bool, error) {
	_, err := c.ddb.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "ResourceNotFoundException" {
			return false, nil
		}
		return false, fmt.Errorf("describing table %q: %w", tableName, err)
	}
	return true, nil
}

func (c *EmulatorClient) DynamoDBGetItem(ctx context.Context, tableName string, key map[string]string) (map[string]string, error) {
	ddbKey := make(map[string]ddbtypes.AttributeValue, len(key))
	for k, v := range key {
		ddbKey[k] = &ddbtypes.AttributeValueMemberS{Value: v}
	}

	out, err := c.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       ddbKey,
	})
	if err != nil {
		return nil, fmt.Errorf("getting item from table %q: %w", tableName, err)
	}
	if out.Item == nil {
		return nil, nil
	}

	result := make(map[string]string, len(out.Item))
	for k, v := range out.Item {
		if sv, ok := v.(*ddbtypes.AttributeValueMemberS); ok {
			result[k] = sv.Value
		}
	}
	return result, nil
}
