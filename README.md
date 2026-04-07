# preflight

Validate CDK and Terraform stacks against Floci before deploying to AWS.

`preflight` runs your infrastructure locally against [Floci](https://floci.io/), deploys the stack, and checks four layers before you touch AWS:

- Structural: did the expected resources get created?
- Wiring: are routes, event source mappings, and triggers connected?
- IAM: do the Lambda roles have the permissions the flow needs?
- Behavioural: does the actual request or event path work end to end?

For a CDK project, the target local POC path is:

```text
API Gateway -> Lambda -> SQS -> Lambda -> DynamoDB
```

## What You Need

- Go `1.23+`
- Docker Desktop or a working Docker daemon
- A CDK app that can already synthesize locally
- Node.js and npm if your CDK project is TypeScript/JavaScript
- AWS CDK CLI available either:
  - in your project at `node_modules/.bin/cdk`
  - via `npx cdk`
  - or globally as `cdk`

`preflight` starts Floci in Docker and points the deploy to it with local test credentials. You do not need a real AWS account for the local validation flow.

## Install

Build from source:

```bash
git clone https://github.com/rmukubvu/preflight.git
cd preflight
make build
```

The binary will be available at:

```bash
./dist/preflight
```

You can also run the setup UI directly:

```bash
make run-setup
```

## Core Commands

```bash
./dist/preflight --help
./dist/preflight setup --help
./dist/preflight deploy --help
```

Main workflow:

```bash
./dist/preflight setup
./dist/preflight deploy --stack-name MyStack --no-ai
```

## CDK Quickstart

This is the shortest path if you already have a working CDK project.

### 1. Open the setup UI

From your CDK project directory:

```bash
preflight setup
```

This creates `.preflight.yaml` in the current directory.

If you do not want a browser to open automatically:

```bash
BROWSER=echo preflight setup
```

### 2. Fill in the stack settings

For CDK, the important fields are:

- `stack.type: cdk`
- `stack.dir: .`
- `stack.cdk_app`: your CDK app command, for example:
  - `npx cdk`
  - `node bin/app.js`
  - `npx ts-node --prefer-ts-exts bin/app.ts`

If your project already has a local CDK CLI in `node_modules/.bin/cdk`, `preflight` prefers that automatically.

### 3. Add behavioural assertions

Structural, wiring, and IAM checks can be discovered from deployed resources.

Behavioural checks are stack-specific, so you need to tell `preflight` what a real success path looks like. For a common HTTP plus queue flow, add:

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

### 4. Run the local validation

```bash
preflight deploy --stack-name MyStack --no-ai
```

If everything works, you will see a summary like:

```text
✓ STRUCTURAL
✓ WIRING
✓ IAM
✓ BEHAVIOURAL

✓ All assertions passed. Safe to deploy to AWS.
```

## How CDK Local Validation Works

When you run:

```bash
preflight deploy --stack-name MyStack
```

`preflight` does this:

1. Starts Floci in Docker if it is not already running.
2. Sets local AWS environment variables so CDK deploys to Floci instead of AWS.
3. Runs `cdk deploy --require-approval never`.
4. Discovers deployed resources from CloudFormation.
5. Runs assertion suites in parallel.
6. Prints a pass/fail summary.

The deploy is redirected with environment variables like:

```text
AWS_ENDPOINT_URL=http://localhost:4566
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_DEFAULT_REGION=us-east-1
CDK_DEFAULT_ACCOUNT=000000000000
CDK_DEFAULT_REGION=us-east-1
```

## Behavioural Config Explained

### HTTP checks

Each HTTP behavioural check tells `preflight` to make a real request and verify the expected response code.

Fields:

- `api`: API Gateway logical ID or deployed API ID
- `integration_function`: optional Lambda fallback for local emulators
- `method`: `GET`, `POST`, `PUT`, `DELETE`, etc.
- `path`: route path such as `/jobs`
- `expected_status`: expected HTTP status code
- `body`: optional request body
- `headers`: optional request headers

The `integration_function` field matters locally because some emulators do not reliably expose API Gateway invoke URLs even when the route and integration exist. In that case, `preflight` can still validate the integration Lambda path.

### SQS -> Lambda -> DynamoDB checks

Each queue behavioural check:

1. sends a real message to SQS
2. waits for the side effect in DynamoDB
3. optionally falls back to a direct consumer Lambda invoke if the emulator does not deliver the event source mapping cleanly

Fields:

- `queue`: logical ID, queue name, ARN, or queue URL
- `table`: logical ID or table name
- `consumer_function`: optional consumer Lambda fallback
- `message_body`: message to enqueue
- `expected_key`: DynamoDB key that proves the message was processed

Example:

```yaml
assertions:
  behavioural:
    sqs_to_lambda_to_dynamodb:
      - queue: JobQueue
        table: JobsTable
        consumer_function: WorkerFunction
        message_body: '{"id":"job-123"}'
        expected_key:
          id: job-123
```

That means:

- push `{"id":"job-123"}` into the queue
- wait for the worker to process it
- pass only if an item with partition key `id=job-123` appears in DynamoDB

## Example `.preflight.yaml` for CDK

This is a practical starter config:

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

## Recommended CDK Conventions

`preflight` works best when your stack uses stable, intentional names.

Recommended:

- give queues, tables, APIs, and functions explicit names where practical
- keep your CDK logical IDs readable
- make the DynamoDB side effect easy to assert with a deterministic key
- use a single request payload that can be replayed safely in local runs

That makes the behavioural config straightforward and maintainable.

## Running From an Existing CDK App

If your CDK project lives elsewhere, you do not have to move it into this repo.

Typical flow:

```bash
cd /path/to/your-cdk-app
preflight setup
preflight deploy --stack-name YourStackName --no-ai
```

Or if you want to point to a different directory explicitly:

```bash
preflight deploy --stack-type cdk --dir /path/to/your-cdk-app --stack-name YourStackName --no-ai
```

## Output and Exit Codes

- Exit code `0`: all assertions passed
- Exit code `1`: one or more assertions failed

You can also write a JSON report:

```bash
preflight deploy --stack-name MyStack --report preflight-report.json --no-ai
```

## Troubleshooting

### `cdk not found`

Install the CDK CLI or add it locally:

```bash
npm install --save-dev aws-cdk
```

### Floci does not start

Check:

- Docker Desktop is running
- your Docker socket is available
- port `4566` is free, or change `floci.port`

### My stack deploys but behavioural checks fail

That usually means one of:

- the request payload does not match your app contract
- the expected HTTP status is wrong
- the queue consumer is not writing the expected DynamoDB key
- the local emulator needs the fallback Lambda fields:
  - `integration_function`
  - `consumer_function`

### API Gateway route exists but HTTP behaviour still fails locally

Use `integration_function` on the HTTP assertion. Some local emulators provision the route metadata but do not expose a stable invoke path.

### SQS wiring exists but the item never appears in DynamoDB

Use `consumer_function` on the SQS assertion. This lets `preflight` validate the queue consumer path even if the local event source mapping delivery is inconsistent.

## Smoke Fixtures

This repo includes local smoke fixtures:

- CDK: [test/fixtures/cdk-http-sqs-ddb](/Users/robson/code/preflight/test/fixtures/cdk-http-sqs-ddb)
- Terraform: [test/fixtures/terraform-http-sqs-ddb](/Users/robson/code/preflight/test/fixtures/terraform-http-sqs-ddb)

Run them from the repo root:

```bash
make smoke-cdk-fixture
make smoke-terraform-fixture
make smoke-fixtures
```

The CDK smoke fixture currently proves the full local behavioural POC path.

## Developer Commands

```bash
make build
make test
make test-unit
make lint
make fmt
make run-setup
```

## Current Scope

Today the strongest local path is CDK:

- CDK local deploy to Floci works
- structural, wiring, IAM, and behavioural checks run
- the full behavioural POC path passes locally in the CDK smoke fixture

Terraform support exists, but the current Terraform smoke fixture still hits Floci/provider compatibility gaps around Lambda code-signing reads and API Gateway v2 stage creation.
