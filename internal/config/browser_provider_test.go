package config

import (
	"strings"
	"testing"
)

func TestParseBrowserProvider(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "empty defaults chrome", input: "", want: BrowserProviderChrome},
		{name: "chrome", input: "chrome", want: BrowserProviderChrome},
		{name: "cloak trimmed case", input: " Cloak ", want: BrowserProviderCloak},
		{name: "rejects unknown", input: "cloack", wantErr: "invalid browser provider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBrowserProvider(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseBrowserProvider(%q) error = %v, want %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBrowserProvider(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseBrowserProvider(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCloakStealthBooleansAreIndependent(t *testing.T) {
	chromeRemote := &RuntimeConfig{
		BrowserProvider: BrowserProviderChrome,
		RemoteCDPURL:    "ws://127.0.0.1:9222/devtools/browser/id",
	}
	if CloakBrowserProviderActive(chromeRemote) {
		t.Fatal("remote CDP attach must not imply native Cloak stealth without cloak provider")
	}
	if PinchTabStealthDefaultsDisabled(chromeRemote) {
		t.Fatal("remote CDP attach must not disable PinchTab stealth defaults without cloak provider opt-in")
	}

	cloakWithDefaults := &RuntimeConfig{
		BrowserProvider: BrowserProviderCloak,
	}
	if !CloakBrowserProviderActive(cloakWithDefaults) {
		t.Fatal("cloak provider should report native Cloak stealth active")
	}
	if PinchTabStealthDefaultsDisabled(cloakWithDefaults) {
		t.Fatal("native Cloak stealth must not imply PinchTab defaults are disabled")
	}

	cloakWithoutPinchTabDefaults := &RuntimeConfig{
		BrowserProvider: BrowserProviderCloak,
		Cloak: CloakBrowserRuntimeConfig{
			DisableDefaultStealthArgs: true,
		},
	}
	if !CloakBrowserProviderActive(cloakWithoutPinchTabDefaults) {
		t.Fatal("cloak provider should report native Cloak stealth active")
	}
	if !PinchTabStealthDefaultsDisabled(cloakWithoutPinchTabDefaults) {
		t.Fatal("explicit disableDefaultStealthArgs should disable PinchTab defaults")
	}
}
