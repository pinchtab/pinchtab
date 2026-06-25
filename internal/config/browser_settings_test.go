package config

import "testing"

func TestFileConfigBrowserAccessors(t *testing.T) {
	fc := &FileConfig{
		Browser: BrowserConfig{
			BrowserVersion:    "144.0.0.0",
			BrowserBinary:     " /opt/browser ",
			BrowserExtraFlags: "--flag",
		},
	}

	if got := fc.BrowserVersion(); got != "144.0.0.0" {
		t.Fatalf("BrowserVersion() = %q", got)
	}
	if got := fc.BrowserBinary(); got != "/opt/browser" {
		t.Fatalf("BrowserBinary() = %q", got)
	}
	if got := fc.BrowserDebugPort(); got != 0 {
		t.Fatalf("BrowserDebugPort() = %d, want 0", got)
	}
	if got := fc.BrowserExtraFlags(); got != "--flag" {
		t.Fatalf("BrowserExtraFlags() = %q", got)
	}

	fc.SetBrowserDebugPort(9555)
	if fc.Browser.BrowserDebugPort == nil || *fc.Browser.BrowserDebugPort != 9555 {
		t.Fatalf("BrowserDebugPort = %v, want 9555", fc.Browser.BrowserDebugPort)
	}

	fc.SetBrowserDebugPort(0)
	if fc.Browser.BrowserDebugPort != nil {
		t.Fatalf("BrowserDebugPort = %v, want nil", fc.Browser.BrowserDebugPort)
	}
}
