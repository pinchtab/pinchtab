package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleRecordStart_Disabled(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: false}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	body := `{"format":"gif","fps":5}`
	req := httptest.NewRequest("POST", "/record/start", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleRecordStart(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 when recording disabled, got %d", w.Code)
	}
}

func TestHandleRecordStart_InvalidFormat(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	body := `{"format":"avi","fps":5}`
	req := httptest.NewRequest("POST", "/record/start", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleRecordStart(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for invalid format, got %d", w.Code)
	}
}

func TestHandleRecordStart_TabNotFound(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: true}
	h := New(&mockBridge{failTab: true}, cfg, nil, nil, nil)

	body := `{"format":"gif","fps":5,"tabId":"missing"}`
	req := httptest.NewRequest("POST", "/record/start", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleRecordStart(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for missing tab, got %d", w.Code)
	}
}

func TestHandleRecordStart_Success(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	body := `{"format":"gif","fps":5}`
	req := httptest.NewRequest("POST", "/record/start", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleRecordStart(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "recording" {
		t.Errorf("expected status=recording, got %v", resp["status"])
	}
	if resp["format"] != "gif" {
		t.Errorf("expected format=gif, got %v", resp["format"])
	}
	if resp["fps"] != float64(5) {
		t.Errorf("expected fps=5, got %v", resp["fps"])
	}
	if resp["tabId"] == nil || resp["tabId"] == "" {
		t.Errorf("expected tabId to be set, got %v", resp["tabId"])
	}
}

func TestRecorderStart_AlreadyRecording(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatalf("first start: %v", err)
	}

	err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0)
	if err == nil {
		t.Fatal("second start should fail")
	}
	if err.Error() != "recording already in progress" {
		t.Errorf("unexpected error: %v", err)
	}

	_, _ = rec.stop("", "")
}

func TestHandleRecordStop_NoRecording(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("POST", "/record/stop", nil)
	w := httptest.NewRecorder()
	h.HandleRecordStop(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 when no recording active, got %d", w.Code)
	}
}

func TestHandleRecordStatus_Inactive(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/record/status", nil)
	w := httptest.NewRecorder()
	h.HandleRecordStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp RecordingStatus
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Active {
		t.Errorf("expected active=false, got true")
	}
	if resp.State != "idle" {
		t.Errorf("expected state=idle, got %q", resp.State)
	}
}

func TestRecorderStatus_Active(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatalf("start: %v", err)
	}

	status := rec.status()
	if !status.Active {
		t.Errorf("expected active=true, got false")
	}
	if status.State != "recording" {
		t.Errorf("expected state=recording, got %q", status.State)
	}
	if status.Format != "gif" {
		t.Errorf("expected format=gif, got %q", status.Format)
	}
	if status.FPS != 5 {
		t.Errorf("expected fps=5, got %d", status.FPS)
	}
	if status.TabID != "tab1" {
		t.Errorf("expected tabId=tab1, got %q", status.TabID)
	}

	_, _ = rec.stop("", "")
}

func TestHandleRecordStart_ClampsInputs(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	body := `{"format":"gif","fps":60,"quality":200,"scale":3.0}`
	req := httptest.NewRequest("POST", "/record/start", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleRecordStart(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["fps"] != float64(maxFPS) {
		t.Errorf("expected fps clamped to %d, got %v", maxFPS, resp["fps"])
	}
	if resp["quality"] != float64(maxQuality) {
		t.Errorf("expected quality clamped to %d, got %v", maxQuality, resp["quality"])
	}
}

func TestRecorderStop_OwnerMismatch(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "agent-1", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	_, err := rec.stop("agent-2", "")
	if err == nil || err.Error() != "recording owned by another session" {
		t.Errorf("expected owner mismatch error, got %v", err)
	}

	_, _ = rec.stop("agent-1", "")
}

func TestRecorderStop_OwnerMatch(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "agent-1", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	// Gets past owner check; fails with "no frames captured" which is expected
	_, err := rec.stop("agent-1", "")
	if err == nil || err.Error() != "no frames captured" {
		t.Errorf("expected 'no frames captured' error, got %v", err)
	}
}

func TestRecorderStop_NoOwnerCanStopAnonymous(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	_, err := rec.stop("any-agent", "")
	if err == nil || err.Error() != "no frames captured" {
		t.Errorf("expected 'no frames captured', got %v", err)
	}
}

func TestFFmpegAvailable(t *testing.T) {
	_ = ffmpegAvailable()
}

func TestRecorderStateString(t *testing.T) {
	tests := []struct {
		state recorderState
		want  string
	}{
		{stateIdle, "idle"},
		{stateRecording, "recording"},
		{stateLimitReached, "limit_reached"},
		{stateStopping, "stopping"},
		{stateEncoding, "encoding"},
		{stateFinished, "finished"},
		{stateAborted, "aborted"},
		{recorderState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("recorderState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestRecorderStatus_Aborted(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()
	// Wait for capture loop to detect cancellation and transition to aborted.
	time.Sleep(100 * time.Millisecond)

	status := rec.status()
	if status.Active {
		t.Errorf("expected active=false after abort")
	}
	if status.State != "aborted" {
		t.Errorf("expected state=aborted, got %q", status.State)
	}
	if status.StopReason != "tab_closed" {
		t.Errorf("expected stopReason=tab_closed, got %q", status.StopReason)
	}
}

func TestRecorderStatus_IdleAfterStop(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatalf("start: %v", err)
	}

	_, _ = rec.stop("", "")

	status := rec.status()
	if status.Active {
		t.Errorf("expected active=false after stop")
	}
	if status.State != "idle" {
		t.Errorf("expected state=idle after stop+cleanup, got %q", status.State)
	}
}
