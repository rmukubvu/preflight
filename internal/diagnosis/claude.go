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

const (
	claudeAPIURL   = "https://api.anthropic.com/v1/messages"
	claudeVersion  = "2023-06-01"
	claudeMaxTokens = 1024
)

// ClaudeProvider diagnoses assertion failures using the Anthropic Messages API.
type ClaudeProvider struct {
	cfg    config.ClaudeConfig
	client *http.Client
}

// NewClaudeProvider constructs a ClaudeProvider. The HTTP client is overridable
// via the unexported field for testing.
func NewClaudeProvider(cfg config.ClaudeConfig) *ClaudeProvider {
	return &ClaudeProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Available(_ context.Context) bool {
	return p.cfg.APIKey != ""
}

func (p *ClaudeProvider) Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	model := p.cfg.Model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	prompt := buildPrompt(req)

	body, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": claudeMaxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeAPIURL, bytes.NewReader(body))
	if err != nil {
		return DiagnoseResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", claudeVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return DiagnoseResponse{}, fmt.Errorf("Claude API returned %d: %v", resp.StatusCode, errBody)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DiagnoseResponse{}, fmt.Errorf("decoding response: %w", err)
	}

	var text string
	for _, c := range result.Content {
		if c.Type == "text" {
			text = c.Text
			break
		}
	}

	return parseStructuredResponse(text, p.Name()), nil
}
