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

	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const (
	DefaultMaxJSONBodyBytes = 1 << 20
	maxErrorMessageBytes    = 1024
)

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
	payload := map[string]any{
		"error": SanitizeErrorMessage(message),
		"code":  code,
	}
	if retryable {
		payload["retryable"] = true
	}
	if len(details) > 0 {
		payload["details"] = details
	}
	JSON(w, status, payload)
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

type StatusWriter struct {
	http.ResponseWriter
	Code int
}

func (w *StatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *StatusWriter) WriteHeader(code int) {
	w.Code = code
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
