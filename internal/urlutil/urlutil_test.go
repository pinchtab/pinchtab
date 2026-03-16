package urlutil

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// No protocol - should add https://
		{"example.com", "https://example.com"},
		{"example.com/path", "https://example.com/path"},
		{"example.com:8080", "https://example.com:8080"},
		{"sub.example.com/path?q=1", "https://sub.example.com/path?q=1"},

		// Already has protocol - should not modify
		{"https://example.com", "https://example.com"},
		{"http://example.com", "http://example.com"},
		{"http://localhost:8080", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input)
			if result != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// Valid URLs
		{"https://example.com", false},
		{"http://example.com", false},
		{"https://example.com/path", false},
		{"http://localhost:8080", false},
		{"example.com", false},          // normalized to https://
		{"example.com/path?q=1", false}, // normalized to https://

		// Invalid URLs
		{"ftp://example.com", true},   // unsupported scheme
		{"javascript:alert(1)", true}, // blocked (XSS)
		{"", true},                    // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := Sanitize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Sanitize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid("https://example.com") {
		t.Error("expected https://example.com to be valid")
	}
	if !IsValid("example.com") {
		t.Error("expected example.com to be valid (normalizes to https)")
	}
	if !IsValid("file:///path/to/file.html") {
		t.Error("expected file:// URL to be valid")
	}
	if IsValid("javascript:alert(1)") {
		t.Error("expected javascript: URL to be invalid")
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com", "example.com"},
		{"https://Example.COM/path", "example.com"},
		{"http://sub.example.com:8080/path", "sub.example.com"},
		{"example.com/path", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ExtractHost(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitize_BrowserURLs(t *testing.T) {
	// These browser-specific URLs should be allowed
	validURLs := []string{
		"file:///path/to/file.html",
		"chrome://settings",
		"chrome://extensions",
		"chrome-extension://abc123/popup.html",
		"about:blank",
		"data:text/html,<h1>hi</h1>",
	}
	for _, u := range validURLs {
		result, err := Sanitize(u)
		if err != nil {
			t.Errorf("expected %q to be valid, got error: %v", u, err)
		}
		if result != u {
			t.Errorf("expected %q unchanged, got %q", u, result)
		}
	}

	// Only javascript: is blocked (XSS risk)
	blockedURLs := []string{
		"javascript:alert(1)",
	}
	for _, u := range blockedURLs {
		_, err := Sanitize(u)
		if err == nil {
			t.Errorf("expected error for blocked URL %q, got nil", u)
		}
	}
}
