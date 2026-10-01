package bench

import (
	"fmt"
	"strings"
	"time"
)

type FakeResponse struct {
	ToolCalls []ToolCall
	FinalText string
}

type FakeRunner struct {
	model     string
	responses []FakeResponse
	turnIndex int
	usage     UsageCounters
}

func NewFakeRunner(model string, responses []FakeResponse) *FakeRunner {
	return &FakeRunner{
		model:     model,
		responses: responses,
	}
}

func (r *FakeRunner) Provider() string     { return "fake" }
func (r *FakeRunner) Source() string       { return "fake-runner" }
func (r *FakeRunner) Model() string        { return r.model }
func (r *FakeRunner) Usage() UsageCounters { return r.usage }

func (r *FakeRunner) ToolDefinitions() any {
	return []map[string]any{
		{
			"name":        "run_command",
			"description": "Run a shell command",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
			},
		},
	}
}

func (r *FakeRunner) InitialConversation(userPrompt string) []any {
	return []any{
		map[string]any{
			"role":    "user",
			"content": userPrompt,
		},
	}
}

func (r *FakeRunner) Send(systemPrompt string, conversation []any) (any, error) {
	r.usage.RequestCount++
	r.usage.InputTokens += 100
	r.usage.OutputTokens += 50

	if r.turnIndex >= len(r.responses) {
		return map[string]any{
			"final_text": "No more scripted responses",
		}, nil
	}

	resp := r.responses[r.turnIndex]
	r.turnIndex++

	if len(resp.ToolCalls) > 0 {
		content := make([]any, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			content[i] = map[string]any{
				"type": "tool_use",
				"id":   tc.ID,
				"input": map[string]any{
					"command":         tc.Command,
					"timeout_seconds": tc.TimeoutSeconds,
				},
			}
		}
		return map[string]any{"content": content}, nil
	}

	return map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": resp.FinalText,
			},
		},
	}, nil
}

func (r *FakeRunner) ExtractToolCalls(response any, defaultTimeout time.Duration) []ToolCall {
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

func (r *FakeRunner) AppendToolResults(conversation []any, response any, results []ToolExecutionResult) []any {
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

func (r *FakeRunner) ExtractFinalText(response any) string {
	resp, _ := response.(map[string]any)

	if text, ok := resp["final_text"].(string); ok {
		return text
	}

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
