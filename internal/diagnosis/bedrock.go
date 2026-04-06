package diagnosis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"

	preflightcfg "github.com/rmukubvu/preflight/internal/config"
)

// BedrockProvider diagnoses failures using Bedrock's OpenAI-compatible
// chat completions endpoint. It uses the existing AWS credential chain —
// no new API key or SDK model-format switching required.
//
// Endpoint: https://bedrock.{region}.amazonaws.com/v1/chat/completions
// Docs: https://docs.aws.amazon.com/bedrock/latest/userguide/inference-chat-completions.html
type BedrockProvider struct {
	cfg    preflightcfg.BedrockConfig
	client *http.Client
}

func NewBedrockProvider(cfg preflightcfg.BedrockConfig) *BedrockProvider {
	return &BedrockProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *BedrockProvider) Name() string { return "bedrock" }

// Available returns true if AWS credentials are present in the environment.
func (p *BedrockProvider) Available(ctx context.Context) bool {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return false
	}
	_, err = cfg.Credentials.Retrieve(ctx)
	return err == nil
}

func (p *BedrockProvider) Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	region := p.cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	modelID := p.cfg.ModelID
	if modelID == "" {
		modelID = "amazon.nova-lite-v1:0"
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("loading AWS config: %w", err)
	}

	// Build a standard OpenAI-format request body — no model-specific branches.
	body, err := json.Marshal(map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildPrompt(req)},
		},
		"max_tokens": 1024,
	})
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("https://bedrock.%s.amazonaws.com/v1/chat/completions", region)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DiagnoseResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Sign the request with AWS SigV4. Bedrock's OpenAI-compatible endpoint
	// uses the "bedrock" service name.
	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("retrieving AWS credentials: %w", err)
	}

	payloadHash := sha256Hex(body)
	signer := v4.NewSigner()
	if err := signer.SignHTTP(ctx, creds, httpReq, payloadHash, "bedrock", region, time.Now()); err != nil {
		return DiagnoseResponse{}, fmt.Errorf("signing request: %w", err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("calling Bedrock: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return DiagnoseResponse{}, fmt.Errorf("Bedrock returned %d: %v", resp.StatusCode, errBody)
	}

	// Parse the standard OpenAI chat completions response.
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DiagnoseResponse{}, fmt.Errorf("decoding response: %w", err)
	}
	if len(result.Choices) == 0 {
		return DiagnoseResponse{}, fmt.Errorf("no choices in Bedrock response")
	}

	return parseStructuredResponse(result.Choices[0].Message.Content, p.Name()), nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// BedrockEndpoint returns the chat completions URL for the given region.
// Exported for use in tests and diagnostics.
func BedrockEndpoint(region string) string {
	return fmt.Sprintf("https://bedrock.%s.amazonaws.com/v1/chat/completions", region)
}
