Smoke fixtures for exercising `preflight` against real IaC projects.

- `cdk-http-sqs-ddb/`
  - Full CDK fixture for the POC path:
    API Gateway -> Lambda -> SQS -> Lambda -> DynamoDB.
  - Includes behavioural HTTP and SQS/DynamoDB assertions.
  - The smoke harness prebuilds Lambda zip files, uploads them to a Floci S3
    bucket, and deploys the stack against a clean Floci container each run.

- `terraform-http-sqs-ddb/`
  - Terraform fixture for the same infrastructure pattern.
  - Uses deterministic queue/table/function names.
  - Behavioural config covers the SQS -> Lambda -> DynamoDB chain.
  - The HTTP route is provisioned, but `preflight` cannot yet resolve a
    Terraform-created API Gateway by logical name because the current runtime
    only resolves API refs through CloudFormation resources.

Run the smoke harness from the repo root:

```bash
./scripts/smoke-fixtures.sh cdk
./scripts/smoke-fixtures.sh terraform
./scripts/smoke-fixtures.sh all
```
