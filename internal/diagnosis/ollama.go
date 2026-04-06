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

// OllamaProvider diagnoses failures using a locally running Ollama instance.
// No API key needed; data never leaves the machine.
type OllamaProvider struct {
	cfg    config.OllamaConfig
	client *http.Client
}

func NewOllamaProvider(cfg config.OllamaConfig) *OllamaProvider {
	return &OllamaProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second}, // local models can be slow
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

// Available returns true if the Ollama server is reachable at the configured URL.
func (p *OllamaProvider) Available(ctx context.Context) bool {
	url := p.cfg.BaseURL
	if url == "" {
		url = "http://localhost:11434"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (p *OllamaProvider) Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	url := p.cfg.BaseURL
	if url == "" {
		url = "http://localhost:11434"
	}
	model := p.cfg.Model
	if model == "" {
		model = "gemma3"
	}

	body, err := json.Marshal(map[string]any{
		"model":  model,
		"prompt": buildPrompt(req),
		"stream": false,
	})
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return DiagnoseResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return DiagnoseResponse{}, fmt.Errorf("calling Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DiagnoseResponse{}, fmt.Errorf("Ollama returned %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DiagnoseResponse{}, fmt.Errorf("decoding response: %w", err)
	}

	return parseStructuredResponse(result.Response, p.Name()), nil
}
