package bridge

import (
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

func TestDetectPlatform(t *testing.T) {
	platform, arch := detectPlatform()
	if platform == "" || arch == "" {
		t.Fatal("expected non-empty platform and arch")
	}
}
