package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

func JSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("json encode", "err", err)
	}
}

func Error(w http.ResponseWriter, code int, err error) {
	ErrorCode(w, code, "error", err.Error(), false, nil)
}

func ErrorCode(w http.ResponseWriter, status int, code, message string, retryable bool, details map[string]any) {
	payload := map[string]any{
		"error": message,
		"code":  code,
	}
	if retryable {
		payload["retryable"] = true
	}
	if details != nil && len(details) > 0 {
		payload["details"] = details
	}
	JSON(w, status, payload)
}

// CancelOnClientDone cancels the given cancel func when the HTTP client disconnects.
func CancelOnClientDone(reqCtx context.Context, cancel context.CancelFunc) {
	<-reqCtx.Done()
	cancel()
}

// StatusWriter wraps ResponseWriter to capture the status code.
// It preserves Hijacker and Flusher interfaces for WebSocket/SSE support.
type StatusWriter struct {
	http.ResponseWriter
	Code int
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
