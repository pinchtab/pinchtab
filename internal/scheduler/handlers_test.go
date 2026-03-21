package scheduler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func setupHandlerTest(t *testing.T) (*Scheduler, *http.ServeMux, *httptest.Server) {
	t.Helper()

	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			t.Errorf("encode failed: %v", err)
		}
	}))

	parts := strings.Split(executor.URL, ":")
	port := parts[len(parts)-1]

	cfg := DefaultConfig()
	cfg.WorkerCount = 1

	s := New(cfg, &mockResolver{port: port})

	mux := http.NewServeMux()
	s.RegisterHandlers(mux)

	return s, mux, executor
}

func TestHandlerSubmit(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"agentId":"agent-1","action":"click","tabId":"tab-1","ref":"e14"}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["taskId"] == nil {
		t.Error("response should contain taskId")
	}
	if resp["state"] != "queued" {
		t.Errorf("expected state queued, got %v", resp["state"])
	}
}

func TestHandlerSubmit_Invalid(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"action":"click"}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerSubmit_InvalidCallbackURL(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"agentId":"agent-1","action":"click","callbackUrl":"http://127.0.0.1/hook"}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerSubmitBadJSON(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	req := httptest.NewRequest("POST", "/tasks", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlerSubmitBodyTooLarge(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	body := `{"agentId":"agent-1","action":"click","ref":"` + strings.Repeat("x", int(httpx.DefaultMaxJSONBodyBytes)) + `"}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGet(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	task, err := s.Submit(SubmitRequest{
		AgentID: "a1", Action: "click", TabID: "tab-1",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/tasks/"+task.ID, nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["taskId"] != task.ID {
		t.Errorf("expected task ID %s, got %v", task.ID, resp["taskId"])
	}
}

func TestHandlerGetNotFound(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	req := httptest.NewRequest("GET", "/tasks/nonexistent", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerCancel(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	task, err := s.Submit(SubmitRequest{
		AgentID: "a1", Action: "click", TabID: "tab-1",
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	req := httptest.NewRequest("POST", "/tasks/"+task.ID+"/cancel", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCancelNotFound(t *testing.T) {
	_, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	req := httptest.NewRequest("POST", "/tasks/nonexistent/cancel", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerList(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	if _, err := s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if _, err := s.Submit(SubmitRequest{AgentID: "a2", Action: "navigate", TabID: "tab-2"}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/tasks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["count"] == nil {
		t.Error("response should contain count")
	}
}

func TestHandlerListWithFilters(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	if _, err := s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if _, err := s.Submit(SubmitRequest{AgentID: "a2", Action: "navigate", TabID: "tab-2"}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/tasks?agentId=a1&state=queued", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("expected 1 filtered task, got %d", count)
	}
}
