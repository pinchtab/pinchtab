package daemon

import "testing"

func TestNewManagerRejectsUnsupportedOS(t *testing.T) {
	_, err := newManager(environment{osName: "windows"}, &fakeCommandRunner{})
	if err == nil {
		t.Fatal("expected unsupported OS error")
	}
}

func TestStatusLooksRunning(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{
			name:   "launchd running",
			status: "state = running",
			want:   true,
		},
		{
			name:   "systemd running",
			status: "Active: active (running) since Thu 2026-03-12",
			want:   true,
		},
		{
			name:   "stopped",
			status: "Active: inactive (dead)",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusLooksRunning(tt.status); got != tt.want {
				t.Fatalf("StatusLooksRunning(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
