# preflight

Validate CDK and Terraform stacks against a local AWS emulator before deploying
to AWS.

Stratus emulates your AWS stack locally. Preflight verifies that it actually
works before you deploy.

Together, Stratus and Preflight give developers a credible local AWS delivery
loop.

What Stratus solves:

- run AWS-shaped infrastructure locally
- use real tooling like AWS CLI, SDKs, and CDK
- get fast feedback without deploying to AWS
- reproduce failures in a deterministic environment

What Preflight solves:

- prove the stack actually works, not just that resources were created
- validate structure, wiring, IAM, and behavior from the outside
- flag missing conditions for scalability, reliability, observability, and security before deploy
- add diagnosis when something fails
- catch compatibility regressions before cloud deployment

`preflight` deploys your infrastructure locally and checks four layers before
you touch a real AWS account:

- Structural: did the expected resources get created?
- Wiring: are routes, event source mappings, and triggers connected?
- IAM: do the Lambda roles have the permissions the flow needs?
- Behavioural: does the actual request or event path work end to end?

For the current smoke fixtures, the primary behavioural path is:

```text
API Gateway -> Lambda -> SQS -> Lambda -> DynamoDB
```

## Current State

`preflight` is an emulator-oriented validator for local AWS delivery loops.

What is true today:

- the root CLI exists at `cmd/preflight/main.go`
- `preflight lint` runs a fast static readiness pass for CDK and Terraform stacks
- `preflight load` replays HTTP behavioural checks under concurrent local load
- `preflight deploy` runs static lint first, then deploys into the configured emulator
- the direct CLI deploy path can target `stratus`, `floci`, or a custom endpoint
- the smoke harness can run against `stratus` or `floci`
- the smoke harness defaults to `stratus`
- the setup/config surface still carries a legacy `floci:` block for
  compatibility
- lint findings now include deterministic diagnoses, with optional AI overlay

## Validated Loop

The current proof path is explicit:

1. Java SDK smoke talks to `stratus` over a real network boundary.
2. CDK deploys a live local stack into `stratus`.
3. `preflight` lint checks readiness conditions before deploy.
4. `preflight deploy` validates structural, wiring, IAM, and behavioural paths.
5. the `stratus` release gate chains the Java SDK smoke and the external `preflight` CDK smoke.

That matters because the claim is not just that a stack synthesizes or creates
resources. The claim is that the same local stack is exercised through real
client tooling and an external validator before AWS.

## What You Need

- Go `1.23+`
- Docker Desktop or a working Docker daemon
- A CDK app that can already synthesize locally, or a Terraform/OpenTofu module
- Node.js and npm for JavaScript/TypeScript CDK projects
- AWS CLI for smoke fixture seeding
- The AWS CDK CLI available either:
  - in your project at `node_modules/.bin/cdk`
  - via `npx cdk`
  - or globally as `cdk`

For `stratus` validation, you also need a `stratus` binary or checkout available
locally.

## Install

Build from source:

```bash
git clone https://github.com/rmukubvu/preflight.git
cd preflight
make build
```

The binary will be:

```bash
./dist/preflight
```

Run the setup UI:

```bash
make run-setup
```

## Core Commands

```bash
./dist/preflight --help
./dist/preflight setup --help
./dist/preflight lint --help
./dist/preflight load --help
./dist/preflight deploy --help
```

Main manual workflow:

```bash
./dist/preflight setup
./dist/preflight lint --stack-name MyStack
./dist/preflight load --stack-name MyStack --vus 4 --iterations 20
./dist/preflight deploy --stack-name MyStack --no-ai
```

`preflight lint` is the fast static pass. It now supports:

- CDK by synthesizing CloudFormation templates
- Terraform by parsing HCL directly

It checks for common readiness gaps such as:

- wildcard IAM permissions
- S3 public access and encryption gaps
- SQS queues without DLQs
- DynamoDB tables without PITR or autoscaling posture
- Lambda functions without explicit log retention
- API stages without access logging
- API routes without explicit auth posture
- Step Functions without logging, tracing, or timeout posture
- EventBridge, SNS, and stream consumers without delivery durability controls
- stacks with workload resources but no CloudWatch alarms

When lint finds issues, `preflight` now prints a diagnosis for each finding.
With no AI provider configured, it falls back to a deterministic rulebook
explanation and fix.

Lint also supports structured external output:

```bash
./dist/preflight lint --output text
./dist/preflight lint --output json
./dist/preflight lint --output markdown
```

- `text` is the default terminal view
- `json` is intended for CI, future UI work, and machine-readable findings
- `markdown` is intended for PR comments and human review summaries

Both `json` and `markdown` include grouped remediation summaries so the same
run can feed local use, CI checks, and future external surfaces.

If you want those artifacts from the normal deploy path, use:

```bash
./dist/preflight deploy \
  --stack-name MyStack \
  --lint-report-json ./artifacts/lint.json \
  --lint-report-markdown ./artifacts/lint.md \
  --report ./artifacts/assertions.json
```

That gives you:

- a lint JSON artifact for machine readers
- a lint markdown summary for PR comments or review threads
- an assertion JSON artifact for the deploy-time validation result

`preflight load` uses the existing behavioural HTTP assertions as a local load
scenario. It can deploy the stack first, then replay those HTTP paths with
configurable concurrency and iteration counts to surface latency and failure
signals before AWS deployment.

## Two Runtime Modes

### 1. Direct CLI deploy path

This is the main built-in path.

When you run:

```bash
./dist/preflight deploy --stack-name MyStack --no-ai
```

`preflight` currently:

1. loads `.preflight.yaml`
2. runs `preflight lint` first unless you pass `--skip-lint`
   - this now applies to both CDK and Terraform stacks
3. starts the configured emulator if needed
4. deploys the CDK or Terraform stack into that local endpoint
5. discovers resources
6. runs structural, wiring, IAM, and behavioural assertions

### 2. Emulator smoke harness

This is the best path today for validating `stratus`.

The smoke harness is:

- [`scripts/smoke-fixtures.sh`](/Users/robson/code/preflight/scripts/smoke-fixtures.sh)

It supports:

- `EMULATOR_TYPE=stratus`
- `EMULATOR_TYPE=floci`

It defaults to:

```bash
EMULATOR_TYPE=stratus
EMULATOR_ENDPOINT=http://localhost:4566
EMULATOR_COMMAND=stratus
```

This is the path used for the current `stratus` integration smoke.

It is also the basis for the current release-gate story:

- Java SDK fixture proves client compatibility
- CDK fixture proves deployability
- `preflight` proves the behavioural path
- the release gate runs them together before changes should be trusted

## CDK Quickstart

If you already have a working CDK app, the shortest path is:

```bash
cd /path/to/your-cdk-app
preflight setup
preflight deploy --stack-name YourStackName --no-ai
```

Or explicitly:

```bash
preflight deploy --stack-type cdk --dir /path/to/your-cdk-app --stack-name YourStackName --no-ai
```

Important setup fields:

- `stack.type: cdk`
- `stack.dir: .`
- `stack.cdk_app`: for example:
  - `npx cdk`
  - `node bin/app.js`
  - `npx ts-node --prefer-ts-exts bin/app.ts`

If your project has a local CDK CLI in `node_modules/.bin/cdk`, `preflight`
prefers that automatically.

## Behavioural Assertions

Structural, wiring, and IAM checks can be discovered from deployed resources.
Behavioural checks are stack-specific, so you need to define what “real success”
means for your application.

### HTTP checks

Fields:

- `api`: API Gateway logical ID or deployed API ID
- `integration_function`: optional Lambda fallback for local emulators
- `method`: `GET`, `POST`, `PUT`, `DELETE`, etc.
- `path`: route path such as `/jobs`
- `expected_status`: expected HTTP status code
- `body`: optional request body
- `headers`: optional request headers

### SQS -> Lambda -> DynamoDB checks

Fields:

- `queue`: logical ID, queue name, ARN, or queue URL
- `table`: logical ID or table name
- `consumer_function`: optional consumer Lambda fallback
- `message_body`: message to enqueue
- `expected_key`: DynamoDB key that proves the message was processed

Example:

```yaml
version: 1

llm:
  provider: none

floci:
  image: hectorvent/floci:latest
  port: 4566

stack:
  type: cdk
  dir: .
  cdk_app: npx cdk

assertions:
  behavioural:
    http:
      - api: JobsApi
        integration_function: JobsHandlerFunction
        method: POST
        path: /jobs
        expected_status: 202
        body: '{"id":"job-123"}'
        headers:
          Content-Type: application/json

    sqs_to_lambda_to_dynamodb:
      - queue: JobQueue
        table: JobsTable
        consumer_function: WorkerFunction
        message_body: '{"id":"job-123"}'
        expected_key:
          id: job-123
```

Note the legacy `floci:` key. That remains in the schema for backward
compatibility, even though the runtime manager now targets a general emulator.

## Using preflight With stratus

The safest `stratus` path is still the smoke harness because it exercises the
full fixture loop end to end.

From the `preflight` repo root:

```bash
bash ./scripts/smoke-fixtures.sh cdk
```

That defaults to `stratus`. You can point it at a specific binary and endpoint:

```bash
EMULATOR_TYPE=stratus \
EMULATOR_COMMAND=/path/to/stratus \
EMULATOR_ENDPOINT=http://127.0.0.1:4567 \
EMULATOR_PORT=4567 \
bash ./scripts/smoke-fixtures.sh cdk
```

What the harness does:

1. builds `preflight`
2. starts the chosen emulator
3. seeds fixture assets for the CDK stack
4. runs `preflight deploy`
5. passes emulator overrides through:
   - `PREFLIGHT_EMULATOR_TYPE`
   - `PREFLIGHT_EMULATOR_COMMAND`
   - `PREFLIGHT_EMULATOR_ENDPOINT`
   - `PREFLIGHT_EMULATOR_PORT`

These overrides are currently the bridge between the old config surface and the
new emulator-oriented validation path.

## Using preflight With Floci

If you want the legacy container-backed path explicitly:

```bash
EMULATOR_TYPE=floci bash ./scripts/smoke-fixtures.sh cdk
```

Or just use:

```bash
./dist/preflight deploy --stack-name MyStack --no-ai
```

That direct deploy path now uses the configured emulator backend.

## Smoke Fixtures

This repo includes two smoke fixtures:

- CDK: [`test/fixtures/cdk-http-sqs-ddb`](/Users/robson/code/preflight/test/fixtures/cdk-http-sqs-ddb)
- Terraform: [`test/fixtures/terraform-http-sqs-ddb`](/Users/robson/code/preflight/test/fixtures/terraform-http-sqs-ddb)

Run them from the repo root:

```bash
./scripts/smoke-fixtures.sh cdk
./scripts/smoke-fixtures.sh terraform
./scripts/smoke-fixtures.sh all
```

Or with `make`:

```bash
make smoke-cdk-fixture
make smoke-terraform-fixture
make smoke-fixtures
```

The fixture overview lives in:

- [`test/fixtures/README.md`](/Users/robson/code/preflight/test/fixtures/README.md)

## How the CDK Smoke Works

The CDK fixture:

- prebuilds Lambda zip assets
- uploads them to a local S3 bucket on the emulator
- deploys the stack
- runs HTTP and queue-driven behavioural assertions

The fixture also now passes a Docker-reachable emulator endpoint into the Lambda
functions so it can work with `stratus` on non-default ports instead of
hardcoding `host.docker.internal:4566`.

## Output and Exit Codes

- exit code `0`: all assertions passed
- exit code `1`: one or more assertions failed

You can also write a JSON report:

```bash
preflight deploy --stack-name MyStack --report preflight-report.json --no-ai
```

## Recommended Conventions

`preflight` works best when your stack uses stable, intentional names.

Recommended:

- give queues, tables, APIs, and functions explicit names where practical
- keep CDK logical IDs readable
- make DynamoDB side effects easy to assert with a deterministic key
- use replay-safe request payloads for local runs

That keeps behavioural config small and maintainable.

## Troubleshooting

### `cdk not found`

Install the CDK CLI or add it locally:

```bash
npm install --save-dev aws-cdk
```

### The emulator does not start

Check:

- Docker is running if you are using Floci
- your Docker socket is available
- the target port is free
- `EMULATOR_COMMAND` points to a valid `stratus` binary if using `stratus`

### Behavioural checks fail even though resources exist

Usually one of:

- the request payload does not match your app contract
- the expected HTTP status is wrong
- the queue consumer is not writing the expected DynamoDB key
- the emulator needs a fallback Lambda field:
  - `integration_function`
  - `consumer_function`

### API Gateway route exists but the HTTP behaviour fails locally

Use `integration_function` on the HTTP assertion. That lets `preflight`
validate the Lambda-backed path even when the local API invoke surface is less
reliable than real AWS.

### SQS wiring exists but the item never appears in DynamoDB

Use `consumer_function` on the SQS assertion. That gives `preflight` a direct
fallback to validate the consumer path.

## Developer Commands

```bash
make build
make test
make test-unit
make lint
make fmt
make run-setup
make tidy
```

## Current Scope

Today the strongest local path is:

- CDK static readiness linting
- CDK fixture deployment
- structural, wiring, IAM, and behavioural assertions
- `stratus` validation through the smoke harness
- emulator-backed validation through the direct deploy path and smoke harness

Terraform support exists, but it is still less mature than the CDK path.

## Summary

`preflight` should be thought of as the external validation harness around a
local AWS emulator, not as emulator implementation code itself.

Today:

- direct CLI deploy runs lint first for CDK stacks and can target `stratus`, `floci`, or a custom endpoint
- the smoke harness is the best path for `stratus`
- the repo still carries a legacy `floci:` config block for backward compatibility

That is the state this README now documents.
