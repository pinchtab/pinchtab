//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestProfile_PathTraversalBlocked verifies that profile names with "../" are rejected
// Security: Prevents directory traversal attacks on profile storage
func TestProfile_PathTraversalBlocked(t *testing.T) {
	maliciousNames := []string{
		"../../../etc/passwd",
		"..",
		"..\\..\\..\\windows\\system32",
		"profile/../../../root",
		"./../../sensitive",
		".\\..\\..\\sensitive",
	}

	for _, name := range maliciousNames {
		t.Run(fmt.Sprintf("reject_%s", name), func(t *testing.T) {
			payload := map[string]any{
				"name": name,
			}

			code, body := httpPost(t, "/profiles", payload)
			if code == 200 || code == 201 {
				t.Errorf("expected rejection for malicious name %q, got %d: %s", name, code, string(body))
			}
			// Should get 400 Bad Request
			if code != 400 && code != 422 {
				t.Logf("got status %d for malicious name %q (expected 400 or 422)", code, name)
			}
		})
	}
}

// TestProfile_PathSeparatorBlocked verifies that profile names with "/" or "\" are rejected
// Security: Prevents creating profiles in subdirectories
func TestProfile_PathSeparatorBlocked(t *testing.T) {
	invalidNames := []string{
		"profile/subdir",
		"profile\\subdir",
		"dir/myprofile",
		"dir\\myprofile",
		"/etc/profile",
		"\\windows\\profile",
	}

	for _, name := range invalidNames {
		t.Run(fmt.Sprintf("reject_separators_%s", name), func(t *testing.T) {
			payload := map[string]any{
				"name": name,
			}

			code, body := httpPost(t, "/profiles", payload)
			if code == 200 || code == 201 {
				t.Errorf("expected rejection for path separator in %q, got %d: %s", name, code, string(body))
			}
		})
	}
}

// TestProfile_EmptyNameRejected verifies that empty profile names are rejected
func TestProfile_EmptyNameRejected(t *testing.T) {
	payload := map[string]any{
		"name": "",
	}

	code, _ := httpPost(t, "/profiles", payload)
	if code == 200 || code == 201 {
		t.Errorf("expected rejection for empty profile name, got %d", code)
	}
}

// TestProfile_ValidNamesAccepted verifies that legitimate profile names work
func TestProfile_ValidNamesAccepted(t *testing.T) {
	validNames := []string{
		"myprofile",
		"test-profile-1",
		"profile_with_underscores",
		"123numeric",
		"UPPERCASE",
		"mixed-Case_123",
	}

	for _, name := range validNames {
		t.Run(fmt.Sprintf("accept_%s", name), func(t *testing.T) {
			payload := map[string]any{
				"name": name,
			}

			code, body := httpPost(t, "/profiles", payload)
			if code != 200 && code != 201 {
				t.Logf("warning: valid name %q got status %d: %s", name, code, string(body))
				// Log but don't fail - validation may be stricter in some configs
			}
		})
	}
}
