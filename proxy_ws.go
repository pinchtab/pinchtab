package main

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// proxyWebSocket does a raw TCP tunnel for WebSocket connections.
// This is the simplest approach — no frame parsing, just bidirectional copy.
func proxyWebSocket(w http.ResponseWriter, r *http.Request, targetURL string) {
	// Convert http(s) to ws
	wsTarget := strings.Replace(targetURL, "http://", "", 1)
	wsTarget = strings.Replace(wsTarget, "https://", "", 1)

	// Extract host:port and path
	host := wsTarget
	path := "/"
	if idx := strings.Index(wsTarget, "/"); idx >= 0 {
		host = wsTarget[:idx]
		path = wsTarget[idx:]
	}

	// Connect to target
	backend, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		slog.Error("ws proxy: backend dial failed", "target", host, "err", err)
		return
	}
	defer backend.Close()

	// Hijack the client connection
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
	defer client.Close()

	// Forward the original HTTP upgrade request to backend
	reqLine := r.Method + " " + path + " HTTP/1.1\r\n"
	backend.Write([]byte(reqLine))
	backend.Write([]byte("Host: " + host + "\r\n"))
	for k, vv := range r.Header {
		for _, v := range vv {
			backend.Write([]byte(k + ": " + v + "\r\n"))
		}
	}
	backend.Write([]byte("\r\n"))

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(client, backend); done <- struct{}{} }()
	go func() { io.Copy(backend, client); done <- struct{}{} }()
	<-done
}
