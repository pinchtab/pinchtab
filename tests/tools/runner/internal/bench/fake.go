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

func (r *FakeRunner) ToolDefinitions() interface{} {
	return []map[string]interface{}{
		{
			"name":        "run_command",
			"description": "Run a shell command",
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string"},
				},
			},
		},
	}
}

func (r *FakeRunner) InitialConversation(userPrompt string) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": userPrompt,
		},
	}
}

func (r *FakeRunner) Send(systemPrompt string, conversation []interface{}) (interface{}, error) {
	r.usage.RequestCount++
	r.usage.InputTokens += 100
	r.usage.OutputTokens += 50

	if r.turnIndex >= len(r.responses) {
		return map[string]interface{}{
			"final_text": "No more scripted responses",
		}, nil
	}

	resp := r.responses[r.turnIndex]
	r.turnIndex++

	if len(resp.ToolCalls) > 0 {
		content := make([]interface{}, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			content[i] = map[string]interface{}{
				"type": "tool_use",
				"id":   tc.ID,
				"input": map[string]interface{}{
					"command":         tc.Command,
					"timeout_seconds": tc.TimeoutSeconds,
				},
			}
		}
		return map[string]interface{}{"content": content}, nil
	}

	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": resp.FinalText,
			},
		},
	}, nil
}

func (r *FakeRunner) ExtractToolCalls(response interface{}, defaultTimeout time.Duration) []ToolCall {
	resp, _ := response.(map[string]interface{})
	content, _ := resp["content"].([]interface{})
	var calls []ToolCall
	for _, item := range content {
		m, _ := item.(map[string]interface{})
		if m["type"] != "tool_use" {
			continue
		}
		input, _ := m["input"].(map[string]interface{})
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

func (r *FakeRunner) AppendToolResults(conversation []interface{}, response interface{}, results []ToolExecutionResult) []interface{} {
	resp, _ := response.(map[string]interface{})
	conversation = append(conversation, map[string]interface{}{
		"role":    "assistant",
		"content": resp["content"],
	})
	if len(results) > 0 {
		toolResults := make([]interface{}, len(results))
		for i, res := range results {
			toolResults[i] = map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": res.ID,
				"is_error":    res.IsError,
				"content":     res.Content,
			}
		}
		conversation = append(conversation, map[string]interface{}{
			"role":    "user",
			"content": toolResults,
		})
	}
	return conversation
}

func (r *FakeRunner) ExtractFinalText(response interface{}) string {
	resp, _ := response.(map[string]interface{})

	if text, ok := resp["final_text"].(string); ok {
		return text
	}

	content, _ := resp["content"].([]interface{})
	var texts []string
	for _, item := range content {
		m, _ := item.(map[string]interface{})
		if m["type"] != "text" {
			continue
		}
		if text, ok := m["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}
