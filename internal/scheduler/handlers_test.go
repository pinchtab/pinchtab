package scheduler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupHandlerTest(t *testing.T) (*Scheduler, *http.ServeMux, *httptest.Server) {
	t.Helper()

	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
	json.Unmarshal(w.Body.Bytes(), &resp)
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

func TestHandlerGet(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1", Action: "click", TabID: "tab-1",
	})

	req := httptest.NewRequest("GET", "/tasks/"+task.ID, nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
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

	task, _ := s.Submit(SubmitRequest{
		AgentID: "a1", Action: "click", TabID: "tab-1",
	})

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

	s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"})
	s.Submit(SubmitRequest{AgentID: "a2", Action: "navigate", TabID: "tab-2"})

	req := httptest.NewRequest("GET", "/tasks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["count"] == nil {
		t.Error("response should contain count")
	}
}

func TestHandlerListWithFilters(t *testing.T) {
	s, mux, executor := setupHandlerTest(t)
	defer executor.Close()

	s.Submit(SubmitRequest{AgentID: "a1", Action: "click", TabID: "tab-1"})
	s.Submit(SubmitRequest{AgentID: "a2", Action: "navigate", TabID: "tab-2"})

	req := httptest.NewRequest("GET", "/tasks?agentId=a1&state=queued", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("expected 1 filtered task, got %d", count)
	}
}
