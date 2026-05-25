package bridge

import (
	"testing"
)

func TestGlobMatch_EdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		url     string
		want    bool
	}{
		{"empty URL returns false", "api.example.com", "", false},
		{"unicode in URL", "café.example.com", "https://café.example.com/", true},
		{"unicode hostname does not match ASCII-only wildcard", "*.example.com", "https://café.example.com/", false},
		{"query string preserved", "api.example.com", "https://api.example.com?foo=bar", true},
		{"fragment preserved", "api.example.com", "https://api.example.com#/path", true},
		{"port in URL", "api.example.com", "https://api.example.com:8080/users", true},
		{"IP address literal", "127.0.0.1", "https://127.0.0.1/api", true},
		{"https vs http", "http://api.example.com", "https://api.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := globMatch(tc.pattern, tc.url)
			if got != tc.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.url, got, tc.want)
			}
		})
	}
}
