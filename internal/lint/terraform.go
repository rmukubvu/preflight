package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type terraformResource struct {
	Type     string
	Name     string
	Body     *hclsyntax.Body
	Filename string
	Source   []byte
}

// EvaluateTerraformDir applies the readiness rule pack to Terraform HCL files.
func EvaluateTerraformDir(dir string) (Result, error) {
	resources, err := loadTerraformResources(dir)
	if err != nil {
		return Result{}, err
	}
	findings := evaluateTerraformResources(resources)
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity == SeverityError
		}
		if findings[i].Category != findings[j].Category {
			return findings[i].Category < findings[j].Category
		}
		if findings[i].TemplateName != findings[j].TemplateName {
			return findings[i].TemplateName < findings[j].TemplateName
		}
		if findings[i].ResourceID != findings[j].ResourceID {
			return findings[i].ResourceID < findings[j].ResourceID
		}
		return findings[i].RuleID < findings[j].RuleID
	})
	return Result{Findings: findings}, nil
}

func loadTerraformResources(dir string) ([]terraformResource, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, fmt.Errorf("listing terraform files: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no terraform files found in %s", dir)
	}

	parser := hclparse.NewParser()
	var resources []terraformResource
	for _, match := range matches {
		src, err := os.ReadFile(match)
		if err != nil {
			return nil, fmt.Errorf("reading terraform file %s: %w", filepath.Base(match), err)
		}
		file, diags := parser.ParseHCL(src, match)
		if diags.HasErrors() {
			return nil, fmt.Errorf("parsing terraform file %s: %s", filepath.Base(match), diags.Error())
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, block := range body.Blocks {
			if block.Type != "resource" || len(block.Labels) < 2 {
				continue
			}
			resources = append(resources, terraformResource{
				Type:     block.Labels[0],
				Name:     block.Labels[1],
				Body:     block.Body,
				Filename: filepath.Base(match),
				Source:   src,
			})
		}
	}
	return resources, nil
}

func evaluateTerraformResources(resources []terraformResource) []Finding {
	var findings []Finding
	if len(resources) == 0 {
		return []Finding{{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "template-empty",
			TemplateName:   "terraform",
			ResourceID:     "module",
			Message:        "Terraform module contains no resources.",
			Recommendation: "Check your module path and selected workspace before relying on the lint result.",
		}}
	}

	hasAlarm := false
	for _, resource := range resources {
		if resource.Type == "aws_cloudwatch_metric_alarm" {
			hasAlarm = true
			break
		}
	}

	for _, resource := range resources {
		switch resource.Type {
		case "aws_s3_bucket":
			findings = append(findings, terraformS3Rules(resources, resource)...)
		case "aws_sqs_queue":
			findings = append(findings, terraformSQSRules(resource)...)
		case "aws_dynamodb_table":
			findings = append(findings, terraformDynamoRules(resources, resource)...)
		case "aws_lambda_function":
			findings = append(findings, terraformLambdaRules(resources, resource)...)
		case "aws_apigatewayv2_stage", "aws_api_gateway_stage":
			findings = append(findings, terraformAPILoggingRules(resource)...)
		case "aws_apigatewayv2_route", "aws_api_gateway_method":
			findings = append(findings, terraformAPIAuthRules(resource)...)
		case "aws_sfn_state_machine":
			findings = append(findings, terraformStepFunctionsRules(resource)...)
		case "aws_cloudwatch_event_target":
			findings = append(findings, terraformEventTargetRules(resource)...)
		case "aws_sns_topic_subscription":
			findings = append(findings, terraformSNSRules(resource)...)
		case "aws_lambda_event_source_mapping":
			findings = append(findings, terraformEventSourceMappingRules(resource)...)
		case "aws_iam_role_policy", "aws_iam_policy":
			findings = append(findings, terraformIAMPolicyRules(resource)...)
		}
	}

	if !hasAlarm && terraformHasWorkload(resources) {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "alarm-coverage",
			TemplateName:   "terraform",
			ResourceID:     "module",
			Message:        "No CloudWatch alarms were declared for workload resources.",
			Recommendation: "Add at least one metric alarm for latency, errors, throttling, or backlog on the critical path.",
		})
	}

	return dedupeFindings(findings)
}

func terraformHasWorkload(resources []terraformResource) bool {
	for _, resource := range resources {
		switch resource.Type {
		case "aws_lambda_function", "aws_dynamodb_table", "aws_sqs_queue", "aws_s3_bucket", "aws_apigatewayv2_api", "aws_api_gateway_rest_api", "aws_kinesis_stream", "aws_sfn_state_machine":
			return true
		}
	}
	return false
}

func terraformS3Rules(resources []terraformResource, resource terraformResource) []Finding {
	var findings []Finding
	if !hasTerraformCompanion(resources, "aws_s3_bucket_server_side_encryption_configuration", resource.Name) {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategorySecurity,
			RuleID:         "s3-encryption",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "S3 bucket does not configure default encryption.",
			Recommendation: "Add aws_s3_bucket_server_side_encryption_configuration for this bucket.",
		})
	}
	if !hasTerraformCompanion(resources, "aws_s3_bucket_public_access_block", resource.Name) {
		findings = append(findings, Finding{
			Severity:       SeverityError,
			Category:       CategorySecurity,
			RuleID:         "s3-public-access-block",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "S3 bucket does not declare a public access block resource.",
			Recommendation: "Add aws_s3_bucket_public_access_block and enable all four block/restrict settings.",
		})
	}
	return findings
}

func terraformSQSRules(resource terraformResource) []Finding {
	if hasTerraformAttribute(resource, "redrive_policy") {
		return nil
	}
	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryReliability,
		RuleID:         "sqs-redrive-policy",
		TemplateName:   resource.Filename,
		ResourceID:     resource.Name,
		Message:        "SQS queue has no dead-letter queue configured.",
		Recommendation: "Set redrive_policy so poison messages do not block consumers indefinitely.",
	}}
}

func terraformDynamoRules(resources []terraformResource, resource terraformResource) []Finding {
	var findings []Finding
	if !hasTerraformBlock(resource, "point_in_time_recovery") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "dynamodb-pitr",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "DynamoDB table does not enable point-in-time recovery.",
			Recommendation: "Add a point_in_time_recovery block with enabled = true.",
		})
	}
	billingMode := strings.ToUpper(terraformAttrSource(resource, "billing_mode"))
	if billingMode == "" || strings.Contains(billingMode, "PROVISIONED") {
		if !hasTerraformAutoScaling(resources, resource.Name) {
			findings = append(findings, Finding{
				Severity:       SeverityWarning,
				Category:       CategoryScalability,
				RuleID:         "dynamodb-autoscaling",
				TemplateName:   resource.Filename,
				ResourceID:     resource.Name,
				Message:        "DynamoDB table uses provisioned capacity without autoscaling targets.",
				Recommendation: "Use PAY_PER_REQUEST or add aws_appautoscaling_target resources for read/write capacity.",
			})
		}
	}
	return findings
}

func terraformLambdaRules(resources []terraformResource, resource terraformResource) []Finding {
	var findings []Finding
	if !hasTerraformAttribute(resource, "timeout") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "lambda-timeout-explicit",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Lambda function does not set an explicit timeout.",
			Recommendation: "Set timeout explicitly so failure budgets stay intentional as the function evolves.",
		})
	}
	if !hasLambdaLogGroup(resources, resource.Name) {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "lambda-log-retention",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Lambda function has no explicit CloudWatch log group with retention configured.",
			Recommendation: "Add aws_cloudwatch_log_group for the function and set retention_in_days.",
		})
	}
	return findings
}

func terraformAPILoggingRules(resource terraformResource) []Finding {
	if hasTerraformBlock(resource, "access_log_settings") || hasTerraformAttribute(resource, "access_log_settings") {
		return nil
	}
	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryObservability,
		RuleID:         "api-access-logs",
		TemplateName:   resource.Filename,
		ResourceID:     resource.Name,
		Message:        "API stage does not configure access logging.",
		Recommendation: "Add access_log_settings so request failures and latency are observable.",
	}}
}

func terraformStepFunctionsRules(resource terraformResource) []Finding {
	var findings []Finding
	if !hasTerraformBlock(resource, "logging_configuration") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "sfn-logging",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Step Functions state machine does not configure execution logging.",
			Recommendation: "Add a logging_configuration block with an explicit log destination and level.",
		})
	}
	if !hasTerraformBlock(resource, "tracing_configuration") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "sfn-tracing",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Step Functions state machine does not enable tracing.",
			Recommendation: "Add tracing_configuration with enabled = true when request-path debugging matters.",
		})
	}
	definition := terraformAttrSource(resource, "definition")
	if definition != "" && !strings.Contains(definition, "TimeoutSeconds") && !strings.Contains(definition, "TimeoutSecondsPath") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "sfn-timeouts",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Step Functions state machine definition does not set task or state timeouts.",
			Recommendation: "Add TimeoutSeconds or TimeoutSecondsPath for long-running states so stuck executions fail predictably.",
		})
	}
	return findings
}

func terraformAPIAuthRules(resource terraformResource) []Finding {
	auth := strings.ToUpper(terraformAttrSource(resource, "authorization_type"))
	switch strings.TrimSpace(auth) {
	case "", "\"NONE\"", "NONE":
		return []Finding{{
			Severity:       SeverityWarning,
			Category:       CategorySecurity,
			RuleID:         "api-auth",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "API route does not configure an authorizer or non-public authorization type.",
			Recommendation: "Set authorization_type explicitly and attach an authorizer when the route is not intended to be public.",
		}}
	default:
		return nil
	}
}

func terraformEventTargetRules(resource terraformResource) []Finding {
	var findings []Finding
	if !hasTerraformBlock(resource, "retry_policy") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "eventbridge-retry-policy",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "EventBridge target does not configure retry policy.",
			Recommendation: "Add a retry_policy block so transient downstream failures do not silently vanish.",
		})
	}
	if !hasTerraformBlock(resource, "dead_letter_config") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "eventbridge-dlq",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "EventBridge target has no dead-letter queue.",
			Recommendation: "Add dead_letter_config so undeliverable events can be inspected and replayed.",
		})
	}
	return findings
}

func terraformSNSRules(resource terraformResource) []Finding {
	if hasTerraformAttribute(resource, "redrive_policy") {
		return nil
	}
	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryReliability,
		RuleID:         "sns-redrive-policy",
		TemplateName:   resource.Filename,
		ResourceID:     resource.Name,
		Message:        "SNS subscription does not configure a redrive policy.",
		Recommendation: "Set redrive_policy so failed deliveries do not disappear without an audit trail.",
	}}
}

func terraformEventSourceMappingRules(resource terraformResource) []Finding {
	source := terraformAttrSource(resource, "event_source_arn")
	if !strings.Contains(source, "kinesis") && !strings.Contains(source, "dynamodb") {
		return nil
	}
	var findings []Finding
	if !hasTerraformAttribute(resource, "maximum_retry_attempts") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "stream-retry-budget",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Stream event source mapping does not set maximum_retry_attempts.",
			Recommendation: "Set maximum_retry_attempts explicitly so poison records do not replay forever.",
		})
	}
	if !hasTerraformAttribute(resource, "bisect_batch_on_function_error") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryScalability,
			RuleID:         "stream-batch-bisect",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Stream event source mapping does not configure bisect_batch_on_function_error.",
			Recommendation: "Enable bisect_batch_on_function_error to reduce backlog amplification from a single bad record.",
		})
	}
	if !hasTerraformAttribute(resource, "maximum_batching_window_in_seconds") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryScalability,
			RuleID:         "stream-batching-window",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "Stream event source mapping does not set a batching window.",
			Recommendation: "Set maximum_batching_window_in_seconds intentionally so throughput and latency tradeoffs stay explicit.",
		})
	}
	return findings
}

func terraformIAMPolicyRules(resource terraformResource) []Finding {
	policy := terraformAttrSource(resource, "policy")
	if policy == "" {
		return nil
	}
	var findings []Finding
	if strings.Contains(policy, "\"Action\": \"*\"") || strings.Contains(policy, "\"Action\":\"*\"") || strings.Contains(policy, "Action = \"*\"") {
		findings = append(findings, Finding{
			Severity:       SeverityError,
			Category:       CategorySecurity,
			RuleID:         "iam-wildcard-action",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "IAM policy grants wildcard actions.",
			Recommendation: "Replace \"*\" actions with the smallest service/action set the workload needs.",
		})
	}
	if strings.Contains(policy, "\"Resource\": \"*\"") || strings.Contains(policy, "\"Resource\":\"*\"") || strings.Contains(policy, "Resource = \"*\"") {
		findings = append(findings, Finding{
			Severity:       SeverityError,
			Category:       CategorySecurity,
			RuleID:         "iam-wildcard-resource",
			TemplateName:   resource.Filename,
			ResourceID:     resource.Name,
			Message:        "IAM policy grants access to all resources.",
			Recommendation: "Scope IAM resources to the concrete ARNs or prefixes required by the workload.",
		})
	}
	return findings
}

func hasTerraformCompanion(resources []terraformResource, companionType, targetName string) bool {
	ref := "." + targetName
	for _, resource := range resources {
		if resource.Type != companionType {
			continue
		}
		if source := terraformAttrSource(resource, "bucket"); source != "" && strings.Contains(source, ref) {
			return true
		}
	}
	return false
}

func hasTerraformAutoScaling(resources []terraformResource, targetName string) bool {
	ref := "table/" + targetName
	for _, resource := range resources {
		if resource.Type != "aws_appautoscaling_target" {
			continue
		}
		if source := terraformAttrSource(resource, "resource_id"); source != "" && strings.Contains(source, ref) {
			return true
		}
	}
	return false
}

func hasLambdaLogGroup(resources []terraformResource, targetName string) bool {
	for _, resource := range resources {
		if resource.Type != "aws_cloudwatch_log_group" {
			continue
		}
		if source := terraformAttrSource(resource, "name"); strings.Contains(source, targetName) && hasTerraformAttribute(resource, "retention_in_days") {
			return true
		}
	}
	return false
}

func hasTerraformAttribute(resource terraformResource, name string) bool {
	_, ok := resource.Body.Attributes[name]
	return ok
}

func hasTerraformBlock(resource terraformResource, name string) bool {
	for _, block := range resource.Body.Blocks {
		if block.Type == name {
			return true
		}
	}
	return false
}

func terraformAttrSource(resource terraformResource, name string) string {
	attr, ok := resource.Body.Attributes[name]
	if !ok {
		return ""
	}
	rng := attr.Expr.Range()
	if int(rng.Start.Byte) >= len(resource.Source) || int(rng.End.Byte) > len(resource.Source) {
		return ""
	}
	return string(resource.Source[rng.Start.Byte:rng.End.Byte])
}
