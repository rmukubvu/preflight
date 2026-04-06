package diagnosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rmukubvu/preflight/internal/config"
)

// OpenAIProvider diagnoses failures using the OpenAI Chat Completions API.
// Also supports Azure OpenAI via the BaseURL config field.
type OpenAIProvider struct {
	cfg    config.OpenAIConfig
	client *http.Client
}

func NewOpenAIProvider(cfg config.OpenAIConfig) *OpenAIProvider {
	return &OpenAIProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Available(_ context.Context) bool {
	return p.cfg.APIKey != ""
}

func (p *OpenAIProvider) Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	model := p.cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	endpoint := baseURL + "/v1/chat/completions"

	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildPrompt(req)},
		},
		"max_tokens": 1024,
	})
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DiagnoseResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("calling OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return DiagnoseResponse{}, fmt.Errorf("OpenAI API returned %d: %v", resp.StatusCode, errBody)
	}

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
		return DiagnoseResponse{}, fmt.Errorf("no choices in response")
	}

	return parseStructuredResponse(result.Choices[0].Message.Content, p.Name()), nil
}
