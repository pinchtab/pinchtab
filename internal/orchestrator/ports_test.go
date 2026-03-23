package orchestrator

import "testing"

func TestParsePortNumber(t *testing.T) {
	port, err := parsePortNumber("1234")
	if err != nil {
		t.Fatalf("parsePortNumber returned error: %v", err)
	}
	if port != 1234 {
		t.Fatalf("port = %d, want 1234", port)
	}
}

func TestParsePortNumber_RejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "abc", "0", "65536", "001234", "0x1234"} {
		if _, err := parsePortNumber(raw); err == nil {
			t.Fatalf("parsePortNumber(%q) should reject invalid port", raw)
		}
	}
}
