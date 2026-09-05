package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestSameOriginRequest_UsesForwardedProtoAndHost(t *testing.T) {
	req := httptest.NewRequest("GET", "http://pinchtab/health", nil)
	req.Host = "pinchtab:9867"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "browser.example.com")

	if !sameOriginRequest("https://browser.example.com/dashboard", req, true) {
		t.Fatal("expected same-origin when trustProxy=true and forwarded proto/host match")
	}
}

func TestSameOriginRequest_UsesForwardedHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "http://pinchtab/health", nil)
	req.Host = "pinchtab:9867"
	req.Header.Set("Forwarded", `for=127.0.0.1;proto=https;host=browser.example.com`)

	if !sameOriginRequest("https://browser.example.com/dashboard", req, true) {
		t.Fatal("expected same-origin when trustProxy=true and RFC 7239 Forwarded header matches")
	}
}

func TestSameOriginRequest_IgnoresForwardedWhenTrustDisabled(t *testing.T) {
	req := httptest.NewRequest("GET", "http://pinchtab/health", nil)
	req.Host = "pinchtab:9867"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "browser.example.com")

	if sameOriginRequest("https://browser.example.com/dashboard", req, false) {
		t.Fatal("expected NOT same-origin when trustProxy=false even with forwarded headers")
	}
}

func TestSameOriginRequest_DefaultIgnoresForwarded(t *testing.T) {
	req := httptest.NewRequest("GET", "http://pinchtab/health", nil)
	req.Host = "pinchtab:9867"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "browser.example.com")

	if sameOriginRequest("https://browser.example.com/dashboard", req, false) {
		t.Fatal("expected NOT same-origin when the proxy is not trusted")
	}
}
