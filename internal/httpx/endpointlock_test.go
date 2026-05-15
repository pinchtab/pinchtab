package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDisabledEndpointHandlerIncludesHintAndRemedy(t *testing.T) {
	handler := DisabledEndpointHandler("recording", "security.allowScreencast", "recording_disabled")

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/record/start", nil)
	handler(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var resp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if resp.Code != "recording_disabled" {
		t.Fatalf("code = %q, want recording_disabled", resp.Code)
	}

	hint, _ := resp.Details["hint"].(string)
	remedy, _ := resp.Details["remedy"].(string)

	if hint == "" {
		t.Fatal("expected non-empty hint in details")
	}
	if remedy == "" {
		t.Fatal("expected non-empty remedy in details")
	}
	if remedy != "pinchtab config set security.allowScreencast true" {
		t.Fatalf("remedy = %q, want config set command", remedy)
	}
}
