package doctor

import (
	"bytes"
	"testing"
)

func TestCheckMarker(t *testing.T) {
	cases := map[CheckStatus]string{
		StatusPass:        "OK  ",
		StatusFail:        "FAIL",
		StatusWarn:        "WARN",
		StatusSkip:        "SKIP",
		CheckStatus("??"): "?   ",
	}
	for status, want := range cases {
		if got := CheckMarker(status); got != want {
			t.Errorf("CheckMarker(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestBrowserStatusMarker(t *testing.T) {
	cases := map[string]string{
		"ready":        "✓",
		"needs-config": "~",
		"missing":      "✗",
		"":             "✗",
		"unknown":      "✗",
	}
	for status, want := range cases {
		if got := BrowserStatusMarker(status); got != want {
			t.Errorf("BrowserStatusMarker(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestCheckDetail_FallsBackToErrMsg(t *testing.T) {
	if got := CheckDetail(CheckResult{Detail: "all good"}); got != "all good" {
		t.Errorf("CheckDetail with detail = %q, want %q", got, "all good")
	}
	if got := CheckDetail(CheckResult{ErrMsg: "boom"}); got != "boom" {
		t.Errorf("CheckDetail empty detail should fall back to ErrMsg, got %q", got)
	}
	if got := CheckDetail(CheckResult{Detail: "primary", ErrMsg: "secondary"}); got != "primary" {
		t.Errorf("CheckDetail should prefer Detail over ErrMsg, got %q", got)
	}
}

func TestWriteBrowserCheckRow(t *testing.T) {
	var buf bytes.Buffer
	WriteBrowserCheckRow(&buf, CheckResult{Name: "binary_exists", Status: StatusPass, Detail: "/usr/bin/chrome"})
	if got, want := buf.String(), "    OK   binary_exists: /usr/bin/chrome\n"; got != want {
		t.Errorf("WriteBrowserCheckRow = %q, want %q", got, want)
	}

	buf.Reset()
	WriteBrowserCheckRow(&buf, CheckResult{Name: "version", Status: StatusFail, ErrMsg: "not found"})
	if got, want := buf.String(), "    FAIL version: not found\n"; got != want {
		t.Errorf("WriteBrowserCheckRow fallback = %q, want %q", got, want)
	}
}

func TestBrowserInstallHintsHasKnownBrowsers(t *testing.T) {
	for _, name := range []string{"chrome", "cloak", "ghost-chrome"} {
		if _, ok := BrowserInstallHints[name]; !ok {
			t.Errorf("BrowserInstallHints missing key %q", name)
		}
	}
}
