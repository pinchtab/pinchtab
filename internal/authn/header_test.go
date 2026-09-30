package authn

import (
	"net/http"
	"testing"
)

func TestAuthorizationHeaderValue(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "empty", token: "", want: ""},
		{name: "bearer token", token: "server-token", want: "Bearer server-token"},
		{name: "session token", token: "ses_token123", want: "Session ses_token123"},
		{name: "trimmed session token", token: "  ses_token123  ", want: "Session ses_token123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AuthorizationHeaderValue(tt.token); got != tt.want {
				t.Fatalf("AuthorizationHeaderValue(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestSetAuthorizationLeavesHeaderUnsetForEmptyToken(t *testing.T) {
	h := http.Header{}
	SetAuthorization(h, "  ")
	if _, ok := h["Authorization"]; ok {
		t.Fatalf("Authorization set for empty token: %q", h.Get("Authorization"))
	}
}
