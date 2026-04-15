package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestCookieAuthAllowed_ActionPost(t *testing.T) {
	req := httptest.NewRequest("POST", "/action", nil)
	if !cookieAuthAllowed(req) {
		t.Fatal("expected cookie auth to allow POST /action for dashboard same-origin sessions")
	}
}

func TestCookieAuthAllowed_InstanceStartPost(t *testing.T) {
	req := httptest.NewRequest("POST", "/instances/start", nil)
	if !cookieAuthAllowed(req) {
		t.Fatal("expected cookie auth to allow POST /instances/start for dashboard same-origin sessions")
	}
}
