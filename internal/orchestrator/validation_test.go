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
		{"empty port", "", true},
		{"non-numeric", "abc", true},
		{"too low", "80", true},
		{"too high", "70000", true},
		{"mysql port", "3306", true},
		{"redis port", "6379", true},
		{"postgres port", "5432", true},
		{"elasticsearch", "9200", true},
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

func TestValidateHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"localhost", "localhost", false},
		{"ipv4 loopback", "127.0.0.1", false},
		{"ipv6 loopback bracket", "[::1]", false},
		{"ipv6 loopback", "::1", false},
		{"other loopback", "127.0.0.2", false},
		{"external ip", "192.168.1.1", true},
		{"domain", "example.com", true},
		{"public ip", "8.8.8.8", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}
