package config

import (
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

func TestParseBrowser(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		configured []string
		want       string
		wantErr    string
	}{
		{name: "empty defaults chrome", input: "", want: BrowserChrome},
		{name: "chrome", input: "chrome", want: BrowserChrome},
		{name: "cloak trimmed case", input: " Cloak ", want: BrowserCloak},
		{name: "rejects unknown", input: "cloack", wantErr: "unknown browser"},
		{name: "configured-only name accepted", input: "my-custom", configured: []string{"my-custom"}, want: "my-custom"},
		{name: "registry name accepted with nil configured", input: "cloak", want: "cloak"},
		{name: "unknown name returns error", input: "netscape", wantErr: "unknown browser"},
		{name: "unknown with configured shows both lists", input: "bad", configured: []string{"alpha", "beta"}, wantErr: "configured:"},
		{name: "unknown with configured shows built-in", input: "bad", configured: []string{"alpha"}, wantErr: "built-in:"},
		{name: "unknown without configured shows known", input: "bad", wantErr: "known:"},
		{name: "nil configured falls back to registry", input: "ghost-chrome", want: "ghost-chrome"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBrowser(tt.input, tt.configured)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseBrowser(%q, %v) error = %v, want %q", tt.input, tt.configured, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBrowser(%q, %v) unexpected error: %v", tt.input, tt.configured, err)
			}
			if got != tt.want {
				t.Fatalf("ParseBrowser(%q, %v) = %q, want %q", tt.input, tt.configured, got, tt.want)
			}
		})
	}
}

func TestResolveBrowser(t *testing.T) {
	tests := []struct {
		name            string
		request         string
		session         string
		instance        string
		globalDefault   string
		configuredOrder []string
		want            string
	}{
		{name: "request wins", request: "cloak", session: "chrome", instance: "chrome", globalDefault: "chrome", want: "cloak"},
		{name: "session wins when no request", session: "cloak", instance: "chrome", globalDefault: "chrome", want: "cloak"},
		{name: "instance wins when no request or session", instance: "cloak", globalDefault: "chrome", want: "cloak"},
		{name: "global default wins when nothing else set", globalDefault: "ghost-chrome", want: "ghost-chrome"},
		{name: "chrome fallback when all empty", want: "chrome"},
		{name: "request overrides all", request: "ghost-chrome", session: "cloak", instance: "chrome", globalDefault: "chrome", want: "ghost-chrome"},
		{name: "session overrides global default without instance", session: "cloak", globalDefault: "chrome", want: "cloak"},
		{name: "request wins with conflicting session and instance", request: "chrome", session: "cloak", instance: "ghost-chrome", globalDefault: "ghost-chrome", want: "chrome"},
		{name: "instance overrides global default without session", instance: "ghost-chrome", globalDefault: "cloak", want: "ghost-chrome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBrowser(tt.request, tt.session, tt.instance, tt.globalDefault, tt.configuredOrder)
			if got != tt.want {
				t.Errorf("ResolveBrowser(%q, %q, %q, %q, %v) = %q, want %q",
					tt.request, tt.session, tt.instance, tt.globalDefault, tt.configuredOrder, got, tt.want)
			}
		})
	}
}

func TestResolveBrowser_ConfiguredOrder(t *testing.T) {
	got := ResolveBrowser("", "", "", "", []string{"ghost-chrome", "cloak"})
	if got != "ghost-chrome" {
		t.Errorf("ResolveBrowser with configuredOrder [ghost-chrome cloak] = %q, want %q", got, "ghost-chrome")
	}
}

func TestResolveBrowser_DefaultWinsOverOrder(t *testing.T) {
	got := ResolveBrowser("", "", "", "cloak", []string{"ghost-chrome"})
	if got != "cloak" {
		t.Errorf("ResolveBrowser with default=cloak and order=[ghost-chrome] = %q, want %q", got, "cloak")
	}
}

func TestResolveBrowser_EmptyOrderFallsToChrome(t *testing.T) {
	got := ResolveBrowser("", "", "", "", nil)
	if got != "chrome" {
		t.Errorf("ResolveBrowser with nil order = %q, want %q", got, "chrome")
	}
}

func TestCloakStealthBooleansAreIndependent(t *testing.T) {
	chromeRemote := &RuntimeConfig{
		DefaultBrowser: BrowserChrome,
		RemoteCDPURL:   "ws://127.0.0.1:9222/devtools/browser/id",
	}
	if CloakBrowserActive(chromeRemote) {
		t.Fatal("remote CDP attach must not imply native Cloak stealth without cloak provider")
	}
	if PinchTabStealthDefaultsDisabled(chromeRemote) {
		t.Fatal("remote CDP attach must not disable PinchTab stealth defaults without cloak provider opt-in")
	}

	cloakWithDefaults := &RuntimeConfig{
		DefaultBrowser: BrowserCloak,
	}
	if !CloakBrowserActive(cloakWithDefaults) {
		t.Fatal("cloak provider should report native Cloak stealth active")
	}
	if PinchTabStealthDefaultsDisabled(cloakWithDefaults) {
		t.Fatal("native Cloak stealth must not imply PinchTab defaults are disabled")
	}

	cloakWithoutPinchTabDefaults := &RuntimeConfig{
		DefaultBrowser: BrowserCloak,
		Cloak: CloakBrowserRuntimeConfig{
			DisableDefaultStealthArgs: true,
		},
	}
	if !CloakBrowserActive(cloakWithoutPinchTabDefaults) {
		t.Fatal("cloak provider should report native Cloak stealth active")
	}
	if !PinchTabStealthDefaultsDisabled(cloakWithoutPinchTabDefaults) {
		t.Fatal("explicit disableDefaultStealthArgs should disable PinchTab defaults")
	}
}
