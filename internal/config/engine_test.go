package config

import (
	"strings"
	"testing"
)

func TestValidate_BrowserProviderTriggersError(t *testing.T) {
	fc := &FileConfig{
		Browser: BrowserConfig{Provider: "cloak"},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "browser.provider is no longer supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected validation error for browser.provider, got: %v", errs)
	}
}

func TestValidate_ServerEngineTriggersError(t *testing.T) {
	fc := &FileConfig{
		Server: ServerConfig{Engine: "chrome"},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "server.engine is no longer supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected validation error for server.engine, got: %v", errs)
	}
}
