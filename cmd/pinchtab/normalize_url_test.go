package main

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// No protocol - should add https://
		{"example.com", "https://example.com"},
		{"example.com/path", "https://example.com/path"},
		{"example.com:8080", "https://example.com:8080"},
		{"subdomain.example.com", "https://subdomain.example.com"},

		// Already has protocol - should not modify
		{"https://example.com", "https://example.com"},
		{"http://example.com", "http://example.com"},
		{"https://example.com/path", "https://example.com/path"},
		{"http://localhost:8080", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
