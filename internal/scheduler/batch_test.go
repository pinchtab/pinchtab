package scheduler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleBatchSuccess(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{
		"agentId": "a1",
		"tasks": [
			{"action":"click","tabId":"tab-1","ref":"e1"},
			{"action":"navigate","tabId":"tab-1","ref":"e2","priority":5}
		]
	}`

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	submitted, ok := resp["submitted"].(float64)
	if !ok || submitted != 2 {
		t.Errorf("expected 2 submitted, got %v", resp["submitted"])
	}

	tasks, ok := resp["tasks"].([]any)
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected 2 task results, got %v", resp["tasks"])
	}

	for i, item := range tasks {
		m := item.(map[string]any)
		if m["taskId"] == nil || m["taskId"] == "" {
			t.Errorf("task[%d] missing taskId", i)
		}
		if m["state"] != "queued" {
			t.Errorf("task[%d] expected queued, got %v", i, m["state"])
		}
	}
}

func TestHandleBatchMissingAgentID(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"tasks":[{"action":"click","tabId":"t1"}]}`
	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBatchEmptyTasks(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"agentId":"a1","tasks":[]}`
	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBatchTooLarge(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	// Build batch with more than the default max (50) tasks.
	tasks := make([]map[string]string, 51)
	for i := range tasks {
		tasks[i] = map[string]string{"action": "click", "tabId": "t1"}
	}
	reqBody := map[string]any{
		"agentId": "a1",
		"tasks":   tasks,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for oversized batch, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBatchBadJSON(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleBatchWithCallbackURL(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{
		"agentId": "a1",
		"callbackUrl": "http://93.184.216.34/hook",
		"tasks": [
			{"action":"click","tabId":"tab-1","ref":"e1"}
		]
	}`

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBatchRejectsInvalidCallbackURL(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{
		"agentId": "a1",
		"callbackUrl": "http://127.0.0.1/hook",
		"tasks": [
			{"action":"click","tabId":"tab-1","ref":"e1"}
		]
	}`

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tasks, ok := resp["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("expected 1 task result, got %v", resp["tasks"])
	}
	item := tasks[0].(map[string]any)
	if item["state"] != "rejected" {
		t.Fatalf("expected rejected state, got %v", item["state"])
	}
	if item["error"] == nil || item["error"] == "" {
		t.Fatal("expected validation error for invalid callbackUrl")
	}
}

func TestHandleBatchPartialFailure(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	// Reduce per-agent limit to force rejection on second task.
	s.queue.SetLimits(s.cfg.MaxQueueSize, 1)

	body := `{
		"agentId": "a1",
		"tasks": [
			{"action":"click","tabId":"tab-1","ref":"e1"},
			{"action":"navigate","tabId":"tab-1","ref":"e2"}
		]
	}`

	req := httptest.NewRequest("POST", "/tasks/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should still return 202 since at least one task was accepted.
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tasks := resp["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 results, got %d", len(tasks))
	}

	first := tasks[0].(map[string]any)
	if first["state"] != "queued" {
		t.Errorf("first task should be queued, got %v", first["state"])
	}

	second := tasks[1].(map[string]any)
	if second["error"] == nil || second["error"] == "" {
		t.Error("second task should have an error due to per-agent limit")
	}
}

func TestHandleStats(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	// Submit a task so there's data.
	_, _ = s.Submit(SubmitRequest{
		AgentID: "a1",
		Action:  "click",
		TabID:   "tab-1",
	})

	req := httptest.NewRequest("GET", "/scheduler/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["queue"] == nil {
		t.Error("response should contain queue stats")
	}
	if resp["metrics"] == nil {
		t.Error("response should contain metrics")
	}
	if resp["config"] == nil {
		t.Error("response should contain config")
	}

	metrics := resp["metrics"].(map[string]any)
	if metrics["tasksSubmitted"].(float64) != 1 {
		t.Errorf("expected 1 submitted, got %v", metrics["tasksSubmitted"])
	}
}
