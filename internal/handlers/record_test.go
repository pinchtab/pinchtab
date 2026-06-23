package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
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

// TestRecorderStop_CountsFrameCapturedAfterStopSignal is a race-focused
// regression test: a frame whose capture is still in flight when stop() closes
// stopCh must be counted. The gated captureFrame freezes the loop mid-capture
// (holding no lock), so stop() reaches its frame-count read while that frame has
// not yet incremented frameNum. The fix reads frameNum only after <-doneCh, so
// the in-flight frame is included; the pre-drain snapshot reported 0.
func TestRecorderStop_CountsFrameCapturedAfterStopSignal(t *testing.T) {
	rec := &recorder{}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var captured atomic.Bool
	rec.captureFrame = func(_ context.Context, _ int) ([]byte, error) {
		// Only the first capture is gated and counts; any later tick errors out
		// so the final frame count is deterministically 1.
		if captured.Swap(true) {
			return nil, fmt.Errorf("only one frame in this test")
		}
		started <- struct{}{}
		<-release
		return []byte("jpeg-bytes"), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// High fps so a ticker tick fires (starting the gated capture) almost at once.
	if err := rec.start(ctx, "tab1", "", "gif", 1000, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	<-started // capture loop is now blocked inside captureFrame, holding no lock

	type stopRes struct {
		r   RecordStopResult
		err error
	}
	resCh := make(chan stopRes, 1)
	go func() {
		r, err := rec.stop("", "") // anonymous owner; output="" → discard path (no ffmpeg)
		resCh <- stopRes{r, err}
	}()

	// Wait until stop() has entered the stopping phase. In the buggy code the
	// frameNum snapshot happens under the same lock hold that sets stateStopping,
	// so once we observe it the (stale) read has already occurred.
	for {
		rec.mu.Lock()
		st := rec.state
		rec.mu.Unlock()
		if st == stateStopping {
			break
		}
		runtime.Gosched()
	}

	close(release) // the in-flight frame completes (frameNum→1); loop then drains on stopCh

	res := <-resCh
	if res.r.Frames != 1 {
		t.Fatalf("expected 1 frame (captured as stop fired), got %d (err=%v)", res.r.Frames, res.err)
	}
	if res.err != nil {
		t.Fatalf("expected nil error on discard of 1 frame, got %v", res.err)
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

func TestAuthenticatedOwner_SessionScopedIsolatesSameAgent(t *testing.T) {
	// Regression: two sessions for the SAME agent must get DISTINCT owners so they
	// cannot stop each other's recordings (previously both collapsed to the agent ID).
	base := httptest.NewRequest("POST", "/record/start", nil)
	r1 := session.WithSession(base, &session.Session{ID: "ses_1", AgentID: "agent-x"})
	r2 := session.WithSession(base, &session.Session{ID: "ses_2", AgentID: "agent-x"})

	o1, o2 := authenticatedOwner(r1), authenticatedOwner(r2)
	if o1 == o2 {
		t.Fatalf("same-agent distinct sessions collapsed to one owner: %q == %q", o1, o2)
	}
	if o1 != "session:ses_1" || o2 != "session:ses_2" {
		t.Fatalf("expected session-scoped owners, got %q and %q", o1, o2)
	}
}

func TestAuthenticatedOwner_SessionIDWinsOverAgentID(t *testing.T) {
	r := session.WithSession(
		httptest.NewRequest("POST", "/record/start", nil),
		&session.Session{ID: "ses_9", AgentID: "agent-y"},
	)
	if got := authenticatedOwner(r); got != "session:ses_9" {
		t.Fatalf("expected session:ses_9 (session ID wins), got %q", got)
	}
}

func TestAuthenticatedOwner_AnonymousIsEmpty(t *testing.T) {
	r := httptest.NewRequest("POST", "/record/start", nil)
	if got := authenticatedOwner(r); got != "" {
		t.Fatalf("expected empty owner for anonymous request, got %q", got)
	}
}

func TestAuthenticatedOwner_TrustedProxyPrefersSessionHeader(t *testing.T) {
	base := httptest.NewRequest("POST", "/record/start", nil)
	base = base.WithContext(MarkTrustedInternalProxy(base.Context()))

	withBoth := base.Clone(base.Context())
	withBoth.Header.Set(activity.HeaderPTSessionID, "ses_p")
	withBoth.Header.Set(activity.HeaderAgentID, "agent-p")
	if got := authenticatedOwner(withBoth); got != "proxy-session:ses_p" {
		t.Fatalf("expected proxy-session:ses_p (session header preferred), got %q", got)
	}

	agentOnly := base.Clone(base.Context())
	agentOnly.Header.Set(activity.HeaderAgentID, "agent-p")
	if got := authenticatedOwner(agentOnly); got != "proxy-agent:agent-p" {
		t.Fatalf("expected proxy-agent:agent-p fallback, got %q", got)
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
