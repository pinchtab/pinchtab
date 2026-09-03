package activity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeOnlyDir returns a directory that can be written into but not enumerated,
// which is what makes the retention sweep fail while the append still works.
// Skips where that separation does not exist.
func writeOnlyDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("directory read and write permissions are not separable here")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses the permission that arranges this")
	}
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.Mkdir(dir, 0o300); err != nil {
		t.Fatalf("create write-only dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("the directory is still enumerable, so this arranges no failing sweep")
	}
	return dir
}

// A retention sweep is housekeeping, and housekeeping must not cost the event.
// Aborting Record on the sweep's error threw away an observation the append would
// have stored perfectly well — and since the throttle advances whether or not the
// sweep succeeded, the loss was one event per hour, landing on whichever request
// happened to trip the timer.
func TestARetentionFailureStillRecordsTheEvent(t *testing.T) {
	dir := writeOnlyDir(t)
	store := &Store{dir: dir, retentionDays: 1, events: EventSourceConfig{Server: true}}

	if err := store.pruneExpiredFiles(time.Now().UTC()); err == nil {
		t.Fatal("the sweep succeeded, so this test no longer arranges the failure it is about")
	}

	store.lastPruneTime = time.Time{}
	if err := store.Record(Event{Source: "server", Method: "GET", Path: "/recorded"}); err != nil {
		t.Fatalf("Record returned %v; a failing retention sweep must not fail the event", err)
	}
	if !appended(t, dir, "/recorded") {
		t.Error("the event was not appended, so the sweep's failure cost the observation")
	}
}

// The append genuinely works in this directory, so the assertion above is about
// the sweep rather than about an unwritable directory.
func TestTheWriteOnlyDirectoryStillAcceptsAppends(t *testing.T) {
	dir := writeOnlyDir(t)
	if err := appendJSONL(filepath.Join(dir, "probe.jsonl"), []byte(`{"path":"/probe"}`)); err != nil {
		t.Fatalf("append into the write-only dir: %v; the retention test would pass for the wrong reason", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "probe.jsonl"))
	if err != nil {
		t.Fatalf("read back the probe: %v", err)
	}
	if !strings.Contains(string(data), "/probe") {
		t.Errorf("probe file holds %q, want the appended line", data)
	}
}

// appended reads the files Record writes BY NAME: the directory under test is
// deliberately not enumerable, so a listing would find nothing whatever was written.
func appended(t *testing.T, dir, path string) bool {
	t.Helper()
	day := time.Now().UTC().Format(time.DateOnly)
	for _, name := range []string{"events.jsonl", "events-server-" + day + ".jsonl"} {
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr == nil && strings.Contains(string(data), `"`+path+`"`) {
			return true
		}
	}
	return false
}

// countingHandler counts log records at or above warn, so a test can assert how
// LOUD a sustained failure is without matching on message text.
type countingHandler struct {
	slog.Handler
	warns int
	infos int
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	switch {
	case r.Level >= slog.LevelWarn:
		h.warns++
	case r.Level >= slog.LevelInfo:
		h.infos++
	}
	return nil
}

func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

type toggleRecorder struct{ failing bool }

func (t *toggleRecorder) Enabled() bool { return true }
func (t *toggleRecorder) Record(Event) error {
	if t.failing {
		return errors.New("cannot record")
	}
	return nil
}
func (t *toggleRecorder) Query(Filter) ([]Event, error) { return nil, nil }

// Recording must never fail a request, so the middleware swallows the error — but
// swallowing it silently is how the feed goes empty with nothing saying why, and
// warning on every request is how the warning stops being read. It reports the
// TRANSITION, in both directions: a latch that never resets is the easy wrong fix
// and passes a test that only fails once.
func TestRecordingFailureIsReportedOncePerTransition(t *testing.T) {
	handler := &countingHandler{}
	restore := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(restore) })

	rec := &toggleRecorder{}
	handlerUnderTest := Middleware(rec, "server", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serve := func() {
		handlerUnderTest.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	}

	rec.failing = true
	for range 5 {
		serve()
	}
	if handler.warns != 1 {
		t.Errorf("five failing requests logged %d warnings, want 1; a per-request warning is the volume at which it stops being read", handler.warns)
	}

	rec.failing = false
	for range 3 {
		serve()
	}
	if handler.infos != 1 {
		t.Errorf("recovery logged %d times, want 1", handler.infos)
	}

	// It must be able to degrade AGAIN. A latch that never resets would report
	// the first fault of the process and stay quiet through every later one.
	rec.failing = true
	for range 3 {
		serve()
	}
	if handler.warns != 2 {
		t.Errorf("a second outage logged %d warnings in total, want 2; the report did not re-arm after recovery", handler.warns)
	}
}
