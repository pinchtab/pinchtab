package bench

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIToolDefinitions(t *testing.T) {
	r := NewOpenAIRunner("key", "gpt-5-mini", 4096, 0, true)
	tools := r.ToolDefinitions().([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("got %d tools; want 1", len(tools))
	}
	if tools[0]["name"] != "run_command" {
		t.Errorf("tool name = %v; want 'run_command'", tools[0]["name"])
	}
	if tools[0]["type"] != "function" {
		t.Errorf("tool type = %v; want 'function'", tools[0]["type"])
	}
}

func TestOpenAIInitialConversation(t *testing.T) {
	r := NewOpenAIRunner("key", "gpt-5-mini", 4096, 0, true)
	conv := r.InitialConversation("test prompt")
	if len(conv) != 1 {
		t.Fatalf("got %d messages; want 1", len(conv))
	}
	msg := conv[0].(map[string]any)
	if msg["role"] != "user" {
		t.Errorf("role = %v; want 'user'", msg["role"])
	}
	content := msg["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("got %d content parts; want 1", len(content))
	}
	part := content[0].(map[string]any)
	if part["type"] != "input_text" {
		t.Errorf("content type = %v; want 'input_text'", part["type"])
	}
}

func TestOpenAISend(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s; want POST", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Authorization = %s; want 'Bearer test-key'", auth)
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []any{
				map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": "Hello"},
				}},
			},
			"usage": map[string]any{
				"input_tokens":  100,
				"output_tokens": 50,
				"input_tokens_details": map[string]any{
					"cached_tokens": 30,
				},
			},
		})
	}))
	defer server.Close()

	r := NewOpenAIRunner("test-key", "gpt-5-mini", 4096, 0.5, true)
	r.baseURL = server.URL
	r.retryCfg.Sleep = func(d time.Duration) {}

	conv := r.InitialConversation("hello")
	_, err := r.Send("system", conv)
	if err != nil {
		t.Fatal(err)
	}

	if receivedBody["model"] != "gpt-5-mini" {
		t.Errorf("model = %v; want gpt-5-mini", receivedBody["model"])
	}
	if receivedBody["max_output_tokens"] != float64(4096) {
		t.Errorf("max_output_tokens = %v; want 4096", receivedBody["max_output_tokens"])
	}
	if receivedBody["prompt_cache_retention"] != "24h" {
		t.Errorf("prompt_cache_retention = %v; want '24h'", receivedBody["prompt_cache_retention"])
	}

	usage := r.Usage()
	if usage.RequestCount != 1 {
		t.Errorf("RequestCount = %d; want 1", usage.RequestCount)
	}
	if usage.InputTokens != 70 {
		t.Errorf("InputTokens = %d; want 70 (100 - 30 cached)", usage.InputTokens)
	}
	if usage.CacheReadInputTokens != 30 {
		t.Errorf("CacheReadInputTokens = %d; want 30", usage.CacheReadInputTokens)
	}
}

func TestOpenAIExtractToolCalls(t *testing.T) {
	r := NewOpenAIRunner("key", "model", 4096, 0, true)
	response := map[string]any{
		"output": []any{
			map[string]any{
				"type":      "function_call",
				"call_id":   "call_123",
				"arguments": `{"command": "ls -la", "timeout_seconds": 30}`,
			},
			map[string]any{
				"type": "message",
			},
		},
	}

	calls := r.ExtractToolCalls(response, 120*time.Second)
	if len(calls) != 1 {
		t.Fatalf("got %d calls; want 1", len(calls))
	}
	if calls[0].ID != "call_123" {
		t.Errorf("ID = %s; want call_123", calls[0].ID)
	}
	if calls[0].Command != "ls -la" {
		t.Errorf("Command = %s; want 'ls -la'", calls[0].Command)
	}
	if calls[0].TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d; want 30", calls[0].TimeoutSeconds)
	}
}

func TestOpenAIExtractFinalText(t *testing.T) {
	r := NewOpenAIRunner("key", "model", 4096, 0, true)

	t.Run("output_text field", func(t *testing.T) {
		response := map[string]any{
			"output_text": "Direct output",
		}
		text := r.ExtractFinalText(response)
		if text != "Direct output" {
			t.Errorf("got %q; want 'Direct output'", text)
		}
	})

	t.Run("from output array", func(t *testing.T) {
		response := map[string]any{
			"output": []any{
				map[string]any{
					"type": "message",
					"content": []any{
						map[string]any{"type": "output_text", "text": "Hello"},
						map[string]any{"type": "text", "text": "World"},
					},
				},
			},
		}
		text := r.ExtractFinalText(response)
		if text != "Hello\nWorld" {
			t.Errorf("got %q; want 'Hello\\nWorld'", text)
		}
	})
}

func TestOpenAIAppendToolResults(t *testing.T) {
	r := NewOpenAIRunner("key", "model", 4096, 0, true)
	conv := []any{}
	response := map[string]any{
		"output": []any{
			map[string]any{"type": "function_call", "call_id": "123"},
		},
	}
	results := []ToolExecutionResult{
		{ID: "123", IsError: false, Content: "output"},
	}

	conv = r.AppendToolResults(conv, response, results)
	if len(conv) != 2 {
		t.Fatalf("got %d items; want 2", len(conv))
	}

	funcOutput := conv[1].(map[string]any)
	if funcOutput["type"] != "function_call_output" {
		t.Errorf("type = %v; want 'function_call_output'", funcOutput["type"])
	}
	if funcOutput["call_id"] != "123" {
		t.Errorf("call_id = %v; want '123'", funcOutput["call_id"])
	}
}
