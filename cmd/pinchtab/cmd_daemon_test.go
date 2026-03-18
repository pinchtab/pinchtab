package main

import "testing"

func TestDaemonMenuOptions(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
		running   bool
		want      []string
	}{
		{
			name:      "not installed",
			installed: false,
			running:   false,
			want:      []string{"install", "exit"},
		},
		{
			name:      "installed stopped",
			installed: true,
			running:   false,
			want:      []string{"start", "uninstall", "exit"},
		},
		{
			name:      "installed running",
			installed: true,
			running:   true,
			want:      []string{"stop", "restart", "uninstall", "exit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daemonMenuOptions(tt.installed, tt.running)
			if len(got) != len(tt.want) {
				t.Fatalf("len(daemonMenuOptions()) = %d, want %d", len(got), len(tt.want))
			}
			for i, want := range tt.want {
				if got[i].value != want {
					t.Fatalf("daemonMenuOptions()[%d] = %q, want %q", i, got[i].value, want)
				}
			}
		})
	}
}
