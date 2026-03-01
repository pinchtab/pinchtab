package uameta

import (
	"testing"
)

func TestBuild_Empty(t *testing.T) {
	if Build("", "") != nil {
		t.Fatal("expected nil for empty chrome version")
	}

	p := Build("", "144.0.0.0")
	if p == nil {
		t.Fatal("expected non-nil for empty user agent with chromeVersion")
	}
	if p.UserAgent == "" {
		t.Fatal("expected generated user agent")
	}
}

func TestBuild_Versions(t *testing.T) {
	p := Build("Mozilla/5.0 Test", "144.0.7559.133")
	if p == nil {
		t.Fatal("expected non-nil")
	}
	meta := p.UserAgentMetadata
	if meta == nil {
		t.Fatal("expected metadata")
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
