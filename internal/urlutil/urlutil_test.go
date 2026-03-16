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
		{"", true}, // empty is the only invalid case
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
	valid := []string{
		"https://example.com",
		"example.com",
		"file:///path/to/file.html",
		"javascript:alert(1)",
		"chrome://settings",
	}
	for _, u := range valid {
		if !IsValid(u) {
			t.Errorf("expected %q to be valid", u)
		}
	}
	if IsValid("") {
		t.Error("expected empty string to be invalid")
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
	// All explicit schemes should be allowed — user knows what they're doing
	validURLs := []string{
		"http://example.com",
		"https://example.com",
		"file:///path/to/file.html",
		"chrome://settings",
		"chrome://extensions",
		"chrome-extension://abc123/popup.html",
		"about:blank",
		"data:text/html,<h1>hi</h1>",
		"javascript:alert(1)",
		"javascript:void(0)",
		"vbscript:msgbox(1)",
		"ftp://files.example.com",
		"view-source:https://example.com",
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
}
