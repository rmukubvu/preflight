variable "endpoint" {
  type    = string
  default = "http://localhost:4566"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true

  endpoints {
    apigatewayv2 = var.endpoint
    cloudformation = var.endpoint
    dynamodb = var.endpoint
    iam = var.endpoint
    lambda = var.endpoint
    sqs = var.endpoint
    sts = var.endpoint
  }
}
