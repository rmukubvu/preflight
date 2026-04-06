package diagnosis

import (
	"context"
	"fmt"

	"github.com/rmukubvu/preflight/internal/config"
)

// Engine auto-selects the best available Provider using the priority chain
// defined in the design spec: Claude → Bedrock → OpenAI → Ollama → Noop.
type Engine struct {
	providers []Provider
}

// NewEngine constructs an Engine from cfg. It builds the provider chain
// and orders them by priority. Noop is always appended as the final fallback.
func NewEngine(cfg config.LLMConfig) *Engine {
	chain := buildChain(cfg)
	return &Engine{providers: chain}
}

// Diagnose walks the provider chain and uses the first available provider
// to diagnose the failure. It always succeeds because NoopProvider is the
// last entry in the chain.
func (e *Engine) Diagnose(ctx context.Context, req DiagnoseRequest) (DiagnoseResponse, error) {
	for _, p := range e.providers {
		if !p.Available(ctx) {
			continue
		}
		resp, err := p.Diagnose(ctx, req)
		if err != nil {
			// Log the failure and try the next provider.
			fmt.Printf("  diagnosis provider %q failed: %v — trying next\n", p.Name(), err)
			continue
		}
		resp.ProviderName = p.Name()
		return resp, nil
	}
	// Should be unreachable: NoopProvider is always available.
	return DiagnoseResponse{}, fmt.Errorf("no diagnosis provider available")
}

// ProviderName returns the name of the first available provider without
// running a full diagnosis. Used for display purposes.
func (e *Engine) ProviderName(ctx context.Context) string {
	for _, p := range e.providers {
		if p.Available(ctx) {
			return p.Name()
		}
	}
	return "none"
}

// buildChain constructs the ordered provider list based on config and
// environment. When provider is "auto", all providers are tried in priority
// order. When a specific provider is named, only that provider + Noop are used.
func buildChain(cfg config.LLMConfig) []Provider {
	noop := &NoopProvider{}

	if cfg.Provider == "none" {
		return []Provider{noop}
	}

	if cfg.Provider != "auto" {
		p := providerFor(cfg.Provider, cfg)
		if p != nil {
			return []Provider{p, noop}
		}
		return []Provider{noop}
	}

	// auto: build priority chain
	var chain []Provider
	for _, name := range []string{"claude", "bedrock", "openai", "ollama"} {
		if p := providerFor(name, cfg); p != nil {
			chain = append(chain, p)
		}
	}
	chain = append(chain, noop)
	return chain
}

func providerFor(name string, cfg config.LLMConfig) Provider {
	switch name {
	case "claude":
		return NewClaudeProvider(cfg.Claude)
	case "bedrock":
		return NewBedrockProvider(cfg.Bedrock)
	case "openai":
		return NewOpenAIProvider(cfg.OpenAI)
	case "ollama":
		return NewOllamaProvider(cfg.Ollama)
	}
	return nil
}
