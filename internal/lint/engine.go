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
	for otherID, other := range template.Template.Resources {
		if other.Type != "AWS::Logs::LogGroup" {
			continue
		}
		if !logGroupTargetsLambda(other.Properties, logicalID) {
			continue
		}
		if numberAt(other.Properties, "RetentionInDays") > 0 {
			return nil
		}
		return []Finding{{
			Severity:       SeverityWarning,
			Category:       CategoryObservability,
			RuleID:         "lambda-log-retention",
			TemplateName:   template.Name,
			ResourceID:     otherID,
			Message:        "Lambda log group exists but does not set retention.",
			Recommendation: "Set RetentionInDays on the Lambda log group so logs stay manageable and explicit.",
		}}
	}

	return []Finding{{
		Severity:       SeverityWarning,
		Category:       CategoryObservability,
		RuleID:         "lambda-log-retention",
		TemplateName:   template.Name,
		ResourceID:     logicalID,
		Message:        "Lambda function has no explicit CloudWatch log group with retention configured.",
		Recommendation: "Create an AWS::Logs::LogGroup for the function and set RetentionInDays.",
	}}
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
