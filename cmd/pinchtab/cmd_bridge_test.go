package main

import (
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
)

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

func TestResolveBridgeBrowser(t *testing.T) {
	tests := []struct {
		name        string
		browserFlag string
		configured  []string
		want        string
		wantErr     bool
	}{
		{name: "browser flag sets cloak", browserFlag: "cloak", want: "cloak"},
		{name: "browser flag sets chrome", browserFlag: "chrome", want: "chrome"},
		{name: "no flag returns empty", want: ""},
		{name: "invalid browser returns error", browserFlag: "netscape", wantErr: true},
		{name: "configured browser accepted", browserFlag: "my-custom", configured: []string{"my-custom"}, want: "my-custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBridgeBrowser(tt.browserFlag, tt.configured)
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

func TestBridgeAttachChildFlagContract(t *testing.T) {
	for _, name := range []string{"cdp-attach", "browser", "remote-browser-name"} {
		if bridgeCmd.Flags().Lookup(name) == nil {
			t.Errorf("bridge command missing child flag %q", name)
		}
	}
	if bridgeCmd.Flags().Lookup("browser-provider") != nil {
		t.Error("bridge command must not register obsolete browser-provider flag")
	}
}
