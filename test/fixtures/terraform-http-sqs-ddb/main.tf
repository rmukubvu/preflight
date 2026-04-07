locals {
  api_function_name    = "jobs-api-handler-fixture"
  queue_name           = "job-queue-fixture"
  table_name           = "jobs-table-fixture"
  worker_function_name = "jobs-worker-fixture"
}

data "archive_file" "api_lambda" {
  type        = "zip"
  source_file = "${path.module}/lambda/api/index.py"
  output_path = "${path.module}/build/api.zip"
}

data "archive_file" "worker_lambda" {
  type        = "zip"
  source_file = "${path.module}/lambda/worker/index.py"
  output_path = "${path.module}/build/worker.zip"
}

resource "aws_dynamodb_table" "jobs_table" {
  name         = local.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }
}

resource "aws_sqs_queue" "job_queue" {
  name                       = local.queue_name
  visibility_timeout_seconds = 30
}

resource "aws_iam_role" "api_lambda_role" {
  name = "jobs-api-handler-role-fixture"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "api_lambda_policy" {
  name = "jobs-api-handler-policy-fixture"
  role = aws_iam_role.api_lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Effect   = "Allow"
        Resource = "*"
      },
      {
        Action = [
          "sqs:GetQueueAttributes",
          "sqs:GetQueueUrl",
          "sqs:SendMessage"
        ]
        Effect   = "Allow"
        Resource = aws_sqs_queue.job_queue.arn
      }
    ]
  })
}

resource "aws_iam_role" "worker_lambda_role" {
  name = "jobs-worker-role-fixture"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "worker_lambda_policy" {
  name = "jobs-worker-policy-fixture"
  role = aws_iam_role.worker_lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Effect   = "Allow"
        Resource = "*"
      },
      {
        Action = [
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
          "sqs:ReceiveMessage"
        ]
        Effect   = "Allow"
        Resource = aws_sqs_queue.job_queue.arn
      },
      {
        Action = [
          "dynamodb:PutItem"
        ]
        Effect   = "Allow"
        Resource = aws_dynamodb_table.jobs_table.arn
      }
    ]
  })
}

resource "aws_lambda_function" "api_handler" {
  function_name    = local.api_function_name
  filename         = data.archive_file.api_lambda.output_path
  source_code_hash = data.archive_file.api_lambda.output_base64sha256
  handler          = "index.handler"
  role             = aws_iam_role.api_lambda_role.arn
  runtime          = "python3.11"
  timeout          = 30

  environment {
    variables = {
      QUEUE_URL = aws_sqs_queue.job_queue.url
    }
  }
}

resource "aws_lambda_function" "worker" {
  function_name    = local.worker_function_name
  filename         = data.archive_file.worker_lambda.output_path
  source_code_hash = data.archive_file.worker_lambda.output_base64sha256
  handler          = "index.handler"
  role             = aws_iam_role.worker_lambda_role.arn
  runtime          = "python3.11"
  timeout          = 30

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.jobs_table.name
    }
  }
}

resource "aws_lambda_event_source_mapping" "worker_queue_mapping" {
  event_source_arn = aws_sqs_queue.job_queue.arn
  function_name    = aws_lambda_function.worker.arn
  batch_size       = 1
  enabled          = true
}

resource "aws_apigatewayv2_api" "jobs_api" {
  name          = "jobs-api-fixture"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "jobs_api_integration" {
  api_id                 = aws_apigatewayv2_api.jobs_api.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.api_handler.invoke_arn
  integration_method     = "POST"
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "jobs_post_route" {
  api_id    = aws_apigatewayv2_api.jobs_api.id
  route_key = "POST /jobs"
  target    = "integrations/${aws_apigatewayv2_integration.jobs_api_integration.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.jobs_api.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "allow_apigw_invoke" {
  statement_id  = "AllowExecutionFromAPIGatewayFixture"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.api_handler.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.jobs_api.execution_arn}/*/*"
}

output "api_id" {
  value = aws_apigatewayv2_api.jobs_api.id
}

output "queue_name" {
  value = aws_sqs_queue.job_queue.name
}

output "table_name" {
  value = aws_dynamodb_table.jobs_table.name
}
