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

	"github.com/pinchtab/pinchtab/internal/httpx"
	internalurls "github.com/pinchtab/pinchtab/internal/urls"
)

const proxyWSBackendAuthorizationHeader = "X-Pinchtab-Proxy-Authorization"

// ProxyWebSocket tunnels WebSocket connections with proper HTTP headers
func ProxyWebSocket(w http.ResponseWriter, r *http.Request, targetURL string) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		httpx.Problem(w, http.StatusBadGateway, "invalid_backend_target", "invalid backend target", false, nil)
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
		httpx.Problem(w, http.StatusBadGateway, "backend_unavailable", "backend unavailable", true, nil)
		slog.Error("ws proxy: backend dial failed", "target", host, "err", err)
		return
	}
	defer func() { _ = backend.Close() }()

	hj, ok := w.(http.Hijacker)
	if !ok {
		httpx.Problem(w, http.StatusInternalServerError, "hijack_unsupported", "server does not support hijacking", false, nil)
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

	for name, values := range filterProxyWSHeaders(r.Header) {
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

func filterProxyWSHeaders(headers http.Header) http.Header {
	filtered := make(http.Header)
	for name, values := range headers {
		if textproto.CanonicalMIMEHeaderKey(name) == proxyWSBackendAuthorizationHeader {
			copied := append([]string(nil), values...)
			filtered["Authorization"] = copied
			continue
		}
		if !allowProxyWSHeader(name) {
			continue
		}
		copied := append([]string(nil), values...)
		filtered[textproto.CanonicalMIMEHeaderKey(name)] = copied
	}
	return filtered
}

func allowProxyWSHeader(name string) bool {
	canonical := textproto.CanonicalMIMEHeaderKey(name)
	if canonical == "Connection" || canonical == "Upgrade" || canonical == "Origin" || canonical == "User-Agent" {
		return true
	}
	return canonical == "Sec-Websocket-Key" || canonical == "Sec-Websocket-Version" || canonical == "Sec-Websocket-Protocol" || canonical == "Sec-Websocket-Extensions"
}

func SetProxyWSBackendAuthorization(headers http.Header, value string) {
	if headers == nil {
		return
	}
	value = textproto.TrimString(value)
	if value == "" {
		headers.Del(proxyWSBackendAuthorizationHeader)
		return
	}
	headers.Set(proxyWSBackendAuthorizationHeader, value)
}
