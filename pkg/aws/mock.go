package aws

import "context"

// MockClient is a test double for Client. Each method delegates to a
// function field so callers can provide targeted stub behaviour per test.
// Unset fields return zero values and nil errors.
type MockClient struct {
	CloudFormationStackStatusFn    func(ctx context.Context, stackName string) (string, error)
	CloudFormationStackResourcesFn func(ctx context.Context, stackName string) ([]StackResource, error)
	LambdaFunctionExistsFn         func(ctx context.Context, functionName string) (bool, error)
	LambdaFunctionConfigFn         func(ctx context.Context, functionName string) (LambdaConfig, error)
	LambdaEventSourceMappingsFn    func(ctx context.Context, functionName string) ([]EventSourceMapping, error)
	LambdaInvokeFn                 func(ctx context.Context, functionName string, payload []byte) ([]byte, error)
	SQSQueueExistsFn               func(ctx context.Context, queueName string) (bool, error)
	SQSQueueURLFn                  func(ctx context.Context, queueName string) (string, error)
	SQSSendMessageFn               func(ctx context.Context, queueURL, body string) error
	S3BucketExistsFn               func(ctx context.Context, bucket string) (bool, error)
	S3BucketNotificationTargetsFn  func(ctx context.Context, bucket string) ([]S3Notification, error)
	IAMRoleExistsFn                func(ctx context.Context, roleName string) (bool, error)
	IAMRolePoliciesFn              func(ctx context.Context, roleName string) ([]PolicyStatement, error)
	APIGatewayV2RoutesFn           func(ctx context.Context, apiID string) ([]APIRoute, error)
	APIGatewayV2APIsFn             func(ctx context.Context) ([]APIDetail, error)
	APIGatewayV2InvokeURLFn        func(ctx context.Context, apiID string) (string, error)
	DynamoDBTableExistsFn          func(ctx context.Context, tableName string) (bool, error)
	DynamoDBGetItemFn              func(ctx context.Context, tableName string, key map[string]string) (map[string]string, error)
}

var _ Client = (*MockClient)(nil)

func (m *MockClient) CloudFormationStackStatus(ctx context.Context, s string) (string, error) {
	if m.CloudFormationStackStatusFn != nil {
		return m.CloudFormationStackStatusFn(ctx, s)
	}
	return "CREATE_COMPLETE", nil
}

func (m *MockClient) CloudFormationStackResources(ctx context.Context, s string) ([]StackResource, error) {
	if m.CloudFormationStackResourcesFn != nil {
		return m.CloudFormationStackResourcesFn(ctx, s)
	}
	return nil, nil
}

func (m *MockClient) LambdaFunctionExists(ctx context.Context, s string) (bool, error) {
	if m.LambdaFunctionExistsFn != nil {
		return m.LambdaFunctionExistsFn(ctx, s)
	}
	return true, nil
}

func (m *MockClient) LambdaFunctionConfig(ctx context.Context, s string) (LambdaConfig, error) {
	if m.LambdaFunctionConfigFn != nil {
		return m.LambdaFunctionConfigFn(ctx, s)
	}
	return LambdaConfig{FunctionName: s, RoleARN: "arn:aws:iam::000000000000:role/test-role"}, nil
}

func (m *MockClient) LambdaEventSourceMappings(ctx context.Context, s string) ([]EventSourceMapping, error) {
	if m.LambdaEventSourceMappingsFn != nil {
		return m.LambdaEventSourceMappingsFn(ctx, s)
	}
	return nil, nil
}

func (m *MockClient) LambdaInvoke(ctx context.Context, s string, payload []byte) ([]byte, error) {
	if m.LambdaInvokeFn != nil {
		return m.LambdaInvokeFn(ctx, s, payload)
	}
	return []byte(`{"statusCode":200}`), nil
}

func (m *MockClient) SQSQueueExists(ctx context.Context, s string) (bool, error) {
	if m.SQSQueueExistsFn != nil {
		return m.SQSQueueExistsFn(ctx, s)
	}
	return true, nil
}

func (m *MockClient) SQSQueueURL(ctx context.Context, s string) (string, error) {
	if m.SQSQueueURLFn != nil {
		return m.SQSQueueURLFn(ctx, s)
	}
	return "http://localhost:4566/000000000000/" + s, nil
}

func (m *MockClient) SQSSendMessage(ctx context.Context, queueURL, body string) error {
	if m.SQSSendMessageFn != nil {
		return m.SQSSendMessageFn(ctx, queueURL, body)
	}
	return nil
}

func (m *MockClient) S3BucketExists(ctx context.Context, s string) (bool, error) {
	if m.S3BucketExistsFn != nil {
		return m.S3BucketExistsFn(ctx, s)
	}
	return true, nil
}

func (m *MockClient) S3BucketNotificationTargets(ctx context.Context, s string) ([]S3Notification, error) {
	if m.S3BucketNotificationTargetsFn != nil {
		return m.S3BucketNotificationTargetsFn(ctx, s)
	}
	return nil, nil
}

func (m *MockClient) IAMRoleExists(ctx context.Context, s string) (bool, error) {
	if m.IAMRoleExistsFn != nil {
		return m.IAMRoleExistsFn(ctx, s)
	}
	return true, nil
}

func (m *MockClient) IAMRolePolicies(ctx context.Context, s string) ([]PolicyStatement, error) {
	if m.IAMRolePoliciesFn != nil {
		return m.IAMRolePoliciesFn(ctx, s)
	}
	return nil, nil
}

func (m *MockClient) APIGatewayV2Routes(ctx context.Context, s string) ([]APIRoute, error) {
	if m.APIGatewayV2RoutesFn != nil {
		return m.APIGatewayV2RoutesFn(ctx, s)
	}
	return nil, nil
}

func (m *MockClient) APIGatewayV2APIs(ctx context.Context) ([]APIDetail, error) {
	if m.APIGatewayV2APIsFn != nil {
		return m.APIGatewayV2APIsFn(ctx)
	}
	return nil, nil
}

func (m *MockClient) APIGatewayV2InvokeURL(ctx context.Context, s string) (string, error) {
	if m.APIGatewayV2InvokeURLFn != nil {
		return m.APIGatewayV2InvokeURLFn(ctx, s)
	}
	return "http://localhost:4566/restapis/" + s + "/test/_user_request_", nil
}

func (m *MockClient) DynamoDBTableExists(ctx context.Context, s string) (bool, error) {
	if m.DynamoDBTableExistsFn != nil {
		return m.DynamoDBTableExistsFn(ctx, s)
	}
	return true, nil
}

func (m *MockClient) DynamoDBGetItem(ctx context.Context, table string, key map[string]string) (map[string]string, error) {
	if m.DynamoDBGetItemFn != nil {
		return m.DynamoDBGetItemFn(ctx, table, key)
	}
	return nil, nil
}
