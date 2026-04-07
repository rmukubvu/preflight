const cdk = require("aws-cdk-lib");
const { SmokeFixtureStack } = require("../lib/smoke-fixture-stack");

const app = new cdk.App();

new SmokeFixtureStack(app, "SmokeFixtureStack", {
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT || "000000000000",
    region: process.env.CDK_DEFAULT_REGION || "us-east-1",
  },
});
