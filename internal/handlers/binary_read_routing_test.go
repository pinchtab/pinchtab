package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestBinaryReadsEnsureRoutedBrowserTargetConfig(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		handle func(*Handlers, http.ResponseWriter, *http.Request)
	}{
		{"pdf", "/pdf?browser=cloak", (*Handlers).HandlePDF},
		{"screenshot", "/screenshot?browser=cloak", (*Handlers).HandleScreenshot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockBridge{ensureBrowserErr: errors.New("stop after ensure")}
			h := New(m, &config.RuntimeConfig{
				DefaultBrowser:    config.BrowserChrome,
				BrowsersAvailable: []string{"chrome", "cloak"},
				BrowserBinary:     "/usr/bin/chrome",
				Targets: config.BrowserTargetsConfig{
					"cloak-local": {Provider: config.BrowserCloak, Binary: "/opt/cloakbrowser/chrome"},
				},
			}, nil, nil, nil)

			tc.handle(h, httptest.NewRecorder(), httptest.NewRequest("GET", tc.path, nil))

			if m.ensureBrowserCfg == nil {
				t.Fatal("EnsureBrowser was not called")
			}
			if m.ensureBrowserCfg.BrowserBinary != "/opt/cloakbrowser/chrome" {
				t.Fatalf("BrowserBinary = %q, want the routed cloak target binary", m.ensureBrowserCfg.BrowserBinary)
			}
		})
	}
}
