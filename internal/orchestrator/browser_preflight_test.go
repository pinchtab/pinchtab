package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestBrowserUnavailableReason(t *testing.T) {
	req := func(target string) *http.Request {
		return httptest.NewRequest(http.MethodGet, target, nil)
	}
	existing := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(existing, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing browser.binary override is reported", func(t *testing.T) {
		o := &Orchestrator{runtimeCfg: &config.RuntimeConfig{
			DefaultBrowser: "chrome",
			BrowserBinary:  filepath.Join(t.TempDir(), "missing"),
		}}
		reason, unavailable := o.BrowserUnavailableReason(req("/text?url=http://x/"))
		if !unavailable {
			t.Fatal("expected a missing override to be reported unavailable")
		}
		if reason == "" {
			t.Error("expected a non-empty reason")
		}
	})

	t.Run("existing override is available", func(t *testing.T) {
		o := &Orchestrator{runtimeCfg: &config.RuntimeConfig{
			DefaultBrowser: "chrome",
			BrowserBinary:  existing,
		}}
		if _, unavailable := o.BrowserUnavailableReason(req("/text?url=http://x/")); unavailable {
			t.Error("an existing override binary should be available")
		}
	})

	t.Run("explicit per-request browser falls through", func(t *testing.T) {
		o := &Orchestrator{runtimeCfg: &config.RuntimeConfig{
			DefaultBrowser: "chrome",
			BrowserBinary:  filepath.Join(t.TempDir(), "missing"),
		}}
		// An explicit ?browser= override must not be short-circuited here.
		if _, unavailable := o.BrowserUnavailableReason(req("/text?browser=chrome&url=http://x/")); unavailable {
			t.Error("explicit per-request browser should fall through, not fast-fail")
		}
	})

	t.Run("external CDP attach falls through", func(t *testing.T) {
		o := &Orchestrator{runtimeCfg: &config.RuntimeConfig{
			DefaultBrowser: "chrome",
			BrowserBinary:  filepath.Join(t.TempDir(), "missing"),
			CDPAttachURL:   "http://127.0.0.1:9222",
		}}
		if _, unavailable := o.BrowserUnavailableReason(req("/text?url=http://x/")); unavailable {
			t.Error("CDP attach needs no local binary; should fall through")
		}
	})

	t.Run("nil config is safe", func(t *testing.T) {
		o := &Orchestrator{}
		if _, unavailable := o.BrowserUnavailableReason(req("/text")); unavailable {
			t.Error("nil runtimeCfg must not report unavailable")
		}
	})
}
