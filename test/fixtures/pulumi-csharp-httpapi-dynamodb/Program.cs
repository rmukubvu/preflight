using System;
using System.Collections.Generic;
using Pulumi;
using Aws = Pulumi.Aws;

return await Deployment.RunAsync(() =>
{
    var lambdaZipPath = Environment.GetEnvironmentVariable("LAMBDA_ZIP_PATH") ?? "lambda/function.zip";

    var provider = new Aws.Provider("stratus", new Aws.ProviderArgs
    {
        AccessKey = "test",
        SecretKey = "test",
        Region = "us-east-1",
        S3UsePathStyle = true,
        SkipCredentialsValidation = true,
        SkipMetadataApiCheck = true,
        SkipRegionValidation = true,
        SkipRequestingAccountId = true,
    });

    var providerOpts = new CustomResourceOptions
    {
        Provider = provider
    };

    var function = new Aws.Lambda.Function("itemsFunction", new Aws.Lambda.FunctionArgs
    {
        Name = "pulumi-csharp-items-handler",
        Role = "arn:aws:iam::000000000000:role/pulumi-csharp-items-role",
        Runtime = "nodejs20.x",
        Handler = "index.handler",
        Code = new FileArchive(lambdaZipPath),
    }, providerOpts);

    var api = new Aws.ApiGatewayV2.Api("httpApi", new Aws.ApiGatewayV2.ApiArgs
    {
        Name = "pulumi-csharp-http-api",
        ProtocolType = "HTTP"
    }, providerOpts);

    var integration = new Aws.ApiGatewayV2.Integration("itemsIntegration", new Aws.ApiGatewayV2.IntegrationArgs
    {
        ApiId = api.Id,
        IntegrationType = "AWS_PROXY",
        IntegrationUri = function.Arn,
        PayloadFormatVersion = "2.0"
    }, providerOpts);

    var route = new Aws.ApiGatewayV2.Route("itemsRoute", new Aws.ApiGatewayV2.RouteArgs
    {
        ApiId = api.Id,
        RouteKey = "POST /items",
        Target = integration.Id.Apply(id => $"integrations/{id}")
    }, providerOpts);

    var stage = new Aws.ApiGatewayV2.Stage("defaultStage", new Aws.ApiGatewayV2.StageArgs
    {
        ApiId = api.Id,
        Name = "$default",
        AutoDeploy = true
    }, providerOpts);

    var permission = new Aws.Lambda.Permission("apiInvokePermission", new Aws.Lambda.PermissionArgs
    {
        Action = "lambda:InvokeFunction",
        Function = function.Name,
        Principal = "apigateway.amazonaws.com",
        SourceArn = api.ExecutionArn.Apply(arn => $"{arn}/*/*")
    }, providerOpts);

    return new Dictionary<string, object?>
    {
        ["apiId"] = api.Id,
        ["apiEndpoint"] = api.ApiEndpoint,
        ["routeId"] = route.Id,
        ["stageName"] = stage.Name,
        ["functionName"] = function.Name,
        ["permissionId"] = permission.Id
    };
});
