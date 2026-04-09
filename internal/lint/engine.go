package lint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity describes the importance of a finding.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Category groups findings by readiness dimension.
type Category string

const (
	CategoryScalability   Category = "scalability"
	CategoryReliability   Category = "reliability"
	CategoryObservability Category = "observability"
	CategorySecurity      Category = "security"
)

// Finding is a single lint result.
type Finding struct {
	Severity       Severity
	Category       Category
	RuleID         string
	TemplateName   string
	ResourceID     string
	Message        string
	Recommendation string
}

// Result is the full lint output.
type Result struct {
	Findings []Finding
}

// CategoryScore summarizes heuristic readiness for one category.
type CategoryScore struct {
	Category Category `json:"category"`
	Score    int      `json:"score"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
}

// HasErrors reports whether any error-severity findings exist.
func (r Result) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasFindings reports whether any findings exist.
func (r Result) HasFindings() bool {
	return len(r.Findings) > 0
}

// CountsByCategory returns finding counts grouped by category.
func (r Result) CountsByCategory() map[Category]int {
	counts := make(map[Category]int)
	for _, finding := range r.Findings {
		counts[finding.Category]++
	}
	return counts
}

// Scores returns heuristic readiness scores by category.
func (r Result) Scores() []CategoryScore {
	order := []Category{
		CategorySecurity,
		CategoryReliability,
		CategoryObservability,
		CategoryScalability,
	}

	type bucket struct {
		errors   int
		warnings int
	}
	buckets := make(map[Category]bucket)
	for _, finding := range r.Findings {
		b := buckets[finding.Category]
		if finding.Severity == SeverityError {
			b.errors++
		} else {
			b.warnings++
		}
		buckets[finding.Category] = b
	}

	scores := make([]CategoryScore, 0, len(order))
	for _, category := range order {
		b := buckets[category]
		score := 100 - (35 * b.errors) - (12 * b.warnings)
		if score < 0 {
			score = 0
		}
		scores = append(scores, CategoryScore{
			Category: category,
			Score:    score,
			Errors:   b.errors,
			Warnings: b.warnings,
		})
	}
	return scores
}

// TemplateFile is a synthesized CloudFormation template ready for analysis.
type TemplateFile struct {
	Name     string
	Template Template
}

// Template captures the CloudFormation structures needed for readiness linting.
type Template struct {
	Resources map[string]Resource `json:"Resources"`
}

// Resource is a single CloudFormation resource.
type Resource struct {
	Type       string                 `json:"Type"`
	Properties map[string]interface{} `json:"Properties"`
}

// LoadTemplates reads all *.template.json files from dir.
func LoadTemplates(dir string) ([]TemplateFile, error) {
	pattern := filepath.Join(dir, "*.template.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("listing synthesized templates: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no synthesized CloudFormation templates found in %s", dir)
	}

	templates := make([]TemplateFile, 0, len(matches))
	for _, match := range matches {
		data, err := os.ReadFile(match)
		if err != nil {
			return nil, fmt.Errorf("reading template %s: %w", filepath.Base(match), err)
		}

		var tpl Template
		if err := json.Unmarshal(data, &tpl); err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", filepath.Base(match), err)
		}

		templates = append(templates, TemplateFile{
			Name:     filepath.Base(match),
			Template: tpl,
		})
	}

	return templates, nil
}

// EvaluateTemplates applies the readiness rule pack to synthesized templates.
func EvaluateTemplates(templates []TemplateFile) Result {
	var findings []Finding

	for _, template := range templates {
		findings = append(findings, evaluateTemplate(template)...)
	}

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

	return Result{Findings: findings}
}

func evaluateTemplate(template TemplateFile) []Finding {
	if len(template.Template.Resources) == 0 {
		return []Finding{{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "template-empty",
			TemplateName:   template.Name,
			ResourceID:     "template",
			Message:        "Synthesized template contains no resources.",
			Recommendation: "Check your CDK app or stack selection before relying on the lint result.",
		}}
	}

	var findings []Finding
	findings = append(findings, globalAlarmCoverageRule(template)...)

	for logicalID, resource := range template.Template.Resources {
		switch resource.Type {
		case "AWS::S3::Bucket":
			findings = append(findings, s3ProtectionRules(template.Name, logicalID, resource)...)
		case "AWS::SQS::Queue":
			findings = append(findings, sqsReliabilityRules(template.Name, logicalID, resource)...)
		case "AWS::DynamoDB::Table":
			findings = append(findings, dynamoReliabilityRules(template, logicalID, resource)...)
		case "AWS::Lambda::Function":
			findings = append(findings, lambdaObservabilityRules(template, logicalID, resource)...)
		case "AWS::ApiGatewayV2::Stage", "AWS::ApiGateway::Stage":
			findings = append(findings, apiLoggingRules(template.Name, logicalID, resource)...)
		case "AWS::ApiGatewayV2::Route", "AWS::ApiGateway::Method":
			findings = append(findings, apiAuthRules(template.Name, logicalID, resource)...)
		case "AWS::StepFunctions::StateMachine":
			findings = append(findings, stepFunctionsRules(template.Name, logicalID, resource)...)
		case "AWS::Events::Rule":
			findings = append(findings, eventBridgeTargetRules(template.Name, logicalID, resource)...)
		case "AWS::SNS::Subscription":
			findings = append(findings, snsSubscriptionRules(template.Name, logicalID, resource)...)
		case "AWS::Lambda::EventSourceMapping":
			findings = append(findings, streamEventSourceRules(template, logicalID, resource)...)
		case "AWS::IAM::Role":
			findings = append(findings, iamRoleRules(template.Name, logicalID, resource)...)
		case "AWS::IAM::Policy", "AWS::IAM::ManagedPolicy":
			findings = append(findings, iamPolicyDocumentRules(template.Name, logicalID, resource)...)
		}
	}

	return findings
}

func globalAlarmCoverageRule(template TemplateFile) []Finding {
	hasAlarm := false
	hasWorkload := false
	for _, resource := range template.Template.Resources {
		if resource.Type == "AWS::CloudWatch::Alarm" {
			hasAlarm = true
		}
		if isWorkloadResource(resource.Type) {
			hasWorkload = true
		}
	}

	if !hasWorkload || hasAlarm {
		return nil
	}

	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryObservability,
		RuleID:         "alarm-coverage",
		TemplateName:   template.Name,
		ResourceID:     "template",
		Message:        "No CloudWatch alarms were synthesized for workload resources.",
		Recommendation: "Add at least one alarm for latency, errors, throttling, or backlog on the critical path.",
	}}
}

func isWorkloadResource(resourceType string) bool {
	switch resourceType {
	case "AWS::Lambda::Function",
		"AWS::DynamoDB::Table",
		"AWS::SQS::Queue",
		"AWS::S3::Bucket",
		"AWS::ApiGatewayV2::Api",
		"AWS::ApiGateway::RestApi",
		"AWS::Kinesis::Stream",
		"AWS::StepFunctions::StateMachine":
		return true
	default:
		return false
	}
}

func s3ProtectionRules(templateName, logicalID string, resource Resource) []Finding {
	var findings []Finding
	encryption := nestedMap(resource.Properties, "BucketEncryption")
	if len(encryption) == 0 {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategorySecurity,
			RuleID:         "s3-encryption",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "S3 bucket does not configure default encryption.",
			Recommendation: "Add BucketEncryption so data at rest is protected by default.",
		})
	}

	publicAccess := nestedMap(resource.Properties, "PublicAccessBlockConfiguration")
	if len(publicAccess) == 0 || !boolAt(publicAccess, "BlockPublicAcls") || !boolAt(publicAccess, "BlockPublicPolicy") || !boolAt(publicAccess, "IgnorePublicAcls") || !boolAt(publicAccess, "RestrictPublicBuckets") {
		findings = append(findings, Finding{
			Severity:       SeverityError,
			Category:       CategorySecurity,
			RuleID:         "s3-public-access-block",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "S3 bucket does not fully block public access.",
			Recommendation: "Enable all four PublicAccessBlockConfiguration flags unless the bucket is intentionally public.",
		})
	}

	return findings
}

func sqsReliabilityRules(templateName, logicalID string, resource Resource) []Finding {
	if len(nestedMap(resource.Properties, "RedrivePolicy")) != 0 {
		return nil
	}
	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryReliability,
		RuleID:         "sqs-redrive-policy",
		TemplateName:   templateName,
		ResourceID:     logicalID,
		Message:        "SQS queue has no dead-letter queue configured.",
		Recommendation: "Add RedrivePolicy so poison messages do not block consumers indefinitely.",
	}}
}

func dynamoReliabilityRules(template TemplateFile, logicalID string, resource Resource) []Finding {
	var findings []Finding
	pitr := nestedMap(resource.Properties, "PointInTimeRecoverySpecification")
	if !boolAt(pitr, "PointInTimeRecoveryEnabled") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "dynamodb-pitr",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "DynamoDB table does not enable point-in-time recovery.",
			Recommendation: "Enable PointInTimeRecoverySpecification.PointInTimeRecoveryEnabled for safer recovery from bad writes.",
		})
	}

	billingMode := strings.ToUpper(stringAt(resource.Properties, "BillingMode"))
	if billingMode == "" {
		billingMode = "PROVISIONED"
	}
	if billingMode == "PROVISIONED" && !hasDynamoAutoScalingTarget(template.Template.Resources, logicalID) {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryScalability,
			RuleID:         "dynamodb-autoscaling",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "DynamoDB table uses provisioned capacity without autoscaling targets.",
			Recommendation: "Use PAY_PER_REQUEST or add Application Auto Scaling targets and policies for read/write capacity.",
		})
	}

	return findings
}

func lambdaObservabilityRules(template TemplateFile, logicalID string, _ Resource) []Finding {
	var findings []Finding
	resource := template.Template.Resources[logicalID]
	if _, ok := resource.Properties["Timeout"]; !ok {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "lambda-timeout-explicit",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "Lambda function does not set an explicit timeout.",
			Recommendation: "Set Timeout explicitly so failure budgets stay intentional as the function evolves.",
		})
	}

	for otherID, other := range template.Template.Resources {
		if other.Type != "AWS::Logs::LogGroup" {
			continue
		}
		if !logGroupTargetsLambda(other.Properties, logicalID) {
			continue
		}
		if numberAt(other.Properties, "RetentionInDays") > 0 {
			return findings
		}
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "lambda-log-retention",
			TemplateName:   template.Name,
			ResourceID:     otherID,
			Message:        "Lambda log group exists but does not set retention.",
			Recommendation: "Set RetentionInDays on the Lambda log group so logs stay manageable and explicit.",
		})
		return findings
	}

	findings = append(findings, Finding{
		Severity:       SeverityWarning,
		Category:       CategoryObservability,
		RuleID:         "lambda-log-retention",
		TemplateName:   template.Name,
		ResourceID:     logicalID,
		Message:        "Lambda function has no explicit CloudWatch log group with retention configured.",
		Recommendation: "Create an AWS::Logs::LogGroup for the function and set RetentionInDays.",
	})
	return findings
}

func apiLoggingRules(templateName, logicalID string, resource Resource) []Finding {
	key := "AccessLogSettings"
	if resource.Type == "AWS::ApiGateway::Stage" {
		key = "AccessLogSetting"
	}
	if len(nestedMap(resource.Properties, key)) != 0 {
		return nil
	}

	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryObservability,
		RuleID:         "api-access-logs",
		TemplateName:   templateName,
		ResourceID:     logicalID,
		Message:        "API stage does not configure access logging.",
		Recommendation: "Add access log settings so request failures and latency are observable without reproducing them locally.",
	}}
}

func iamRoleRules(templateName, logicalID string, resource Resource) []Finding {
	var findings []Finding
	for _, policy := range arrayAt(resource.Properties, "Policies") {
		policyMap := asMap(policy)
		findings = append(findings, lintPolicyDocument(templateName, logicalID, nestedMap(policyMap, "PolicyDocument"))...)
	}
	return findings
}

func iamPolicyDocumentRules(templateName, logicalID string, resource Resource) []Finding {
	return lintPolicyDocument(templateName, logicalID, nestedMap(resource.Properties, "PolicyDocument"))
}

func stepFunctionsRules(templateName, logicalID string, resource Resource) []Finding {
	var findings []Finding
	logging := nestedMap(resource.Properties, "LoggingConfiguration")
	if len(logging) == 0 || strings.EqualFold(stringAt(logging, "Level"), "OFF") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "sfn-logging",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "Step Functions state machine does not configure execution logging.",
			Recommendation: "Add LoggingConfiguration with an explicit log destination and level.",
		})
	}

	tracing := nestedMap(resource.Properties, "TracingConfiguration")
	if !boolAt(tracing, "Enabled") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "sfn-tracing",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "Step Functions state machine does not enable tracing.",
			Recommendation: "Enable X-Ray tracing on the state machine when request-path debugging matters.",
		})
	}

	definition := flattenStringLike(resource.Properties["DefinitionString"])
	if definition != "" && !strings.Contains(definition, "TimeoutSeconds") && !strings.Contains(definition, "TimeoutSecondsPath") {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "sfn-timeouts",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "Step Functions state machine definition does not set task or state timeouts.",
			Recommendation: "Add TimeoutSeconds or TimeoutSecondsPath for long-running states so stuck executions fail predictably.",
		})
	}

	return findings
}

func apiAuthRules(templateName, logicalID string, resource Resource) []Finding {
	authType := strings.ToUpper(strings.TrimSpace(stringAt(resource.Properties, "AuthorizationType")))
	switch authType {
	case "", "NONE":
		return []Finding{{
			Severity:       SeverityWarning,
			Category:       CategorySecurity,
			RuleID:         "api-auth",
			TemplateName:   templateName,
			ResourceID:     logicalID,
			Message:        "API route does not configure an authorizer or non-public authorization type.",
			Recommendation: "Set AuthorizationType explicitly and attach an authorizer when the route is not intended to be public.",
		}}
	default:
		return nil
	}
}

func eventBridgeTargetRules(templateName, logicalID string, resource Resource) []Finding {
	var findings []Finding
	for _, targetValue := range arrayAt(resource.Properties, "Targets") {
		target := asMap(targetValue)
		if len(target) == 0 {
			continue
		}
		targetID := logicalID
		if id := stringAt(target, "Id"); id != "" {
			targetID = logicalID + ":" + id
		}
		if len(nestedMap(target, "RetryPolicy")) == 0 {
			findings = append(findings, Finding{
				Severity:       SeverityWarning,
				Category:       CategoryReliability,
				RuleID:         "eventbridge-retry-policy",
				TemplateName:   templateName,
				ResourceID:     targetID,
				Message:        "EventBridge target does not configure retry policy.",
				Recommendation: "Set RetryPolicy so transient downstream failures do not silently vanish.",
			})
		}
		if len(nestedMap(target, "DeadLetterConfig")) == 0 {
			findings = append(findings, Finding{
				Severity:       SeverityWarning,
				Category:       CategoryReliability,
				RuleID:         "eventbridge-dlq",
				TemplateName:   templateName,
				ResourceID:     targetID,
				Message:        "EventBridge target has no dead-letter queue.",
				Recommendation: "Add DeadLetterConfig so undeliverable events can be inspected and replayed.",
			})
		}
	}
	return findings
}

func snsSubscriptionRules(templateName, logicalID string, resource Resource) []Finding {
	protocol := strings.ToLower(stringAt(resource.Properties, "Protocol"))
	switch protocol {
	case "lambda", "sqs", "http", "https":
	default:
		return nil
	}
	if _, ok := resource.Properties["RedrivePolicy"]; ok {
		return nil
	}
	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryReliability,
		RuleID:         "sns-redrive-policy",
		TemplateName:   templateName,
		ResourceID:     logicalID,
		Message:        "SNS subscription does not configure a redrive policy.",
		Recommendation: "Add RedrivePolicy so failed deliveries do not disappear without an audit trail.",
	}}
}

func streamEventSourceRules(template TemplateFile, logicalID string, resource Resource) []Finding {
	if !isStreamEventSource(template.Template.Resources, resource.Properties["EventSourceArn"]) {
		return nil
	}

	var findings []Finding
	if _, ok := resource.Properties["MaximumRetryAttempts"]; !ok {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryReliability,
			RuleID:         "stream-retry-budget",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "Stream event source mapping does not set MaximumRetryAttempts.",
			Recommendation: "Set MaximumRetryAttempts explicitly so replay behavior under poison records is bounded.",
		})
	}
	if _, ok := resource.Properties["BisectBatchOnFunctionError"]; !ok {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryScalability,
			RuleID:         "stream-batch-bisect",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "Stream event source mapping does not configure BisectBatchOnFunctionError.",
			Recommendation: "Enable BisectBatchOnFunctionError to reduce backlog amplification from a single bad record.",
		})
	}
	if _, ok := resource.Properties["MaximumBatchingWindowInSeconds"]; !ok {
		findings = append(findings, Finding{
			Severity:       SeverityWarning,
			Category:       CategoryScalability,
			RuleID:         "stream-batching-window",
			TemplateName:   template.Name,
			ResourceID:     logicalID,
			Message:        "Stream event source mapping does not set a batching window.",
			Recommendation: "Set MaximumBatchingWindowInSeconds intentionally so throughput/latency tradeoffs stay explicit.",
		})
	}
	return findings
}

func lintPolicyDocument(templateName, logicalID string, document map[string]interface{}) []Finding {
	var findings []Finding
	for _, stmt := range arrayish(document["Statement"]) {
		statement := asMap(stmt)
		if hasExactWildcard(statement["Action"]) || hasExactWildcard(statement["NotAction"]) {
			findings = append(findings, Finding{
				Severity:       SeverityError,
				Category:       CategorySecurity,
				RuleID:         "iam-wildcard-action",
				TemplateName:   templateName,
				ResourceID:     logicalID,
				Message:        "IAM policy grants wildcard actions.",
				Recommendation: "Replace \"*\" actions with the smallest service/action set the workload needs.",
			})
		}
		if hasExactWildcard(statement["Resource"]) || hasExactWildcard(statement["NotResource"]) {
			findings = append(findings, Finding{
				Severity:       SeverityError,
				Category:       CategorySecurity,
				RuleID:         "iam-wildcard-resource",
				TemplateName:   templateName,
				ResourceID:     logicalID,
				Message:        "IAM policy grants access to all resources.",
				Recommendation: "Scope IAM resources to the concrete ARNs or prefixes required by the workload.",
			})
		}
	}
	return dedupeFindings(findings)
}

func dedupeFindings(findings []Finding) []Finding {
	seen := make(map[string]bool)
	out := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := string(finding.Severity) + "|" + string(finding.Category) + "|" + finding.RuleID + "|" + finding.TemplateName + "|" + finding.ResourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, finding)
	}
	return out
}

func hasDynamoAutoScalingTarget(resources map[string]Resource, logicalID string) bool {
	for _, resource := range resources {
		if resource.Type != "AWS::ApplicationAutoScaling::ScalableTarget" {
			continue
		}
		resourceID := stringAt(resource.Properties, "ResourceId")
		if strings.Contains(resourceID, "table/"+logicalID) || referencesLogicalID(resource.Properties["ResourceId"], logicalID) {
			return true
		}
	}
	return false
}

func isStreamEventSource(resources map[string]Resource, value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, ":kinesis:") || strings.Contains(v, "stream/")
	case map[string]interface{}:
		if getAtt, ok := v["Fn::GetAtt"].([]interface{}); ok && len(getAtt) > 0 {
			if logicalID, ok := getAtt[0].(string); ok {
				resource, exists := resources[logicalID]
				return exists && (resource.Type == "AWS::Kinesis::Stream" || resource.Type == "AWS::DynamoDB::Table")
			}
		}
		if ref, ok := v["Ref"].(string); ok {
			resource, exists := resources[ref]
			return exists && (resource.Type == "AWS::Kinesis::Stream" || resource.Type == "AWS::DynamoDB::Table")
		}
		for _, item := range v {
			if isStreamEventSource(resources, item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if isStreamEventSource(resources, item) {
				return true
			}
		}
	}
	return false
}

func logGroupTargetsLambda(properties map[string]interface{}, logicalID string) bool {
	logGroupName := properties["LogGroupName"]
	if referencesLogicalID(logGroupName, logicalID) {
		return true
	}
	if rendered := flattenStringLike(logGroupName); strings.Contains(rendered, "/aws/lambda/") && strings.Contains(rendered, logicalID) {
		return true
	}
	return false
}

func referencesLogicalID(value interface{}, logicalID string) bool {
	switch v := value.(type) {
	case map[string]interface{}:
		if ref, ok := v["Ref"].(string); ok && ref == logicalID {
			return true
		}
		for _, item := range v {
			if referencesLogicalID(item, logicalID) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if referencesLogicalID(item, logicalID) {
				return true
			}
		}
	case string:
		return strings.Contains(v, logicalID)
	}
	return false
}

func flattenStringLike(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			parts = append(parts, flattenStringLike(item))
		}
		return strings.Join(parts, "")
	case map[string]interface{}:
		if join, ok := v["Fn::Join"].([]interface{}); ok && len(join) == 2 {
			return flattenStringLike(join[1])
		}
		if sub, ok := v["Fn::Sub"].(string); ok {
			return sub
		}
		if ref, ok := v["Ref"].(string); ok {
			return ref
		}
		if getAtt, ok := v["Fn::GetAtt"].([]interface{}); ok && len(getAtt) > 0 {
			return flattenStringLike(getAtt[0])
		}
	}
	return ""
}

func nestedMap(src map[string]interface{}, key string) map[string]interface{} {
	return asMap(src[key])
}

func asMap(v interface{}) map[string]interface{} {
	if out, ok := v.(map[string]interface{}); ok {
		return out
	}
	return nil
}

func arrayAt(src map[string]interface{}, key string) []interface{} {
	return arrayish(src[key])
}

func arrayish(v interface{}) []interface{} {
	switch typed := v.(type) {
	case []interface{}:
		return typed
	case nil:
		return nil
	default:
		return []interface{}{typed}
	}
}

func stringAt(src map[string]interface{}, key string) string {
	if value, ok := src[key].(string); ok {
		return value
	}
	return ""
}

func boolAt(src map[string]interface{}, key string) bool {
	if value, ok := src[key].(bool); ok {
		return value
	}
	return false
}

func numberAt(src map[string]interface{}, key string) int {
	switch v := src[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

func hasExactWildcard(value interface{}) bool {
	for _, candidate := range arrayish(value) {
		if s, ok := candidate.(string); ok && strings.TrimSpace(s) == "*" {
			return true
		}
	}
	return false
}
