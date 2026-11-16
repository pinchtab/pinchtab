package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// The resolver preserves absolute and empty paths while anchoring relative paths.
func TestConfiguredPathResolution(t *testing.T) {
	base := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "browser")
	for _, tc := range []struct{ name, input, want string }{
		{"relative", filepath.Join("profiles", "automation"), filepath.Join(base, "profiles", "automation")},
		{"parent", filepath.Join("..", "state"), filepath.Join(filepath.Dir(base), "state")},
		{"absolute", absolute, absolute},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveConfiguredPath(base, tc.input); got != tc.want {
				t.Fatalf("resolved path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFileConfigResolvesPathsWithoutMutatingSource(t *testing.T) {
	base := t.TempDir()
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(base, "config.json"))
	t.Chdir(t.TempDir())
	file := DefaultFileConfig()
	file.Server.StateDir = "state"
	file.Profiles.BaseDir = "profiles"
	file.Browser.BrowserBinary = "chrome"
	file.Browser.ExtensionPaths = []string{"extension"}
	file.Browser.Cloak.FontsDir = "fonts"
	file.Browser.Targets = BrowserTargetsConfig{"alternate": {Provider: BrowserChrome, Binary: "alternate-chrome"}}
	target := file.Browser.Targets["alternate"]
	target.Cloak.FontsDir = "alternate-fonts"
	file.Browser.Targets["alternate"] = target
	before, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	var runtime RuntimeConfig
	applyFileConfig(&runtime, &file)
	for _, tc := range []struct{ name, got, relative string }{
		{"state", runtime.StateDir, "state"},
		{"profiles", runtime.ProfilesBaseDir, "profiles"},
		{"binary", runtime.BrowserBinary, "chrome"},
		{"fonts", runtime.Cloak.FontsDir, "fonts"},
		{"target binary", runtime.Targets["alternate"].Binary, "alternate-chrome"},
		{"target fonts", runtime.Targets["alternate"].Cloak.FontsDir, "alternate-fonts"},
	} {
		if want := filepath.Join(base, tc.relative); tc.got != want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, want)
		}
	}
	if len(runtime.ExtensionPaths) != 1 || runtime.ExtensionPaths[0] != filepath.Join(base, "extension") {
		t.Errorf("extensions = %v", runtime.ExtensionPaths)
	}
	after, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("applying config mutated source paths")
	}
}
