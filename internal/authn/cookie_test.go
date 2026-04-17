package authn

import (
	"net/http/httptest"
	"testing"
)

func TestSessionCookieSecure_AutoDetect(t *testing.T) {
	t.Run("https is secure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.com/dashboard", nil)
		if !sessionCookieSecure(req, false, nil) {
			t.Fatal("expected https request to set Secure cookie")
		}
	})

	t.Run("localhost over http is not secure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://localhost:9867/dashboard", nil)
		if sessionCookieSecure(req, false, nil) {
			t.Fatal("expected localhost http request to clear Secure cookie")
		}
	})

	t.Run("lan host over http is not secure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://192.168.1.50:9867/dashboard", nil)
		if sessionCookieSecure(req, false, nil) {
			t.Fatal("expected lan http request to omit Secure so browser sessions work over plain HTTP")
		}
	})
}

func TestSessionCookieSecure_ConfigOverride(t *testing.T) {
	t.Run("explicit false disables secure flag", func(t *testing.T) {
		force := false
		req := httptest.NewRequest("GET", "https://example.com/dashboard", nil)
		if sessionCookieSecure(req, false, &force) {
			t.Fatal("expected explicit false to disable Secure cookie")
		}
	})

	t.Run("explicit true enables secure flag", func(t *testing.T) {
		force := true
		req := httptest.NewRequest("GET", "http://localhost:9867/dashboard", nil)
		if !sessionCookieSecure(req, false, &force) {
			t.Fatal("expected explicit true to enable Secure cookie")
		}
	})
}
