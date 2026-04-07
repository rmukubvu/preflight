package assertions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	awsclient "github.com/rmukubvu/preflight/pkg/aws"
)

// APIGatewayHTTPCheck makes a real HTTP request to the deployed API Gateway
// and asserts the response status code matches the expected value.
// This is the primary end-to-end behavioural check for HTTP APIs.
type APIGatewayHTTPCheck struct {
	apiID          string
	method         string
	path           string // e.g. "/items" or "/"
	body           []byte // optional request body
	expectedStatus int
	headers        map[string]string
	httpClient     *http.Client
	fallbackLambda string
}

// NewAPIGatewayHTTPCheck constructs an HTTP check assertion.
// body may be nil for GET/DELETE requests.
func NewAPIGatewayHTTPCheck(apiID, method, path string, body []byte, expectedStatus int) *APIGatewayHTTPCheck {
	return &APIGatewayHTTPCheck{
		apiID:          apiID,
		method:         method,
		path:           normalizeHTTPPath(path),
		body:           body,
		expectedStatus: expectedStatus,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

// WithHeaders applies request headers to the HTTP check.
func (a *APIGatewayHTTPCheck) WithHeaders(headers map[string]string) *APIGatewayHTTPCheck {
	if len(headers) == 0 {
		a.headers = nil
		return a
	}

	a.headers = make(map[string]string, len(headers))
	for k, v := range headers {
		a.headers[k] = v
	}
	return a
}

// WithHTTPClient overrides the HTTP client used to perform the request.
func (a *APIGatewayHTTPCheck) WithHTTPClient(httpClient *http.Client) *APIGatewayHTTPCheck {
	if httpClient != nil {
		a.httpClient = httpClient
	}
	return a
}

// WithIntegrationLambda configures an optional Lambda integration fallback
// used when the local emulator cannot expose the API Gateway invoke path.
func (a *APIGatewayHTTPCheck) WithIntegrationLambda(functionName string) *APIGatewayHTTPCheck {
	a.fallbackLambda = strings.TrimSpace(functionName)
	return a
}

func (a *APIGatewayHTTPCheck) Name() string {
	return fmt.Sprintf("apigw-http:%s %s%s", a.method, a.apiID, a.path)
}

func (a *APIGatewayHTTPCheck) Category() Category { return CategoryBehavioural }

func (a *APIGatewayHTTPCheck) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	baseURL, err := client.APIGatewayV2InvokeURL(ctx, a.apiID)
	if err != nil {
		return Result{}, fmt.Errorf("getting invoke URL for API %q: %w", a.apiID, err)
	}

	url := strings.TrimRight(baseURL, "/") + a.path

	var bodyReader io.Reader
	if len(a.body) > 0 {
		bodyReader = bytes.NewReader(a.body)
	}

	req, err := http.NewRequestWithContext(ctx, a.method, url, bodyReader)
	if err != nil {
		return Result{}, fmt.Errorf("building HTTP request: %w", err)
	}
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}
	if len(a.body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		if a.fallbackLambda != "" {
			return a.runLambdaFallback(ctx, client, fmt.Sprintf("HTTP %s %s failed: %v", a.method, url, err))
		}
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %s %s failed: %v", a.method, url, err),
		}, nil
	}
	defer resp.Body.Close()

	bodyPreview := readBodyPreview(resp.Body)
	passed := resp.StatusCode == a.expectedStatus
	message := fmt.Sprintf(
		"HTTP %s %s → %d (expected %d)",
		a.method, url, resp.StatusCode, a.expectedStatus,
	)
	if !passed && bodyPreview != "" {
		message = fmt.Sprintf("%s, body=%q", message, bodyPreview)
	}
	if !passed && a.shouldUseLambdaFallback(resp.StatusCode, bodyPreview) {
		return a.runLambdaFallback(ctx, client, message)
	}

	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     passed,
		Message:    message,
		ResourceID: a.apiID,
	}, nil
}

func (a *APIGatewayHTTPCheck) shouldUseLambdaFallback(statusCode int, bodyPreview string) bool {
	if a.fallbackLambda == "" {
		return false
	}
	if statusCode == http.StatusBadRequest && strings.Contains(bodyPreview, "POST requires either ?uploads") {
		return true
	}
	return false
}

func (a *APIGatewayHTTPCheck) runLambdaFallback(ctx context.Context, client awsclient.Client, cause string) (Result, error) {
	payload, err := client.LambdaInvoke(ctx, a.fallbackLambda, buildAPIGatewayV2Event(a.method, a.path, a.body, a.headers))
	if err != nil {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  fmt.Sprintf("%s; fallback invoke of %q failed: %v", cause, a.fallbackLambda, err),
		}, nil
	}

	statusCode, err := lambdaHTTPStatusCode(payload)
	if err != nil {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  fmt.Sprintf("%s; fallback invoke of %q returned unreadable payload: %s", cause, a.fallbackLambda, string(payload)),
		}, nil
	}

	passed := statusCode == a.expectedStatus
	message := fmt.Sprintf(
		"%s; fallback invoke %q → %d (expected %d)",
		cause, a.fallbackLambda, statusCode, a.expectedStatus,
	)

	return Result{
		Name:       a.Name(),
		Category:   a.Category(),
		Passed:     passed,
		Message:    message,
		ResourceID: a.apiID,
	}, nil
}

// SQSToLambdaToDynamoDB is a synthetic end-to-end assertion.
// It sends a message to an SQS queue, waits for the Lambda consumer to
// process it, then verifies the expected side-effect in DynamoDB.
//
// This exercises the most common IaC failure pattern: ESM is attached,
// but the Lambda role is missing sqs:ReceiveMessage so messages pile up silently.
type SQSToLambdaToDynamoDB struct {
	queueName    string
	messageBody  string            // JSON body to enqueue
	tableName    string            // DynamoDB table to check
	expectedKey  map[string]string // attribute filter to look for
	pollInterval time.Duration
	pollTimeout  time.Duration
	consumerFn   string
}

const (
	consumerFallbackAttempts = 3
	consumerFallbackDelay    = 2 * time.Second
)

// NewSQSToLambdaToDynamoDB constructs the end-to-end chain assertion.
// expectedKey is a map of DynamoDB attribute name → expected string value
// that proves the Lambda processed the message (e.g. {"id": "test-123"}).
func NewSQSToLambdaToDynamoDB(
	queueName, messageBody, tableName string,
	expectedKey map[string]string,
) *SQSToLambdaToDynamoDB {
	return &SQSToLambdaToDynamoDB{
		queueName:    queueName,
		messageBody:  messageBody,
		tableName:    tableName,
		expectedKey:  expectedKey,
		pollInterval: 500 * time.Millisecond,
		pollTimeout:  15 * time.Second,
	}
}

// WithPolling overrides the default DynamoDB polling cadence used by the
// end-to-end queue assertion.
func (a *SQSToLambdaToDynamoDB) WithPolling(interval, timeout time.Duration) *SQSToLambdaToDynamoDB {
	if interval > 0 {
		a.pollInterval = interval
	}
	if timeout > 0 {
		a.pollTimeout = timeout
	}
	return a
}

// WithConsumerLambda configures an optional Lambda consumer fallback used when
// the local emulator does not deliver SQS events via event source mappings.
func (a *SQSToLambdaToDynamoDB) WithConsumerLambda(functionName string) *SQSToLambdaToDynamoDB {
	a.consumerFn = strings.TrimSpace(functionName)
	return a
}

func (a *SQSToLambdaToDynamoDB) Name() string {
	return fmt.Sprintf("e2e-sqs-lambda-ddb:%s→%s", a.queueName, a.tableName)
}

func (a *SQSToLambdaToDynamoDB) Category() Category { return CategoryBehavioural }

func (a *SQSToLambdaToDynamoDB) Run(ctx context.Context, client awsclient.Client) (Result, error) {
	// ── 1. Get the queue URL ──────────────────────────────────────────────────
	queueURL, err := client.SQSQueueURL(ctx, a.queueName)
	if err != nil {
		return Result{}, fmt.Errorf("getting queue URL: %w", err)
	}

	// ── 2. Send the test message ──────────────────────────────────────────────
	if err := client.SQSSendMessage(ctx, queueURL, a.messageBody); err != nil {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  fmt.Sprintf("failed to send SQS message to %q: %v", a.queueName, err),
		}, nil
	}

	if item, err := a.waitForExpectedItem(ctx, client, a.pollTimeout); item != nil {
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   true,
			Message: fmt.Sprintf(
				"SQS message processed: item appeared in DynamoDB table %q",
				a.tableName,
			),
			ResourceID: a.queueName,
		}, nil
	} else if a.consumerFn != "" {
		return a.runConsumerFallback(ctx, client, err)
	} else {
		message := fmt.Sprintf(
			"SQS message sent to %q but item did not appear in DynamoDB table %q within %s; check Lambda ESM state and execution role permissions",
			a.queueName, a.tableName, a.pollTimeout,
		)
		if err != nil {
			message = fmt.Sprintf("%s (last DynamoDB error: %v)", message, err)
		}
		return Result{
			Name:     a.Name(),
			Category: a.Category(),
			Passed:   false,
			Message:  message,
		}, nil
	}
}

func (a *SQSToLambdaToDynamoDB) waitForExpectedItem(ctx context.Context, client awsclient.Client, timeout time.Duration) (map[string]string, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	var lastGetItemErr error
	for {
		item, err := client.DynamoDBGetItem(ctx, a.tableName, a.expectedKey)
		if err == nil && item != nil {
			return item, nil
		}
		if err != nil {
			lastGetItemErr = err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, lastGetItemErr
		case <-ticker.C:
		}
	}
}

func (a *SQSToLambdaToDynamoDB) runConsumerFallback(ctx context.Context, client awsclient.Client, lastGetItemErr error) (Result, error) {
	payload := buildSQSEventPayload(a.messageBody)
	var lastInvokeErr error
	for attempt := 1; attempt <= consumerFallbackAttempts; attempt++ {
		if _, err := client.LambdaInvoke(ctx, a.consumerFn, payload); err == nil {
			if item, pollErr := a.waitForExpectedItem(ctx, client, a.pollInterval*4); item != nil {
				return Result{
					Name:       a.Name(),
					Category:   a.Category(),
					Passed:     true,
					Message:    fmt.Sprintf("SQS message processed after fallback invoke of %q: item appeared in DynamoDB table %q", a.consumerFn, a.tableName),
					ResourceID: a.queueName,
				}, nil
			} else if pollErr != nil {
				lastGetItemErr = pollErr
			}
			break
		} else {
			lastInvokeErr = err
		}

		if item, err := client.DynamoDBGetItem(ctx, a.tableName, a.expectedKey); err == nil && item != nil {
			return Result{
				Name:       a.Name(),
				Category:   a.Category(),
				Passed:     true,
				Message:    fmt.Sprintf("SQS message processed after fallback invoke retry of %q: item appeared in DynamoDB table %q", a.consumerFn, a.tableName),
				ResourceID: a.queueName,
			}, nil
		}

		if attempt == consumerFallbackAttempts {
			break
		}
		select {
		case <-ctx.Done():
			lastInvokeErr = ctx.Err()
			attempt = consumerFallbackAttempts
		case <-time.After(consumerFallbackDelay):
		}
	}

	if item, err := client.DynamoDBGetItem(ctx, a.tableName, a.expectedKey); err == nil && item != nil {
		return Result{
			Name:       a.Name(),
			Category:   a.Category(),
			Passed:     true,
			Message:    fmt.Sprintf("SQS message processed after fallback invoke of %q: item appeared in DynamoDB table %q", a.consumerFn, a.tableName),
			ResourceID: a.queueName,
		}, nil
	} else if err != nil {
		lastGetItemErr = err
	}

	message := fmt.Sprintf("SQS message sent to %q but fallback invoke of %q failed", a.queueName, a.consumerFn)
	if lastInvokeErr == nil {
		message = fmt.Sprintf(
			"SQS message sent to %q and fallback invoke of %q completed, but item still did not appear in DynamoDB table %q",
			a.queueName, a.consumerFn, a.tableName,
		)
	} else {
		message = fmt.Sprintf("%s after %d attempt(s): %v", message, consumerFallbackAttempts, lastInvokeErr)
	}
	if lastGetItemErr != nil {
		message = fmt.Sprintf("%s (last DynamoDB error: %v)", message, lastGetItemErr)
	}

	return Result{
		Name:     a.Name(),
		Category: a.Category(),
		Passed:   false,
		Message:  message,
	}, nil
}

func normalizeHTTPPath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func readBodyPreview(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 512))
	if err != nil {
		return ""
	}
	return string(data)
}

func buildAPIGatewayV2Event(method, path string, body []byte, headers map[string]string) []byte {
	eventHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		eventHeaders[k] = v
	}
	if len(body) > 0 && eventHeaders["content-type"] == "" && eventHeaders["Content-Type"] == "" {
		eventHeaders["content-type"] = "application/json"
	}

	event := map[string]any{
		"version":         "2.0",
		"routeKey":        method + " " + path,
		"rawPath":         path,
		"headers":         eventHeaders,
		"isBase64Encoded": false,
		"requestContext": map[string]any{
			"http": map[string]any{
				"method": method,
				"path":   path,
			},
		},
	}
	if len(body) > 0 {
		event["body"] = string(body)
	}

	data, _ := json.Marshal(event)
	return data
}

func lambdaHTTPStatusCode(payload []byte) (int, error) {
	var response struct {
		StatusCode int `json:"statusCode"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return 0, err
	}
	if response.StatusCode == 0 {
		return 0, fmt.Errorf("missing statusCode")
	}
	return response.StatusCode, nil
}

func buildSQSEventPayload(messageBody string) []byte {
	event := map[string]any{
		"Records": []map[string]string{
			{
				"body": messageBody,
			},
		},
	}
	data, _ := json.Marshal(event)
	return data
}
