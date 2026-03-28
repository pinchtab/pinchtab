// Package llm provides the last-resort LLM fallback for the autosolver
// system. It is used ONLY when all semantic and built-in solvers fail.
//
// The provider interface is designed to minimize token usage by sending
// trimmed HTML and structured context rather than full DOM snapshots.
package llm

import (
	"context"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

// ProviderConfig holds LLM provider configuration.
type ProviderConfig struct {
	Provider    string  `json:"provider"`    // "openai", "anthropic", etc.
	Model       string  `json:"model"`       // Model name (e.g., "gpt-4o-mini")
	APIKey      string  `json:"apiKey"`      // Provider API key
	MaxTokens   int     `json:"maxTokens"`   // Max output tokens (default: 256)
	Temperature float64 `json:"temperature"` // Sampling temperature (default: 0.1)
}

// Provider implements autosolver.LLMProvider as a skeleton.
// The actual HTTP client for OpenAI/Anthropic must be implemented
// based on the chosen provider.
type Provider struct {
	config ProviderConfig
}

// NewProvider creates an LLM provider with the given configuration.
func NewProvider(cfg ProviderConfig) *Provider {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 256
	}
	if cfg.Temperature <= 0 {
		cfg.Temperature = 0.1
	}
	return &Provider{config: cfg}
}

// SuggestNextAction builds a structured prompt from the page context and
// asks the LLM for the next action.
//
// The prompt is designed to be token-efficient:
//   - Page title + URL (always included)
//   - Trimmed HTML (scripts/styles removed, max ~4000 chars)
//   - Previous attempt summary (what failed and why)
//   - Structured output format (action type + parameters)
func (p *Provider) SuggestNextAction(ctx context.Context, req autosolver.LLMRequest) (*autosolver.LLMResponse, error) {
	if p.config.APIKey == "" {
		return nil, fmt.Errorf("llm: API key not configured for provider %q", p.config.Provider)
	}

	// Build the prompt.
	prompt := buildPrompt(req)

	// TODO: Implement actual LLM API call based on p.config.Provider.
	// For now, return an error indicating skeleton-only status.
	_ = prompt

	return nil, fmt.Errorf("llm: provider %q not yet implemented — skeleton only", p.config.Provider)
}

// buildPrompt creates a structured prompt for the LLM.
func buildPrompt(req autosolver.LLMRequest) string {
	prompt := fmt.Sprintf(`You are a browser automation assistant. Analyze the following page and suggest the next action.

Page Title: %s
Page URL: %s
Detected Type: %s

Page HTML (trimmed):
%s

`, req.PageTitle, req.PageURL, req.DetectedType, req.TrimmedHTML)

	if len(req.PrevAttempts) > 0 {
		prompt += "Previous failed attempts:\n"
		for _, a := range req.PrevAttempts {
			prompt += fmt.Sprintf("- Solver: %s, Status: %s, Error: %s\n",
				a.Solver, a.Status, a.Error)
		}
		prompt += "\n"
	}

	prompt += `Respond with a JSON object:
{
  "action": "click" | "type" | "wait" | "navigate" | "none",
  "selector": "CSS selector if action=click",
  "text": "text to type if action=type",
  "url": "URL if action=navigate",
  "reasoning": "brief explanation",
  "confidence": 0.0-1.0
}`

	return prompt
}
