package orchestrator

import (
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"valid port", "8080", false},
		{"valid high port", "65000", false},
		{"valid pinchtab port", "9867", false},
		{"port 80", "80", false},
		{"port 443", "443", false},
		{"empty port", "", true},
		{"non-numeric", "abc", true},
		{"negative", "-1", true},
		{"zero", "0", true},
		{"too high", "70000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}
