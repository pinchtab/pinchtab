package handlers

import (
	"net/http"
	"testing"
)

func TestFilterProxyWSHeaders_StripsSensitiveHeaders(t *testing.T) {
	headers := http.Header{
		"Connection":               {"Upgrade"},
		"Upgrade":                  {"websocket"},
		"Sec-WebSocket-Key":        {"abc123"},
		"Sec-WebSocket-Version":    {"13"},
		"Sec-WebSocket-Protocol":   {"chat"},
		"Sec-WebSocket-Extensions": {"permessage-deflate"},
		"Authorization":            {"Bearer secret-token"},
		"Cookie":                   {"pinchtab_auth_token=session-secret"},
		"X-Forwarded-For":          {"203.0.113.1"},
		"X-Request-Id":             {"request-123"},
	}

	filtered := filterProxyWSHeaders(headers)

	for _, forbidden := range []string{"Authorization", "Cookie", "X-Forwarded-For", "X-Request-Id"} {
		if got := filtered.Get(forbidden); got != "" {
			t.Fatalf("%s should have been stripped, got %q", forbidden, got)
		}
	}
	for _, allowed := range []string{"Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"} {
		if got := filtered.Get(allowed); got == "" {
			t.Fatalf("%s should have been forwarded", allowed)
		}
	}
}

func TestFilterProxyWSHeaders_ForwardsExplicitBackendAuthorizationOnly(t *testing.T) {
	headers := http.Header{
		"Authorization":                   {"Bearer user-token"},
		proxyWSBackendAuthorizationHeader: {"Bearer backend-token"},
	}

	filtered := filterProxyWSHeaders(headers)

	if got := filtered.Get("Authorization"); got != "Bearer backend-token" {
		t.Fatalf("Authorization = %q, want backend token", got)
	}
	if got := filtered.Get(proxyWSBackendAuthorizationHeader); got != "" {
		t.Fatalf("%s should not be forwarded directly, got %q", proxyWSBackendAuthorizationHeader, got)
	}
}

func TestAllowProxyWSHeader_OriginAndUserAgentAllowed(t *testing.T) {
	for _, name := range []string{"Origin", "User-Agent"} {
		if !allowProxyWSHeader(name) {
			t.Fatalf("%s should be allowed", name)
		}
	}
}

func TestSetProxyWSBackendAuthorization(t *testing.T) {
	headers := http.Header{}

	SetProxyWSBackendAuthorization(headers, "Bearer backend-token")
	if got := headers.Get(proxyWSBackendAuthorizationHeader); got != "Bearer backend-token" {
		t.Fatalf("proxy auth header = %q, want backend token", got)
	}

	SetProxyWSBackendAuthorization(headers, "")
	if got := headers.Get(proxyWSBackendAuthorizationHeader); got != "" {
		t.Fatalf("proxy auth header should be cleared, got %q", got)
	}
}
