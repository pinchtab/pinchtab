package bench

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const AnthropicAPIURL = "https://api.anthropic.com/v1/messages"

type AnthropicRunner struct {
	apiKey        string
	model         string
	maxTokens     int
	temperature   float64
	promptCaching bool
	usage         UsageCounters
	client        *http.Client
	retryCfg      RetryConfig
}

func NewAnthropicRunner(apiKey, model string, maxTokens int, temperature float64, promptCaching bool) *AnthropicRunner {
	return &AnthropicRunner{
		apiKey:        apiKey,
		model:         model,
		maxTokens:     maxTokens,
		temperature:   temperature,
		promptCaching: promptCaching,
		client:        &http.Client{Timeout: 5 * time.Minute},
		retryCfg:      DefaultRetryConfig(),
	}
}

func (r *AnthropicRunner) Provider() string     { return "anthropic" }
func (r *AnthropicRunner) Source() string       { return "anthropic-messages" }
func (r *AnthropicRunner) Model() string        { return r.model }
func (r *AnthropicRunner) Usage() UsageCounters { return r.usage }

func (r *AnthropicRunner) ToolDefinitions() any {
	tool := map[string]any{
		"name":        "run_command",
		"description": "Run a shell command in a persistent bash session rooted at tests/tools/. Use ./scripts/pt or ./scripts/ab directly.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":         map[string]any{"type": "string"},
				"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 600},
			},
			"required": []string{"command"},
		},
	}
	// Breakpoint #1: cache system + tools together. The marker on the last
	// tool tells the API "everything up through this tool definition is a
	// cacheable prefix." System prompt and tool definitions are stable for
	// the lifetime of a run, so they become pure cache_read after turn 1.
	if r.promptCaching {
		tool["cache_control"] = map[string]string{"type": "ephemeral"}
	}
	return []map[string]any{tool}
}

func (r *AnthropicRunner) InitialConversation(userPrompt string) []any {
	content := []any{
		map[string]any{
			"type": "text",
			"text": userPrompt,
		},
	}
	// Breakpoint #2: cache the big initial task prompt. Subsequent turns
	// append to `messages` but never touch `messages[0]`, so the prefix up
	// through this block is stable and served from cache on every turn
	// after the first. Placed on the content block (not the message) because
	// Anthropic's cache_control lives on content blocks.
	if r.promptCaching {
		content[0].(map[string]any)["cache_control"] = map[string]string{"type": "ephemeral"}
	}
	return []any{
		map[string]any{
			"role":    "user",
			"content": content,
		},
	}
}

func (r *AnthropicRunner) Send(systemPrompt string, conversation []any) (any, error) {
	// System prompt stays a plain string — the cache breakpoint on the last
	// tool definition already captures the system+tools prefix. Anthropic
	// computes cache keys over system first, then tools, then messages, so
	// a marker on tools[last] covers both.
	body := map[string]any{
		"model":       r.model,
		"max_tokens":  r.maxTokens,
		"temperature": r.temperature,
		"system":      systemPrompt,
		"tools":       r.ToolDefinitions(),
		"messages":    conversation,
	}

	result, err := postJSON(context.Background(), r.client, r.retryCfg, AnthropicAPIURL, map[string]string{
		"X-Api-Key":         r.apiKey,
		"Anthropic-Version": "2023-06-01",
	}, body)
	if err != nil {
		return nil, fmt.Errorf("anthropic %w", err)
	}
	r.updateUsage(result)
	return result, nil
}

func (r *AnthropicRunner) updateUsage(result map[string]any) {
	usage, _ := result["usage"].(map[string]any)
	if usage == nil {
		return
	}
	r.usage.RequestCount++
	r.usage.InputTokens += toInt(usage["input_tokens"])
	r.usage.OutputTokens += toInt(usage["output_tokens"])
	r.usage.CacheCreationInputTokens += toInt(usage["cache_creation_input_tokens"])
	r.usage.CacheReadInputTokens += toInt(usage["cache_read_input_tokens"])
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func (r *AnthropicRunner) ExtractToolCalls(response any, defaultTimeout time.Duration) []ToolCall {
	resp, _ := response.(map[string]any)
	content, _ := resp["content"].([]any)
	var calls []ToolCall
	for _, item := range content {
		m, _ := item.(map[string]any)
		if m["type"] != "tool_use" {
			continue
		}
		input, _ := m["input"].(map[string]any)
		cmd, _ := input["command"].(string)
		if cmd == "" {
			continue
		}
		timeout := int(defaultTimeout.Seconds())
		if t := toInt(input["timeout_seconds"]); t > 0 {
			timeout = t
		}
		calls = append(calls, ToolCall{
			ID:             fmt.Sprintf("%v", m["id"]),
			Command:        cmd,
			TimeoutSeconds: timeout,
		})
	}
	return calls
}

func (r *AnthropicRunner) AppendToolResults(conversation []any, response any, results []ToolExecutionResult) []any {
	resp, _ := response.(map[string]any)
	conversation = append(conversation, map[string]any{
		"role":    "assistant",
		"content": resp["content"],
	})
	if len(results) > 0 {
		toolResults := make([]any, len(results))
		for i, res := range results {
			toolResults[i] = map[string]any{
				"type":        "tool_result",
				"tool_use_id": res.ID,
				"is_error":    res.IsError,
				"content":     res.Content,
			}
		}
		conversation = append(conversation, map[string]any{
			"role":    "user",
			"content": toolResults,
		})
	}
	return conversation
}

func (r *AnthropicRunner) ExtractFinalText(response any) string {
	resp, _ := response.(map[string]any)
	content, _ := resp["content"].([]any)
	var texts []string
	for _, item := range content {
		m, _ := item.(map[string]any)
		if m["type"] != "text" {
			continue
		}
		if text, ok := m["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}
