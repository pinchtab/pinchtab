package main

import "testing"

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name     string
		cfgPort  string
		override string
		want     string
		wantErr  bool
	}{
		{"rejects empty", "", "", "", true},
		{"uses config port", "8080", "", "8080", false},
		{"override wins", "8080", "7777", "7777", false},
		{"override wins over empty config", "", "7777", "7777", false},
		{"min valid port", "", "1", "1", false},
		{"max valid port", "", "65535", "65535", false},
		{"rejects zero", "", "0", "", true},
		{"rejects negative", "", "-1", "", true},
		{"rejects too high", "", "65536", "", true},
		{"rejects non-numeric", "", "abc", "", true},
		{"rejects non-numeric config", "notaport", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePort(tt.cfgPort, tt.override)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolvePort(%q, %q) expected error, got %q", tt.cfgPort, tt.override, got)
				}
				return
			}
			if err != nil {
				t.Errorf("resolvePort(%q, %q) unexpected error: %v", tt.cfgPort, tt.override, err)
				return
			}
			if got != tt.want {
				t.Errorf("resolvePort(%q, %q) = %q, want %q", tt.cfgPort, tt.override, got, tt.want)
			}
		})
	}
}

func TestResolveHost(t *testing.T) {
	tests := []struct {
		bind string
		want string
	}{
		{"", "127.0.0.1"},
		{"loopback", "127.0.0.1"},
		{"localhost", "127.0.0.1"},
		{"lan", "127.0.0.1"},
		{"0.0.0.0", "127.0.0.1"},
		{"192.168.1.100", "192.168.1.100"},
		{"10.0.0.5", "10.0.0.5"},
	}
	for _, tt := range tests {
		t.Run("bind="+tt.bind, func(t *testing.T) {
			got := resolveHost(tt.bind)
			if got != tt.want {
				t.Errorf("resolveHost(%q) = %q, want %q", tt.bind, got, tt.want)
			}
		})
	}
}

func TestIsDashboardReachable_Unreachable(t *testing.T) {
	// A port that almost certainly has nothing listening
	if isDashboardReachable("127.0.0.1", "19") {
		t.Error("expected unreachable port to return false")
	}
}
