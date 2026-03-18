package authn

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestTokenFromRequest_HeaderWins(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	req.Header.Set("Cookie", CookieName+"="+url.QueryEscape("cookie-token"))

	if got := TokenFromRequest(req); got != "header-token" {
		t.Fatalf("TokenFromRequest() = %q, want %q", got, "header-token")
	}
	if creds := CredentialsFromRequest(req); creds.Method != MethodHeader {
		t.Fatalf("CredentialsFromRequest().Method = %q, want %q", creds.Method, MethodHeader)
	}
}

func TestTokenFromRequest_CookieFallback(t *testing.T) {
	const want = "cookie token/+"
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", CookieName+"="+url.QueryEscape(want))

	if got := TokenFromRequest(req); got != want {
		t.Fatalf("TokenFromRequest() = %q, want %q", got, want)
	}
	if creds := CredentialsFromRequest(req); creds.Method != MethodCookie {
		t.Fatalf("CredentialsFromRequest().Method = %q, want %q", creds.Method, MethodCookie)
	}
}

func TestTokenFromRequest_NoToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	if got := TokenFromRequest(req); got != "" {
		t.Fatalf("TokenFromRequest() = %q, want empty", got)
	}
}

func TestClientIP_IgnoresForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:43123"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-Ip", "203.0.113.8")

	if got := ClientIP(req); got != "198.51.100.10" {
		t.Fatalf("ClientIP() = %q, want %q", got, "198.51.100.10")
	}
}

func TestClientIP_FallsBackToRawRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10"

	if got := ClientIP(req); got != "198.51.100.10" {
		t.Fatalf("ClientIP() = %q, want %q", got, "198.51.100.10")
	}
}
