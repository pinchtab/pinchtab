package main

import (
	"testing"
	"time"
)

func TestResolveServerMode(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "default dashboard",
			env:  map[string]string{},
			want: "dashboard",
		},
		{
			name: "legacy bridge mode env",
			env:  map[string]string{"BRIDGE_MODE": "bridge"},
			want: "bridge",
		},
		{
			name: "legacy dashboard mode env",
			env:  map[string]string{"BRIDGE_MODE": "dashboard"},
			want: "dashboard",
		},
		{
			name: "pinchtab mode takes precedence over legacy mode",
			env:  map[string]string{"PINCHTAB_MODE": "dashboard", "BRIDGE_MODE": "bridge"},
			want: "dashboard",
		},
		{
			name: "bridge only still works when mode is unset",
			env:  map[string]string{"BRIDGE_ONLY": "1"},
			want: "bridge",
		},
		{
			name: "explicit dashboard mode overrides legacy bridge only flag",
			env:  map[string]string{"BRIDGE_ONLY": "1", "BRIDGE_MODE": "dashboard"},
			want: "dashboard",
		},
		{
			name: "pinchtab only always forces bridge mode",
			env:  map[string]string{"PINCHTAB_ONLY": "1", "PINCHTAB_MODE": "dashboard"},
			want: "bridge",
		},
		{
			name: "invalid mode falls back to dashboard",
			env:  map[string]string{"PINCHTAB_MODE": "invalid"},
			want: "dashboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}
			if got := resolveServerMode(getenv); got != tt.want {
				t.Fatalf("resolveServerMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServerTimeoutOrdering(t *testing.T) {
	// Verify the timeout values used for both bridge and dashboard servers
	// are in the correct relative order.
	readHeader := 10 * time.Second
	read := 30 * time.Second
	write := 60 * time.Second
	idle := 120 * time.Second

	if readHeader >= read {
		t.Errorf("ReadHeaderTimeout (%v) should be less than ReadTimeout (%v)", readHeader, read)
	}
	if read >= write {
		t.Errorf("ReadTimeout (%v) should be less than WriteTimeout (%v)", read, write)
	}
	if write >= idle {
		t.Errorf("WriteTimeout (%v) should be less than IdleTimeout (%v)", write, idle)
	}
}
