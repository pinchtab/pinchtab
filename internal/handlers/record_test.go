package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
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

// captureNothing is the stub for fixtures that assert state or ownership rather than
// frames. stop() now waits out the first frame interval when no frame has arrived, so the
// capture loop reaches its first tick in these tests where it previously never did — and a
// nil captureFrame is called there.
func captureNothing(context.Context, int) ([]byte, error) {
	return nil, fmt.Errorf("no capture in this test")
}

func TestRecorderStart_AlreadyRecording(t *testing.T) {
	rec := &recorder{captureFrame: captureNothing}
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
	// A real StateDir: the stop path reserves its output name on disk, so an empty
	// StateDir would write into the package directory.
	cfg := &config.RuntimeConfig{StateDir: t.TempDir(), AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("POST", "/record/stop", nil)
	w := httptest.NewRecorder()
	h.HandleRecordStop(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 when no recording active, got %d", w.Code)
	}
}

func TestHandleRecordStatus_Inactive(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowScreencast: true}
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
	rec := &recorder{captureFrame: captureNothing}
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
	rec := &recorder{captureFrame: captureNothing}
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
	rec := &recorder{captureFrame: captureNothing}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "agent-1", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	// Gets past the owner check; the capture never succeeds, so the frameless refusal
	// is expected.
	_, err := rec.stop("agent-1", "")
	if err == nil || !strings.HasPrefix(err.Error(), "no frames captured") {
		t.Errorf("expected the frameless refusal, got %v", err)
	}
}

// The defect: a stop issued straight after start refused with "no frames captured" and
// DISCARDED the recording, because the first tick had not arrived yet. That is the shape
// every script has — start, one fast action, stop — so the commonest use of the recorder
// was the one that lost its output. The capture here always succeeds, so the only thing
// that can produce zero frames is stopping before the first tick.
func TestRecorderStopImmediatelyAfterStartWaitsForTheFirstFrame(t *testing.T) {
	rec := &recorder{captureFrame: func(context.Context, int) ([]byte, error) {
		return []byte("jpeg-bytes"), nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	res, err := rec.stop("", "")

	if err != nil {
		t.Fatalf("a stop straight after start was refused: %v", err)
	}
	if res.Frames < 1 {
		t.Fatalf("frames = %d; the recording was discarded for not having reached its first tick", res.Frames)
	}
}

// A recording that already has a frame must not pay the wait — the fix is for the case
// before the first tick, not a delay on every stop.
func TestRecorderStopDoesNotWaitOnceAFrameExists(t *testing.T) {
	captured := make(chan struct{}, 1)
	rec := &recorder{captureFrame: func(context.Context, int) ([]byte, error) {
		select {
		case captured <- struct{}{}:
		default:
		}
		return []byte("jpeg-bytes"), nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1 fps, so the wait this test must not observe would be a full second.
	if err := rec.start(ctx, "tab1", "", "gif", 1, 80, 1.0); err != nil {
		t.Fatal(err)
	}
	<-captured

	start := time.Now()
	if _, err := rec.stop("", ""); err != nil {
		t.Fatalf("stop with a frame in hand was refused: %v", err)
	}
	if waited := time.Since(start); waited > frameInterval(1)/2 {
		t.Errorf("stop took %s with a frame already captured, so it waited for a first frame it already had", waited)
	}
}

// The refusal has to survive: every arm names the elapsed time, the rate, and what one
// frame costs at that rate, because those three decide the next attempt. Driven through
// stop() as well as the table, since a table alone cannot show the handler passes the
// recording's own fps rather than a constant.
func TestNoFramesRefusalNamesTheElapsedTimeTheRateAndTheFrameCost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		elapsed    time.Duration
		fps        int
		stopReason string
		wantSays   []string
	}{
		{name: "stopped before the first frame", elapsed: 12 * time.Millisecond, fps: 5, wantSays: []string{"12ms", "5 fps", "200ms", "record for longer"}},
		{name: "ran long enough and captured nothing", elapsed: 3 * time.Second, fps: 10, wantSays: []string{"3s", "10 fps", "100ms", "tab is still open"}},
		{name: "the loop ended early", elapsed: 40 * time.Millisecond, fps: 5, stopReason: "tab_closed", wantSays: []string{"40ms", "5 fps", "200ms", "tab_closed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := noFramesError(tc.elapsed, tc.fps, tc.stopReason).Error()
			for _, want := range tc.wantSays {
				if !strings.Contains(message, want) {
					t.Errorf("refusal %q does not name %q", message, want)
				}
			}
		})
	}

	rec := &recorder{captureFrame: func(context.Context, int) ([]byte, error) {
		return nil, fmt.Errorf("this capture never succeeds")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	_, err := rec.stop("", "")

	if err == nil {
		t.Fatal("a recording that captured nothing was accepted")
	}
	for _, want := range []string{"5 fps", "200ms"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %q, so it carries a constant rather than this recording's own rate", err.Error(), want)
		}
	}
}

// TestRecorderStop_CountsFrameCapturedAfterStopSignal is a race-focused
// regression test: a frame whose capture is still in flight when stop() closes
// stopCh must be counted. The gated captureFrame freezes the loop mid-capture
// (holding no lock), so stop() reaches its frame-count read while that frame has
// not yet incremented frameNum. The fix reads frameNum only after <-doneCh, so
// the in-flight frame is included; the pre-drain snapshot reported 0.
func TestRecorderStop_CountsFrameCapturedAfterStopSignal(t *testing.T) {
	rec := &recorder{captureFrame: captureNothing}

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
	rec := &recorder{captureFrame: captureNothing}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rec.start(ctx, "tab1", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatal(err)
	}

	_, err := rec.stop("any-agent", "")
	if err == nil || !strings.HasPrefix(err.Error(), "no frames captured") {
		t.Errorf("expected the frameless refusal, got %v", err)
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
	rec := &recorder{captureFrame: captureNothing}
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
	rec := &recorder{captureFrame: captureNothing}
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

func jpegFrame(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// The tab a recording films can vanish — closed, or its browser died — and the
// frames must survive that: the state names the reason, the log carries it, and
// stop() encodes what was captured to the path the caller names. The zero-frame
// idle case is the one this must differ from, so it is asserted beside it.
func TestATabClosingMidRecordingKeepsTheFramesRecoverable(t *testing.T) {
	logs := captureLogs(t)
	frame := jpegFrame(t)
	captured := make(chan struct{}, 1)
	rec := &recorder{captureFrame: func(context.Context, int) ([]byte, error) {
		select {
		case captured <- struct{}{}:
		default:
		}
		return frame, nil
	}}
	tabCtx, closeTab := context.WithCancel(context.Background())
	if err := rec.start(tabCtx, "tab-filmed", "", "gif", 30, 80, 1.0); err != nil {
		t.Fatal(err)
	}
	<-captured
	closeTab()
	<-rec.doneCh

	st := rec.status()
	if st.Active || st.State != "aborted" || st.StopReason != "tab_closed" || st.Frames < 1 || st.TabID != "tab-filmed" {
		t.Fatalf("status after the tab closed = %+v; it must say the recording ended early, why, and keep its frames", st)
	}
	if idle := (&recorder{}).status(); idle.State == st.State {
		t.Fatalf("an ended recording (%q) reads like one never started (%q)", st.State, idle.State)
	}
	for _, want := range []string{"recording ended early", "tab=tab-filmed", "reason=tab_closed", "frames="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, logs.String())
		}
	}

	out := filepath.Join(t.TempDir(), "kept.gif")
	res, err := rec.stop("", out)
	if err != nil {
		t.Fatalf("stop after the tab closed refused instead of encoding the kept frames: %v", err)
	}
	if res.Frames < 1 {
		t.Fatalf("stop reported %d frames", res.Frames)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := rec.status()
		if s.State == "finished" {
			if s.Error != "" {
				t.Fatalf("encode failed: %s", s.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("encode never finished: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Fatalf("the recording was not written to %s: %v", out, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := rec.start(ctx, "tab-next", "", "gif", 5, 80, 1.0); err != nil {
		t.Fatalf("a new recording after an early end was refused: %v", err)
	}
	_, _ = rec.stop("", "")
}

// Ended before its first frame, the recording still says so: stop names the
// reason instead of answering as if nothing had been started.
func TestATabClosingBeforeTheFirstFrameNamesTheReasonOnStop(t *testing.T) {
	rec := &recorder{captureFrame: captureNothing}
	tabCtx, closeTab := context.WithCancel(context.Background())
	if err := rec.start(tabCtx, "tab-filmed", "", "gif", 1, 80, 1.0); err != nil {
		t.Fatal(err)
	}
	closeTab()
	<-rec.doneCh

	_, err := rec.stop("", filepath.Join(t.TempDir(), "never.gif"))
	if err == nil || !strings.Contains(err.Error(), "ended early (tab_closed)") {
		t.Fatalf("stop on a recording ended before its first frame = %v; want the reason named", err)
	}
	if _, idleErr := (&recorder{}).stop("", ""); idleErr == nil || idleErr.Error() == err.Error() {
		t.Fatalf("the ended-early answer %q is indistinguishable from never started %v", err, idleErr)
	}
}
