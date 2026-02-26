package handlers

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/textproto"
	"strings"
)

// ProxyWebSocket does a raw TCP tunnel for WebSocket connections.
// Uses proper HTTP request construction for standards compliance.
func ProxyWebSocket(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Parse target URL
	wsTarget := strings.TrimPrefix(targetURL, "http://")
	wsTarget = strings.TrimPrefix(wsTarget, "https://")

	host := wsTarget
	path := "/"
	if idx := strings.Index(wsTarget, "/"); idx >= 0 {
		host = wsTarget[:idx]
		path = wsTarget[idx:]
	}

	// Connect to backend
	backend, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		slog.Error("ws proxy: backend dial failed", "target", host, "err", err)
		return
	}
	defer func() { _ = backend.Close() }()

	// Hijack client connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		slog.Error("ws proxy: hijack failed", "err", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Write properly formatted HTTP request to backend
	writer := bufio.NewWriter(backend)

	// Request line
	_, _ = fmt.Fprintf(writer, "%s %s HTTP/1.1\r\n", r.Method, path)

	// Host header (required for HTTP/1.1)
	_, _ = fmt.Fprintf(writer, "Host: %s\r\n", host)

	// Copy other headers (using canonical header format)
	for name, values := range r.Header {
		// textproto.CanonicalMIMEHeaderKey ensures proper header formatting
		canonicalName := textproto.CanonicalMIMEHeaderKey(name)
		for _, value := range values {
			_, _ = fmt.Fprintf(writer, "%s: %s\r\n", canonicalName, value)
		}
	}

	// End of headers
	_, _ = fmt.Fprintf(writer, "\r\n")

	// Flush the buffered request
	if err := writer.Flush(); err != nil {
		slog.Error("ws proxy: failed to write request", "err", err)
		return
	}

	// Bidirectional copy between client and backend
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(client, backend)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(backend, client)
		done <- struct{}{}
	}()

	// Wait for one direction to complete
	<-done
}
