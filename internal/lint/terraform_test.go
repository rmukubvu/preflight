package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateTerraformDirFindsReadinessGaps(t *testing.T) {
	dir := t.TempDir()
	tf := `
resource "aws_s3_bucket" "assets" {
  bucket = "demo-assets"
}

resource "aws_sqs_queue" "jobs" {
  name = "jobs"
}

resource "aws_dynamodb_table" "jobs" {
  name           = "jobs"
  billing_mode   = "PROVISIONED"
  read_capacity  = 5
  write_capacity = 5
  hash_key       = "id"
}

resource "aws_lambda_function" "worker" {
  function_name = "worker"
  role          = "arn:aws:iam::123456789012:role/demo"
  runtime       = "python3.11"
  handler       = "index.handler"
  filename      = "worker.zip"
}

resource "aws_apigatewayv2_route" "jobs" {
  api_id    = aws_apigatewayv2_api.jobs.id
  route_key = "POST /jobs"
}

resource "aws_apigatewayv2_stage" "jobs" {
  api_id      = aws_apigatewayv2_api.jobs.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_sfn_state_machine" "workflow" {
  name       = "workflow"
  role_arn   = "arn:aws:iam::123456789012:role/demo"
  definition = jsonencode({ StartAt = "Done", States = { Done = { Type = "Pass", End = true }}})
}

resource "aws_iam_role_policy" "wildcard" {
  role   = "demo"
  policy = jsonencode({ Version = "2012-10-17", Statement = [{ Action = "*", Resource = "*" }] })
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tf), 0o644); err != nil {
		t.Fatalf("write terraform file: %v", err)
	}

	result, err := EvaluateTerraformDir(dir)
	if err != nil {
		t.Fatalf("EvaluateTerraformDir returned error: %v", err)
	}

	assertHasRule(t, result, "s3-encryption")
	assertHasRule(t, result, "s3-public-access-block")
	assertHasRule(t, result, "sqs-redrive-policy")
	assertHasRule(t, result, "sqs-encryption")
	assertHasRule(t, result, "dynamodb-pitr")
	assertHasRule(t, result, "dynamodb-encryption")
	assertHasRule(t, result, "dynamodb-autoscaling")
	assertHasRule(t, result, "lambda-timeout-explicit")
	assertHasRule(t, result, "lambda-concurrency-explicit")
	assertHasRule(t, result, "lambda-log-retention")
	assertHasRule(t, result, "api-auth")
	assertHasRule(t, result, "api-throttling")
	assertHasRule(t, result, "sfn-logging")
	assertHasRule(t, result, "sfn-timeouts")
	assertHasRule(t, result, "iam-wildcard-action")
	assertHasRule(t, result, "iam-wildcard-resource")
}
