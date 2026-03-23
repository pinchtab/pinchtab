package bridge

import (
	"strings"
	"testing"
)

func TestBuild_Empty(t *testing.T) {
	if buildUserAgentOverride("", "") != nil {
		t.Fatal("expected nil for empty chrome version")
	}

	p := buildUserAgentOverride("", "144.0.0.0")
	if p == nil {
		t.Fatal("expected non-nil for empty user agent with chromeVersion")
		return
	}
	if p.UserAgent == "" {
		t.Fatal("expected generated user agent")
	}
}

func TestBuild_UsesResolvedUserAgent(t *testing.T) {
	p := buildUserAgentOverride("", "144.0.0.0")
	if p == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(p.UserAgent, "Chrome/144.0.0.0") {
		t.Fatalf("expected resolved UA to contain full Chrome version, got %q", p.UserAgent)
	}
}

func TestBuild_Versions(t *testing.T) {
	p := buildUserAgentOverride("Mozilla/5.0 Test", "144.0.7559.133")
	if p == nil {
		t.Fatal("expected non-nil")
		return
	}
	meta := p.UserAgentMetadata
	if meta == nil {
		t.Fatal("expected metadata")
		return
	}
	for _, b := range meta.Brands {
		if b.Brand == "Google Chrome" && b.Version != "144" {
			t.Errorf("expected major version 144, got %s", b.Version)
		}
	}
	for _, b := range meta.FullVersionList {
		if b.Brand == "Google Chrome" && b.Version != "144.0.7559.133" {
			t.Errorf("expected full version 144.0.7559.133, got %s", b.Version)
		}
	}
}

func TestBuild_UsesPersonaMetadata(t *testing.T) {
	p := buildUserAgentOverride("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.7559.133 Safari/537.36", "144.0.7559.133")
	if p == nil || p.UserAgentMetadata == nil {
		t.Fatal("expected metadata")
	}
	if p.AcceptLanguage != "en-US,en" {
		t.Fatalf("expected accept language from persona, got %q", p.AcceptLanguage)
	}
	if p.Platform != "Win32" {
		t.Fatalf("expected navigator platform Win32, got %q", p.Platform)
	}
	if got := p.UserAgentMetadata.Platform; got != "Windows" {
		t.Fatalf("expected UA data platform Windows, got %q", got)
	}
}

func TestBuildLocaleOverride_UsesPersonaLanguage(t *testing.T) {
	p := buildLocaleOverride("", "144.0.7559.133")
	if p == nil {
		t.Fatal("expected locale override")
	}
	if p.Locale != "en-US" {
		t.Fatalf("expected locale override en-US, got %q", p.Locale)
	}
}
