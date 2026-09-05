package httpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/safelog"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const launchFailure = `failed to start: fork/exec /Users/op/.pinchtab/bin/pinchtab: exec format error`

// captureServerLog installs the process logger the server runs with — the safelog
// handler, which is what decides whether a path survives into a log line — and returns
// what it wrote. Testing against a bare slog handler would prove nothing about the
// sanitizing the real one applies.
func captureServerLog(t *testing.T, level slog.Level, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	previousLevel := safelog.CurrentLevel()
	slog.SetDefault(slog.New(safelog.NewDefaultHandler(&buf)))
	safelog.SetLevel(level)
	t.Cleanup(func() {
		slog.SetDefault(previous)
		safelog.SetLevel(previousLevel)
	})
	fn()
	return buf.String()
}

// The whole point of the card: the operator cannot act on "[path]", so the frame that
// still holds the real path has to write it down before the boundary rewrites it.
func TestServerFailureLogsTheUnredactedCauseWhileTheResponseStaysSanitized(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set(RequestIDHeader, "6c8c6f0aaf8df5a3")

	logged := captureServerLog(t, slog.LevelError, func() {
		ErrorCode(rec, http.StatusInternalServerError, "error", launchFailure, false, nil)
	})

	if !strings.Contains(logged, "/Users/op/.pinchtab/bin/pinchtab") {
		t.Errorf("the server log does not name the executable that failed, so the fault is still undiagnosable:\n%s", logged)
	}
	if !strings.Contains(logged, "exec format error") {
		t.Errorf("the server log dropped the underlying cause:\n%s", logged)
	}
	if strings.Contains(logged, "[path]") {
		t.Errorf("the server-side copy was redacted too, which leaves no unredacted copy anywhere:\n%s", logged)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wire, _ := body["error"].(string)
	if strings.Contains(wire, "/Users/op") {
		t.Errorf("the HTTP response leaked the absolute path: %q", wire)
	}
	if !strings.Contains(wire, "[path]") {
		t.Errorf("the HTTP response is no longer path-sanitized: %q", wire)
	}
}

// AC-2: the access log records a requestId and nothing else on disk was keyed by it.
// The cause line has to carry the same id or the two files still do not join.
func TestFailureCauseCarriesTheRequestIDTheActivityEventRecords(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set(RequestIDHeader, "6c8c6f0aaf8df5a3")

	logged := captureServerLog(t, slog.LevelError, func() {
		Problem(rec, http.StatusServiceUnavailable, "instance_not_ready", "instance not ready after 10s", true, nil)
	})

	if !strings.Contains(logged, "requestId=6c8c6f0aaf8df5a3") {
		t.Errorf("the cause is not keyed by the requestId, so a 5xx in the access log joins to nothing:\n%s", logged)
	}
	if !strings.Contains(logged, "status=503") {
		t.Errorf("the cause line does not carry the status it was returned with:\n%s", logged)
	}
}

// Every producer that writes a non-2xx has to log its cause; a new one that skips the
// helper reintroduces exactly the hole this card is about.
func TestEveryErrorProducerLogsItsCause(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(w http.ResponseWriter)
	}{
		{"ErrorCode", func(w http.ResponseWriter) {
			ErrorCode(w, http.StatusInternalServerError, "error", launchFailure, false, nil)
		}},
		{"Error", func(w http.ResponseWriter) {
			Error(w, http.StatusInternalServerError, errStub{})
		}},
		{"Problem", func(w http.ResponseWriter) {
			Problem(w, http.StatusInternalServerError, "error", launchFailure, false, nil)
		}},
		{"JSONError", func(w http.ResponseWriter) {
			JSONError(w, http.StatusInternalServerError, "error", launchFailure, map[string]any{"status": "error"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			logged := captureServerLog(t, slog.LevelError, func() { tc.write(rec) })
			if !strings.Contains(logged, "/Users/op/.pinchtab/bin/pinchtab") {
				t.Errorf("%s returned a 500 without logging its unredacted cause:\n%s", tc.name, logged)
			}
		})
	}
}

type errStub struct{}

func (errStub) Error() string { return launchFailure }

// A 4xx is the caller's input, not a server fault: it stays out of the error stream so
// the signal an operator greps for is bounded by server failures rather than by client
// behaviour — and it is still recoverable by raising the level.
func TestClientErrorsAreLoggedAtDebugNotError(t *testing.T) {
	rec := httptest.NewRecorder()

	atError := captureServerLog(t, slog.LevelError, func() {
		ErrorCode(rec, http.StatusBadRequest, "bad_request", "missing required field 'kind'", false, nil)
	})
	if strings.Contains(atError, "missing required field") {
		t.Errorf("a 400 was logged at error level, which floods the stream an operator greps for real faults:\n%s", atError)
	}

	rec = httptest.NewRecorder()
	atDebug := captureServerLog(t, slog.LevelDebug, func() {
		ErrorCode(rec, http.StatusBadRequest, "bad_request", "missing required field 'kind'", false, nil)
	})
	if !strings.Contains(atDebug, "missing required field") {
		t.Errorf("a 400 is unrecoverable even at debug level:\n%s", atDebug)
	}
}

const (
	minHttpxSourceFiles  = 4
	minModuleSourceFiles = 300
	minFailureProducers  = 3
	moduleRoot           = "../.."
	recorderChainFunc    = "RecordFailureReason"
	failurePreludeFunc   = "recordFailure"
)

var directFailureWrite = regexp.MustCompile(`http\.Error\(|WriteHeader\(http\.Status(BadRequest|Unauthorized|Forbidden|NotFound|MethodNotAllowed|Conflict|Gone|RequestEntityTooLarge|TooManyRequests|InternalServerError|NotImplemented|BadGateway|ServiceUnavailable|GatewayTimeout)\)|WriteHeader\([45][0-9][0-9]\)`)

func TestEveryFailureRecorderAlsoLogsItsCause(t *testing.T) {
	pkg := srccensus.Load(t, ".", minHttpxSourceFiles)

	logsItsCause := map[string]bool{}
	for _, site := range pkg.Calls(t, "logFailureCause") {
		logsItsCause[site.Func] = true
	}
	for _, site := range pkg.Calls(t, failurePreludeFunc) {
		logsItsCause[site.Func] = true
	}

	var offenders []string
	producers := 0
	for _, site := range append(pkg.Calls(t, recorderChainFunc), pkg.Calls(t, failurePreludeFunc)...) {
		if site.Func == recorderChainFunc || site.Func == failurePreludeFunc {
			continue
		}
		producers++
		if !logsItsCause[site.Func] {
			offenders = append(offenders, site.String())
		}
	}
	if producers < minFailureProducers {
		t.Fatalf("found %d failure producers outside the recorder chain, want at least %d; the census matched almost nothing and would pass vacuously", producers, minFailureProducers)
	}
	if len(offenders) > 0 {
		t.Errorf("these producers sanitize a failure and record it, but never log the unredacted cause, so the fault they describe is recoverable from nowhere; call logFailureCause before the sanitize, in the frame that still holds the raw message:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func TestNoSiteOutsideHttpxWritesAFailureResponseDirectly(t *testing.T) {
	var offenders []string
	for _, file := range srccensus.Tree(t, moduleRoot, minModuleSourceFiles) {
		if strings.HasPrefix(file.Name, "internal/httpx/") {
			continue
		}
		for i, line := range strings.Split(file.Text, "\n") {
			if directFailureWrite.MatchString(line) {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", file.Name, i+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("these sites write a failure response without going through httpx, so no cause is logged for them and no requestId joins them to the access log; route them through httpx.Error, httpx.ErrorCode or httpx.Problem:\n%s",
			strings.Join(offenders, "\n"))
	}
}
