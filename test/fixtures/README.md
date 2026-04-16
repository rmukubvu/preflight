Smoke fixtures for exercising `preflight` against real IaC projects.

- `cdk-http-sqs-ddb/`
  - Full CDK fixture for the POC path:
    API Gateway -> Lambda -> SQS -> Lambda -> DynamoDB.
  - Includes behavioural HTTP and SQS/DynamoDB assertions.
  - The smoke harness prebuilds Lambda zip files, uploads them to the emulator S3
    bucket, and deploys the stack against a clean local emulator each run.

- `terraform-http-sqs-ddb/`
  - Terraform fixture for the same infrastructure pattern.
  - Uses deterministic queue/table/function names.
  - Behavioural config covers the SQS -> Lambda -> DynamoDB chain.
  - The HTTP route is provisioned, but `preflight` cannot yet resolve a
    Terraform-created API Gateway by logical name because the current runtime
    only resolves API refs through CloudFormation resources.

- `pulumi-csharp-httpapi-dynamodb/`
  - Pulumi fixture written in C#.
  - Deploys `API Gateway -> Lambda` against Stratus.
  - Uses explicit deployed resource names, so `preflight deploy --skip-lint`
    can run behavioural assertions without CloudFormation resource discovery.
  - The Lambda runtime is `nodejs20.x`; C# Lambda execution is a separate
    runtime tranche and is not part of this fixture.

Run the smoke harness from the repo root:

```bash
./scripts/smoke-fixtures.sh cdk
./scripts/smoke-fixtures.sh terraform
./scripts/smoke-fixtures.sh pulumi
./scripts/smoke-fixtures.sh all
```
