package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const OpenAIResponsesURL = "https://api.openai.com/v1/responses"

type OpenAIRunner struct {
	baseURL       string
	apiKey        string
	model         string
	maxTokens     int
	temperature   float64
	promptCaching bool
	usage         UsageCounters
	client        *http.Client
	retryCfg      RetryConfig
}

func NewOpenAIRunner(apiKey, model string, maxTokens int, temperature float64, promptCaching bool) *OpenAIRunner {
	return &OpenAIRunner{
		baseURL:       OpenAIResponsesURL,
		apiKey:        apiKey,
		model:         model,
		maxTokens:     maxTokens,
		temperature:   temperature,
		promptCaching: promptCaching,
		client:        &http.Client{Timeout: 5 * time.Minute},
		retryCfg:      DefaultRetryConfig(),
	}
}

func (r *OpenAIRunner) Provider() string     { return "openai" }
func (r *OpenAIRunner) Source() string       { return "openai-responses" }
func (r *OpenAIRunner) Model() string        { return r.model }
func (r *OpenAIRunner) Usage() UsageCounters { return r.usage }

func (r *OpenAIRunner) ToolDefinitions() any {
	return []map[string]any{
		{
			"type":        "function",
			"name":        "run_command",
			"description": "Run a shell command in a persistent bash session rooted at tests/tools/. Use ./scripts/pt or ./scripts/ab directly.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":         map[string]any{"type": "string"},
					"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 600},
				},
				"required": []string{"command"},
			},
		},
	}
}

func (r *OpenAIRunner) InitialConversation(userPrompt string) []any {
	return []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": userPrompt},
			},
		},
	}
}

func (r *OpenAIRunner) Send(systemPrompt string, conversation []any) (any, error) {
	body := map[string]any{
		"model":             r.model,
		"instructions":      systemPrompt,
		"tools":             r.ToolDefinitions(),
		"input":             conversation,
		"max_output_tokens": r.maxTokens,
	}
	if r.promptCaching {
		body["prompt_cache_retention"] = "24h"
	}

	result, err := postJSON(context.Background(), r.client, r.retryCfg, r.baseURL, map[string]string{
		"Authorization": "Bearer " + r.apiKey,
	}, body)
	if err != nil {
		return nil, fmt.Errorf("OpenAI %w", err)
	}
	r.updateUsage(result)
	return result, nil
}

func (r *OpenAIRunner) updateUsage(result map[string]any) {
	usage, _ := result["usage"].(map[string]any)
	if usage == nil {
		return
	}
	r.usage.RequestCount++

	totalInput := toInt(usage["input_tokens"])
	cached := 0
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		cached = toInt(details["cached_tokens"])
	}
	r.usage.InputTokens += max(0, totalInput-cached)
	r.usage.OutputTokens += toInt(usage["output_tokens"])
	r.usage.CacheReadInputTokens += cached
}

func (r *OpenAIRunner) ExtractToolCalls(response any, defaultTimeout time.Duration) []ToolCall {
	resp, _ := response.(map[string]any)
	output, _ := resp["output"].([]any)
	var calls []ToolCall
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m["type"] != "function_call" {
			continue
		}
		argsStr, _ := m["arguments"].(string)
		var args map[string]any
		if argsStr != "" {
			// On parse failure args stays nil and the tool call is skipped below.
			_ = json.Unmarshal([]byte(argsStr), &args)
		}
		cmd, _ := args["command"].(string)
		if cmd == "" {
			continue
		}
		timeout := int(defaultTimeout.Seconds())
		if t := toInt(args["timeout_seconds"]); t > 0 {
			timeout = t
		}
		callID := fmt.Sprintf("%v", m["call_id"])
		if callID == "" || callID == "<nil>" {
			callID = fmt.Sprintf("%v", m["id"])
		}
		calls = append(calls, ToolCall{
			ID:             callID,
			Command:        cmd,
			TimeoutSeconds: timeout,
		})
	}
	return calls
}

func (r *OpenAIRunner) AppendToolResults(conversation []any, response any, results []ToolExecutionResult) []any {
	resp, _ := response.(map[string]any)
	output, _ := resp["output"].([]any)
	conversation = append(conversation, output...)
	for _, res := range results {
		conversation = append(conversation, map[string]any{
			"type":    "function_call_output",
			"call_id": res.ID,
			"output":  res.Content,
		})
	}
	return conversation
}

func (r *OpenAIRunner) ExtractFinalText(response any) string {
	resp, _ := response.(map[string]any)

	if text, ok := resp["output_text"].(string); ok && text != "" {
		return text
	}

	output, _ := resp["output"].([]any)
	var texts []string
	for _, item := range output {
		m, _ := item.(map[string]any)
		if m["type"] != "message" {
			continue
		}
		content, _ := m["content"].([]any)
		for _, part := range content {
			p, _ := part.(map[string]any)
			ptype, _ := p["type"].(string)
			if ptype == "output_text" || ptype == "text" {
				if text, ok := p["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
	}
	return strings.Join(texts, "\n")
}
