package httpx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const (
	DefaultMaxJSONBodyBytes = 1 << 20
	maxErrorMessageBytes    = 1024

	MaxNavigationTimeout      = 120 * time.Second
	NavigationTransportGrace  = 15 * time.Second
	MaxNavigationHTTPDuration = MaxNavigationTimeout + NavigationTransportGrace
)

type ProblemDetails struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Detail    string         `json:"detail,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	Code      string         `json:"code,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// JSONError writes a hand-shaped error body — one the standard envelope cannot express,
// such as a health payload a client parses by key — and records the reason with it. Every
// non-2xx JSON response goes through here or through ErrorCode, so no failure reaches the
// log, the metrics or the activity record with the response body as its only witness.
func JSONError(w http.ResponseWriter, status int, code, message string, payload any) {
	logFailureCause(w, status, code, message)
	RecordFailureReason(w, code, SanitizeErrorMessage(message))
	JSON(w, status, payload)
}

func JSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("json encode", "err", err)
	}
}

func Error(w http.ResponseWriter, code int, err error) {
	message := http.StatusText(code)
	if err != nil {
		message = err.Error()
	}
	if message == "" {
		message = "error"
	}
	ErrorCode(w, code, "error", message, false, nil)
}

func ErrorCode(w http.ResponseWriter, status int, code, message string, retryable bool, details map[string]any) {
	logFailureCause(w, status, code, message)
	sanitized := SanitizeErrorMessage(message)
	RecordFailureReason(w, code, sanitized)
	payload := map[string]any{
		"error": sanitized,
		"code":  code,
	}
	if retryable {
		payload["retryable"] = true
	}
	if len(details) > 0 {
		payload["details"] = sanitizeDetails(details)
	}
	JSON(w, status, payload)
}

// sanitizeDetails returns a copy of details with every string cleaned the same
// way as the message beside it. Details carry page-controlled data — dialog
// text, document titles, navigated URLs — and the CLI prints some of them
// straight to a terminal, so leaving them raw would let a visited page smuggle
// ANSI escapes past the sanitizing the message already gets. The input is
// copied rather than rewritten so callers may reuse their map.
func sanitizeDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]any, len(details))
	for key, value := range details {
		out[key] = sanitizeDetailValue(value, 0)
	}
	return out
}

const maxDetailDepth = 4

func sanitizeDetailValue(value any, depth int) any {
	if depth > maxDetailDepth {
		return nil
	}
	switch v := value.(type) {
	case string:
		// Not SanitizeErrorMessage: an intentionally empty detail must stay
		// empty rather than becoming the message-level "error" placeholder.
		return sanitize.CleanError(v, maxErrorMessageBytes)
	case []string:
		out := make([]string, len(v))
		for i, s := range v {
			out[i] = sanitize.CleanError(s, maxErrorMessageBytes)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = sanitizeDetailValue(e, depth+1)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = sanitizeDetailValue(e, depth+1)
		}
		return out
	default:
		return value
	}
}

func Problem(w http.ResponseWriter, status int, code, detail string, retryable bool, details map[string]any) {
	title := http.StatusText(status)
	if title == "" {
		title = "Error"
	}

	logFailureCause(w, status, code, detail)
	sanitized := SanitizeErrorMessage(detail)
	RecordFailureReason(w, code, sanitized)
	payload := ProblemDetails{
		Type:    "about:blank",
		Title:   title,
		Status:  status,
		Detail:  sanitized,
		Code:    code,
		Details: sanitizeDetails(details),
	}
	if retryable {
		payload.Retryable = true
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("problem encode", "err", err)
	}
}

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxJSONBodyBytes
	}
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes)).Decode(dst)
}

func StatusForJSONDecodeError(err error) int {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func SanitizeErrorMessage(message string) string {
	message = sanitize.CleanError(message, maxErrorMessageBytes)
	if message == "" {
		return "error"
	}
	return message
}

// CancelOnClientDone cancels the given cancel func when the HTTP client disconnects.
func CancelOnClientDone(reqCtx context.Context, cancel context.CancelFunc) {
	<-reqCtx.Done()
	cancel()
}

// ExtendWriteDeadline pushes the connection's write deadline d into the
// future so a long-running handler (multi-page audit/scrape) is not killed
// by the server's default WriteTimeout before it can write its response.
// Best-effort: an unsupported ResponseWriter keeps the server default.
func ExtendWriteDeadline(w http.ResponseWriter, d time.Duration) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
}

// ReasonRecorder is implemented by a ResponseWriter that keeps the reason a failure was
// written with, so an error producer can hand it back to the middlewares wrapped around
// it. It is what makes the reason travel from the one frame that has it to the sinks,
// instead of leaving the response body as its only copy.
type ReasonRecorder interface {
	RecordFailureReason(code, message string)
}

// FailureCodeHeader and FailureMessageHeader carry the reason across a proxy hop: the
// producing instance stamps them, the front door reads them back into
// RecordFailureReason, and the public boundary strips them (StatusWriter, below). The
// X-Pinchtab- prefix keeps them inside the internal-header trust rules on the request
// side too.
const (
	FailureCodeHeader    = "X-Pinchtab-Failure-Code"
	FailureMessageHeader = "X-Pinchtab-Failure-Message"
)

// RequestIDHeader is the id the request-id middleware stamps on every response and
// the activity recorder copies into its event, so it is the join key between an
// access-log line and the cause logged for it.
//
// It is the ONLY spelling of this header in the module, and the census beside it
// keeps it that way. The name was written out twice here and as a bare literal in
// four more packages, so renaming it — to lower-case it, or to move to a
// standard trace header — would have changed some readers and not others, and the
// halves fail quietly: a stamp nobody reads logs an empty id, and a reader whose
// writer moved on lets the outer chain's id through twice.
const RequestIDHeader = "X-Request-Id"

// logFailureCause writes the UNREDACTED reason a failure was returned. The message
// that crosses the HTTP boundary has its absolute paths rewritten to [path], which
// removes the one fact that identifies a fault like a malformed executable — so the
// producer's own copy is logged here, before sanitizing, keyed by the requestId the
// activity event already carries.
//
// 5xx logs at error: the server is at fault and an operator has to be able to find it
// without raising a level. 4xx logs at debug — it is the caller's input, and the
// volume follows client behaviour rather than anything the server controls.
func logFailureCause(w http.ResponseWriter, status int, code, message string) {
	if status < 400 || message == "" {
		return
	}
	level := slog.LevelDebug
	if status >= 500 {
		level = slog.LevelError
	}
	slog.Log(context.Background(), level, "request failed",
		"requestId", w.Header().Get(RequestIDHeader),
		"status", status,
		"code", code,
		"cause", sanitize.CleanForLog(message, maxErrorMessageBytes))
}

// RecordFailureReason gives w the code and the SANITIZED message that were just written.
// It stamps the hop headers so the reason survives a proxy boundary, and walks the
// recorder chain for the in-process sinks; nothing re-reads the response body to recover
// what the producer already held.
func RecordFailureReason(w http.ResponseWriter, code, message string) {
	if code == "" && message == "" {
		return
	}
	w.Header().Set(FailureCodeHeader, code)
	w.Header().Set(FailureMessageHeader, message)
	if recorder, ok := w.(ReasonRecorder); ok {
		recorder.RecordFailureReason(code, message)
	}
}

type StatusWriter struct {
	http.ResponseWriter
	Code int

	FailureCode    string
	FailureMessage string

	// StripFailureHeaders removes the hop headers at flush: set on the public
	// boundary, left false for a trusted internal hop so the front door can read
	// them. The header map is shared along the wrapper chain, so the innermost
	// writer's decision is the one that acts.
	StripFailureHeaders bool
}

// RecordFailureReason keeps the reason and passes it OUTWARD along the wrapper chain: the
// request path stacks several StatusWriters (activity outside, logging inside), and each
// one is a sink that needs the same fact. Whichever writer the handler holds, every
// StatusWriter wrapped around it ends up with the reason.
func (w *StatusWriter) RecordFailureReason(code, message string) {
	w.FailureCode, w.FailureMessage = code, message
	RecordFailureReason(w.ResponseWriter, code, message)
}

func (w *StatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *StatusWriter) WriteHeader(code int) {
	w.Code = code
	if w.StripFailureHeaders {
		w.Header().Del(FailureCodeHeader)
		w.Header().Del(FailureMessageHeader)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *StatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter is not a Hijacker")
}

func (w *StatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
