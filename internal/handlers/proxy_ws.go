package handlers

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// ProxyWebSocket does a raw TCP tunnel for WebSocket connections.
// This is the simplest approach — no frame parsing, just bidirectional copy.
func ProxyWebSocket(w http.ResponseWriter, r *http.Request, targetURL string) {

	wsTarget := strings.Replace(targetURL, "http://", "", 1)
	wsTarget = strings.Replace(wsTarget, "https://", "", 1)

	host := wsTarget
	path := "/"
	if idx := strings.Index(wsTarget, "/"); idx >= 0 {
		host = wsTarget[:idx]
		path = wsTarget[idx:]
	}

	backend, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		slog.Error("ws proxy: backend dial failed", "target", host, "err", err)
		return
	}
	defer func() { _ = backend.Close() }()

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

	reqLine := r.Method + " " + path + " HTTP/1.1\r\n"
	_, _ = backend.Write([]byte(reqLine))
	_, _ = backend.Write([]byte("Host: " + host + "\r\n"))
	for k, vv := range r.Header {
		for _, v := range vv {
			_, _ = backend.Write([]byte(k + ": " + v + "\r\n"))
		}
	}
	_, _ = backend.Write([]byte("\r\n"))

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	go func() { _, _ = io.Copy(backend, client); done <- struct{}{} }()
	<-done
}
