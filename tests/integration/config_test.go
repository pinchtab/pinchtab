//go:build integration

package integration

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// extractChromeVersion extracts the Chrome version from a user agent string.
// Returns the version string (e.g., "145.0.0.0") or empty string if not found.
func extractChromeVersion(ua string) string {
	// Match "Chrome/X.Y.Z.W" or "HeadlessChrome/X.Y.Z.W"
	re := regexp.MustCompile(`(?:Headless)?Chrome[/\s]+(\d+\.\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(ua)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// CF7: Chrome version default
// Navigate, eval navigator.userAgent, verify it contains a valid Chrome version.
func TestConfig_ChromeVersionDefault(t *testing.T) {
	navigate(t, "https://example.com")

	code, body := httpPost(t, "/evaluate", map[string]string{
		"expression": "navigator.userAgent",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}

	ua := jsonField(t, body, "result")
	// Remove quotes if present (JSON string)
	ua = strings.Trim(ua, `"`)

	// Extract Chrome version from UA
	version := extractChromeVersion(ua)
	if version == "" {
		t.Errorf("expected user agent to contain Chrome version, got: %s", ua)
	}

	// Verify it's a valid UA string (contains Chrome/version)
	if !strings.Contains(ua, "Chrome/") && !strings.Contains(ua, "HeadlessChrome/") {
		t.Errorf("expected user agent to contain 'Chrome/' or 'HeadlessChrome/', got: %s", ua)
	}

	t.Logf("Chrome version: %s", version)
}

// CF8: Chrome version in fingerprint
// Navigate, POST /fingerprint/rotate, verify UA still contains the same Chrome version.
func TestConfig_ChromeVersionInFingerprint(t *testing.T) {
	navigate(t, "https://example.com")

	// Get initial user agent
	code1, body1 := httpPost(t, "/evaluate", map[string]string{
		"expression": "navigator.userAgent",
	})
	if code1 != 200 {
		t.Fatalf("expected 200 for initial UA eval, got %d", code1)
	}
	initialUA := jsonField(t, body1, "result")
	initialUA = strings.Trim(initialUA, `"`)

	// Extract initial Chrome version
	initialVersion := extractChromeVersion(initialUA)
	if initialVersion == "" {
		t.Fatalf("expected initial UA to contain Chrome version, got: %s", initialUA)
	}

	// Rotate fingerprint with "mac" OS to ensure consistent test results
	// (don't use random because we want to verify the Chrome version is preserved)
	code2, body2 := httpPost(t, "/fingerprint/rotate", map[string]string{
		"os": "mac",
	})
	if code2 != 200 {
		t.Fatalf("expected 200 for fingerprint rotate, got %d (body: %s)", code2, body2)
	}

	// Get user agent after rotation
	code3, body3 := httpPost(t, "/evaluate", map[string]string{
		"expression": "navigator.userAgent",
	})
	if code3 != 200 {
		t.Fatalf("expected 200 for post-rotate UA eval, got %d", code3)
	}
	rotatedUA := jsonField(t, body3, "result")
	rotatedUA = strings.Trim(rotatedUA, `"`)

	// Extract rotated Chrome version
	rotatedVersion := extractChromeVersion(rotatedUA)
	if rotatedVersion == "" {
		t.Fatalf("expected rotated UA to contain Chrome version, got: %s", rotatedUA)
	}

	// Verify Chrome version is preserved after fingerprint rotation
	// (fingerprint rotation should preserve Chrome version from BRIDGE_CHROME_VERSION)
	if initialVersion != rotatedVersion {
		t.Errorf("expected Chrome version to be preserved after fingerprint rotation, but got %s initially and %s after rotation", initialVersion, rotatedVersion)
	}

	t.Logf("Initial version: %s, Rotated version: %s", initialVersion, rotatedVersion)
}

// CF6: Chrome version override
// Set BRIDGE_CHROME_VERSION via TEST_CHROME_VERSION environment variable.
// Usage: TEST_CHROME_VERSION=999.0.0.0 go test -tags integration -v -run TestConfig_ChromeVersionOverride
func TestConfig_ChromeVersionOverride(t *testing.T) {
	// Check if TEST_CHROME_VERSION was set (which would have been passed to Pinchtab via BRIDGE_CHROME_VERSION)
	testVersion := os.Getenv("TEST_CHROME_VERSION")
	if testVersion == "" {
		t.Skip("TEST_CHROME_VERSION not set; set it to run this test (e.g., TEST_CHROME_VERSION=999.0.0.0 go test -tags integration -v)")
	}

	navigate(t, "https://example.com")

	code, body := httpPost(t, "/evaluate", map[string]string{
		"expression": "navigator.userAgent",
	})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}

	ua := jsonField(t, body, "result")
	ua = strings.Trim(ua, `"`)

	// Extract Chrome version from UA
	version := extractChromeVersion(ua)
	if version == "" {
		t.Errorf("expected user agent to contain Chrome version, got: %s", ua)
	}

	// Verify the Chrome version matches what was set via TEST_CHROME_VERSION
	if version != testVersion {
		t.Errorf("expected Chrome version %q, but got %q in UA: %s", testVersion, version, ua)
	}

	t.Logf("Chrome version override working: %s", version)
}
