package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateTemplatesFindsReadinessGaps(t *testing.T) {
	tpl := TemplateFile{
		Name: "AppStack.template.json",
		Template: Template{
			Resources: map[string]Resource{
				"AppBucket": {
					Type: "AWS::S3::Bucket",
					Properties: map[string]interface{}{
						"PublicAccessBlockConfiguration": map[string]interface{}{
							"BlockPublicAcls": true,
						},
					},
				},
				"AppQueue": {
					Type:       "AWS::SQS::Queue",
					Properties: map[string]interface{}{},
				},
				"AppTable": {
					Type: "AWS::DynamoDB::Table",
					Properties: map[string]interface{}{
						"ProvisionedThroughput": map[string]interface{}{
							"ReadCapacityUnits":  5,
							"WriteCapacityUnits": 5,
						},
					},
				},
				"AppFunction": {
					Type:       "AWS::Lambda::Function",
					Properties: map[string]interface{}{},
				},
				"HttpStage": {
					Type:       "AWS::ApiGatewayV2::Stage",
					Properties: map[string]interface{}{},
				},
				"AppRole": {
					Type: "AWS::IAM::Role",
					Properties: map[string]interface{}{
						"Policies": []interface{}{
							map[string]interface{}{
								"PolicyDocument": map[string]interface{}{
									"Statement": []interface{}{
										map[string]interface{}{
											"Action":   "*",
											"Resource": "*",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := EvaluateTemplates([]TemplateFile{tpl})

	if !result.HasErrors() {
		t.Fatalf("expected error-severity findings")
	}
	if got := len(result.Findings); got < 7 {
		t.Fatalf("expected multiple findings, got %d", got)
	}

	assertHasRule(t, result, "s3-encryption")
	assertHasRule(t, result, "s3-public-access-block")
	assertHasRule(t, result, "sqs-redrive-policy")
	assertHasRule(t, result, "sqs-encryption")
	assertHasRule(t, result, "dynamodb-pitr")
	assertHasRule(t, result, "dynamodb-encryption")
	assertHasRule(t, result, "dynamodb-autoscaling")
	assertHasRule(t, result, "lambda-concurrency-explicit")
	assertHasRule(t, result, "lambda-log-retention")
	assertHasRule(t, result, "api-access-logs")
	assertHasRule(t, result, "api-throttling")
	assertHasRule(t, result, "iam-wildcard-action")
	assertHasRule(t, result, "iam-wildcard-resource")
	assertHasRule(t, result, "alarm-coverage")
}

func TestEvaluateTemplatesPassesHealthyTemplate(t *testing.T) {
	tpl := TemplateFile{
		Name: "Healthy.template.json",
		Template: Template{
			Resources: map[string]Resource{
				"Alarm": {
					Type:       "AWS::CloudWatch::Alarm",
					Properties: map[string]interface{}{},
				},
				"Bucket": {
					Type: "AWS::S3::Bucket",
					Properties: map[string]interface{}{
						"BucketEncryption": map[string]interface{}{
							"ServerSideEncryptionConfiguration": []interface{}{
								map[string]interface{}{"ServerSideEncryptionByDefault": map[string]interface{}{"SSEAlgorithm": "AES256"}},
							},
						},
						"PublicAccessBlockConfiguration": map[string]interface{}{
							"BlockPublicAcls":       true,
							"BlockPublicPolicy":     true,
							"IgnorePublicAcls":      true,
							"RestrictPublicBuckets": true,
						},
					},
				},
				"Queue": {
					Type: "AWS::SQS::Queue",
					Properties: map[string]interface{}{
						"RedrivePolicy":        map[string]interface{}{"deadLetterTargetArn": "arn:aws:sqs:us-east-1:123456789012:dlq", "maxReceiveCount": 5},
						"SqsManagedSseEnabled": true,
					},
				},
				"Table": {
					Type: "AWS::DynamoDB::Table",
					Properties: map[string]interface{}{
						"BillingMode": "PAY_PER_REQUEST",
						"SSESpecification": map[string]interface{}{
							"SSEEnabled": true,
						},
						"PointInTimeRecoverySpecification": map[string]interface{}{
							"PointInTimeRecoveryEnabled": true,
						},
					},
				},
				"Function": {
					Type: "AWS::Lambda::Function",
					Properties: map[string]interface{}{
						"Timeout":                      15,
						"ReservedConcurrentExecutions": 10,
					},
				},
				"FunctionLogGroup": {
					Type: "AWS::Logs::LogGroup",
					Properties: map[string]interface{}{
						"LogGroupName": map[string]interface{}{
							"Fn::Join": []interface{}{
								"",
								[]interface{}{"/aws/lambda/", map[string]interface{}{"Ref": "Function"}},
							},
						},
						"RetentionInDays": 14,
					},
				},
				"Stage": {
					Type: "AWS::ApiGatewayV2::Stage",
					Properties: map[string]interface{}{
						"AccessLogSettings": map[string]interface{}{"DestinationArn": "arn:aws:logs:..."},
						"DefaultRouteSettings": map[string]interface{}{
							"ThrottlingBurstLimit": 100,
							"ThrottlingRateLimit":  50,
						},
					},
				},
				"Role": {
					Type: "AWS::IAM::Role",
					Properties: map[string]interface{}{
						"Policies": []interface{}{
							map[string]interface{}{
								"PolicyDocument": map[string]interface{}{
									"Statement": []interface{}{
										map[string]interface{}{
											"Action":   []interface{}{"s3:GetObject"},
											"Resource": []interface{}{"arn:aws:s3:::bucket/*"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := EvaluateTemplates([]TemplateFile{tpl})
	if result.HasFindings() {
		t.Fatalf("expected no findings, got %#v", result.Findings)
	}
}

func TestLoadTemplatesReadsSynthesizedTemplates(t *testing.T) {
	dir := t.TempDir()
	data, err := json.Marshal(Template{
		Resources: map[string]Resource{
			"Bucket": {Type: "AWS::S3::Bucket", Properties: map[string]interface{}{}},
		},
	})
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}

	path := filepath.Join(dir, "Stack.template.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	templates, err := LoadTemplates(dir)
	if err != nil {
		t.Fatalf("LoadTemplates returned error: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("want 1 template, got %d", len(templates))
	}
	if templates[0].Name != "Stack.template.json" {
		t.Fatalf("unexpected template name %q", templates[0].Name)
	}
}

func TestEvaluateTemplatesFindsEventDrivenDurabilityGaps(t *testing.T) {
	tpl := TemplateFile{
		Name: "Evented.template.json",
		Template: Template{
			Resources: map[string]Resource{
				"DataStream": {
					Type:       "AWS::Kinesis::Stream",
					Properties: map[string]interface{}{},
				},
				"PipelineStateMachine": {
					Type:       "AWS::StepFunctions::StateMachine",
					Properties: map[string]interface{}{},
				},
				"Rule": {
					Type: "AWS::Events::Rule",
					Properties: map[string]interface{}{
						"Targets": []interface{}{
							map[string]interface{}{
								"Id":  "InvokeWorker",
								"Arn": "arn:aws:lambda:us-east-1:000000000000:function:worker",
							},
						},
					},
				},
				"Subscription": {
					Type: "AWS::SNS::Subscription",
					Properties: map[string]interface{}{
						"Protocol": "lambda",
					},
				},
				"StreamMapping": {
					Type: "AWS::Lambda::EventSourceMapping",
					Properties: map[string]interface{}{
						"EventSourceArn": map[string]interface{}{
							"Fn::GetAtt": []interface{}{"DataStream", "Arn"},
						},
					},
				},
			},
		},
	}

	result := EvaluateTemplates([]TemplateFile{tpl})

	assertHasRule(t, result, "sfn-logging")
	assertHasRule(t, result, "sfn-tracing")
	assertHasRule(t, result, "eventbridge-retry-policy")
	assertHasRule(t, result, "eventbridge-dlq")
	assertHasRule(t, result, "sns-redrive-policy")
	assertHasRule(t, result, "stream-retry-budget")
	assertHasRule(t, result, "stream-batch-bisect")
	assertHasRule(t, result, "stream-batching-window")
}

func assertHasRule(t *testing.T, result Result, ruleID string) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.RuleID == ruleID {
			return
		}
	}
	t.Fatalf("expected finding with rule %q, got %#v", ruleID, result.Findings)
}
