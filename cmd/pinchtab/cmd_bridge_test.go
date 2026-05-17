package main

import "testing"

func TestResolveBridgeEngine(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		cfgValue  string
		want      string
		wantErr   bool
	}{
		{name: "config default", cfgValue: "lite", want: "lite"},
		{name: "flag overrides config", flagValue: "auto", cfgValue: "chrome", want: "auto"},
		{name: "empty falls back to chrome", want: "chrome"},
		{name: "invalid", flagValue: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBridgeEngine(tt.flagValue, tt.cfgValue)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestValidateBridgeCDPURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "browser websocket", raw: "ws://127.0.0.1:9222/devtools/browser/abc"},
		{name: "http origin", raw: "http://127.0.0.1:9222"},
		{name: "json version", raw: "https://cdp.example/json/version"},
		{name: "page websocket rejected", raw: "ws://127.0.0.1:9222/devtools/page/abc", wantErr: true},
		{name: "websocket without browser path rejected", raw: "ws://127.0.0.1:9222", wantErr: true},
		{name: "missing scheme rejected", raw: "127.0.0.1:9222", wantErr: true},
		{name: "unsupported scheme rejected", raw: "ftp://127.0.0.1:9222", wantErr: true},
		{name: "bad http path rejected", raw: "http://127.0.0.1:9222/devtools/page/abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateBridgeCDPURL(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
