package handlers

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/textproto"
	"net/url"

	internalurls "github.com/pinchtab/pinchtab/internal/urls"
)

// ProxyWebSocket tunnels WebSocket connections with proper HTTP headers
func ProxyWebSocket(w http.ResponseWriter, r *http.Request, targetURL string) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "invalid backend target", http.StatusBadGateway)
		slog.Error("ws proxy: invalid target", "target", internalurls.RedactForLog(targetURL), "err", err)
		return
	}

	host := parsed.Host
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}

	var backend net.Conn
	switch parsed.Scheme {
	case "https", "wss":
		backend, err = tls.Dial("tcp", host, &tls.Config{ServerName: parsed.Hostname()})
	default:
		backend, err = net.Dial("tcp", host)
	}
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

	writer := bufio.NewWriter(backend)

	_, _ = fmt.Fprintf(writer, "%s %s HTTP/1.1\r\n", r.Method, path)
	_, _ = fmt.Fprintf(writer, "Host: %s\r\n", host)

	for name, values := range r.Header {
		canonicalName := textproto.CanonicalMIMEHeaderKey(name)
		for _, value := range values {
			_, _ = fmt.Fprintf(writer, "%s: %s\r\n", canonicalName, value)
		}
	}

	_, _ = fmt.Fprintf(writer, "\r\n")

	if err := writer.Flush(); err != nil {
		slog.Error("ws proxy: failed to write request", "err", err)
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(client, backend)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(backend, client)
		done <- struct{}{}
	}()

	<-done
}
