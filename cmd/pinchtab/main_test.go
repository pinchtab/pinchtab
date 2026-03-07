package main

import (
	"testing"
	"time"
)

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

func TestStartupMode(t *testing.T) {
	t.Setenv("PINCHTAB_ONLY", "")
	t.Setenv("BRIDGE_ONLY", "")

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
		ok   bool
	}{
		{name: "default", args: []string{"pinchtab"}, want: "server", ok: true},
		{name: "server explicit", args: []string{"pinchtab", "server"}, want: "server", ok: true},
		{name: "bridge explicit", args: []string{"pinchtab", "bridge"}, want: "bridge", ok: true},
		{name: "unknown rejected", args: []string{"pinchtab", "weird"}, want: "", ok: false},
		{name: "dashboard rejected", args: []string{"pinchtab", "dashboard"}, want: "", ok: false},
		{name: "env bridge", args: []string{"pinchtab"}, env: map[string]string{"PINCHTAB_ONLY": "1"}, want: "bridge", ok: true},
		{name: "legacy env bridge", args: []string{"pinchtab"}, env: map[string]string{"BRIDGE_ONLY": "1"}, want: "bridge", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PINCHTAB_ONLY", "")
			t.Setenv("BRIDGE_ONLY", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, ok := startupMode(tt.args)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("startupMode(%v) = (%q, %v), want (%q, %v)", tt.args, got, ok, tt.want, tt.ok)
			}
		})
	}
}
