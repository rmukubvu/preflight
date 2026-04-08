const cdk = require("aws-cdk-lib");
const apigwv2 = require("aws-cdk-lib/aws-apigatewayv2");
const integrations = require("aws-cdk-lib/aws-apigatewayv2-integrations");
const dynamodb = require("aws-cdk-lib/aws-dynamodb");
const lambda = require("aws-cdk-lib/aws-lambda");
const lambdaEventSources = require("aws-cdk-lib/aws-lambda-event-sources");
const s3 = require("aws-cdk-lib/aws-s3");
const sqs = require("aws-cdk-lib/aws-sqs");

const assetBucketName = "preflight-cdk-fixture-assets";
const jobsAPIName = "cdk-jobs-api-fixture";
const jobsHandlerAssetKey = process.env.JOBS_HANDLER_ASSET_KEY || "jobs-handler.zip";
const jobsHandlerName = "cdk-jobs-handler-fixture";
const jobQueueName = "cdk-job-queue-fixture";
const jobsTableName = "cdk-jobs-table-fixture";
const workerAssetKey = process.env.WORKER_ASSET_KEY || "worker.zip";
const workerFunctionName = "cdk-jobs-worker-fixture";
const defaultEmulatorEndpoint = "http://host.docker.internal:4566";

function dockerReachableEndpoint(rawEndpoint) {
  const endpoint = new URL(rawEndpoint || defaultEmulatorEndpoint);
  if (endpoint.hostname === "127.0.0.1" || endpoint.hostname === "localhost") {
    endpoint.hostname = "host.docker.internal";
  }
  return endpoint.toString().replace(/\/$/, "");
}

class SmokeFixtureStack extends cdk.Stack {
  constructor(scope, id, props) {
    super(scope, id, {
      ...props,
      synthesizer: new cdk.BootstraplessSynthesizer(),
    });

    const assetsBucket = s3.Bucket.fromBucketName(
      this,
      "FixtureAssetsBucket",
      assetBucketName,
    );
    const emulatorEndpoint = dockerReachableEndpoint(process.env.EMULATOR_ENDPOINT);
    const queueUrl = `${emulatorEndpoint}/000000000000/${jobQueueName}`;

    const table = new dynamodb.Table(this, "JobsTable", {
      partitionKey: {
        name: "id",
        type: dynamodb.AttributeType.STRING,
      },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      tableName: jobsTableName,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    const queue = new sqs.Queue(this, "JobQueue", {
      queueName: jobQueueName,
      visibilityTimeout: cdk.Duration.seconds(30),
    });

    const jobsHandler = new lambda.Function(this, "JobsHandlerFunction", {
      functionName: jobsHandlerName,
      runtime: lambda.Runtime.PYTHON_3_11,
      handler: "index.handler",
      timeout: cdk.Duration.seconds(30),
      code: lambda.Code.fromBucket(assetsBucket, jobsHandlerAssetKey),
      environment: {
        EMULATOR_ENDPOINT: emulatorEndpoint,
        QUEUE_URL: queueUrl,
      },
    });

    const workerFunction = new lambda.Function(this, "WorkerFunction", {
      functionName: workerFunctionName,
      runtime: lambda.Runtime.PYTHON_3_11,
      handler: "index.handler",
      timeout: cdk.Duration.seconds(30),
      code: lambda.Code.fromBucket(assetsBucket, workerAssetKey),
      environment: {
        EMULATOR_ENDPOINT: emulatorEndpoint,
        TABLE_NAME: jobsTableName,
      },
    });

    queue.grantSendMessages(jobsHandler);
    queue.grantConsumeMessages(workerFunction);
    table.grantWriteData(workerFunction);

    workerFunction.addEventSource(
      new lambdaEventSources.SqsEventSource(queue, {
        batchSize: 1,
      }),
    );

    const httpApi = new apigwv2.HttpApi(this, "JobsApi", {
      apiName: jobsAPIName,
      createDefaultStage: true,
    });

    httpApi.addRoutes({
      path: "/jobs",
      methods: [apigwv2.HttpMethod.POST],
      integration: new integrations.HttpLambdaIntegration(
        "JobsHandlerIntegration",
        jobsHandler,
      ),
    });

    new cdk.CfnOutput(this, "JobsApiUrl", {
      value: httpApi.apiEndpoint,
    });
  }
}

module.exports = { SmokeFixtureStack };
