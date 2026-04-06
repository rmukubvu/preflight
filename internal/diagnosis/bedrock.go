package diagnosis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	preflightcfg "github.com/rmukubvu/preflight/internal/config"
)

// BedrockProvider diagnoses failures using AWS Bedrock InvokeModel.
// It uses the existing AWS credential chain — no new API key required.
type BedrockProvider struct {
	cfg preflightcfg.BedrockConfig
}

func NewBedrockProvider(cfg preflightcfg.BedrockConfig) *BedrockProvider {
	return &BedrockProvider{cfg: cfg}
}

func (p *BedrockProvider) Name() string { return "bedrock" }

// Available returns true if AWS credentials are present in the environment.
// It attempts to load the default config; if that fails, credentials are absent.
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

	client := bedrockruntime.NewFromConfig(awsCfg)
	prompt := buildPrompt(req)

	// Build the request body. Format varies by model family.
	var body []byte
	if isNovaModel(modelID) {
		body, err = json.Marshal(map[string]any{
			"messages": []map[string]any{
				{"role": "user", "content": []map[string]string{{"text": prompt}}},
			},
			"inferenceConfig": map[string]int{"maxTokens": 1024},
		})
	} else {
		// Claude on Bedrock format
		body, err = json.Marshal(map[string]any{
			"anthropic_version": "bedrock-2023-05-31",
			"max_tokens":        1024,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		})
	}
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("marshaling request: %w", err)
	}

	out, err := client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		Body:        body,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("invoking Bedrock model: %w", err)
	}

	text, err := extractBedrockText(out.Body, modelID)
	if err != nil {
		return DiagnoseResponse{}, err
	}

	return parseStructuredResponse(text, p.Name()), nil
}

func isNovaModel(modelID string) bool {
	return len(modelID) > 6 && modelID[:6] == "amazon"
}

func extractBedrockText(body []byte, modelID string) (string, error) {
	if isNovaModel(modelID) {
		var resp struct {
			Output struct {
				Message struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"output"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", fmt.Errorf("decoding Nova response: %w", err)
		}
		if len(resp.Output.Message.Content) > 0 {
			return resp.Output.Message.Content[0].Text, nil
		}
		return "", fmt.Errorf("empty Nova response")
	}

	// Claude on Bedrock format
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decoding Claude-on-Bedrock response: %w", err)
	}
	for _, c := range resp.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text in Bedrock response")
}
