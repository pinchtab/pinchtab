package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestApplyRecommendedSecurityDefaults(t *testing.T) {
	allowEvaluate := true
	attachEnabled := true
	fc := &config.FileConfig{
		Server: config.ServerConfig{
			Port:  "9999",
			Bind:  "0.0.0.0",
			Token: "secret",
		},
		Security: config.SecurityConfig{
			AllowEvaluate: &allowEvaluate,
			Attach: config.AttachConfig{
				Enabled:    &attachEnabled,
				AllowHosts: []string{"chrome.internal"},
			},
			IDPI: &config.IDPIConfig{
				Enabled: false,
			},
		},
	}

	ApplyRecommendedSecurityDefaults(fc)

	if fc.Server.Port != "9999" {
		t.Fatalf("expected port to be preserved, got %q", fc.Server.Port)
	}
	if fc.Server.Token != "secret" {
		t.Fatalf("expected token to be preserved, got %q", fc.Server.Token)
	}
	if fc.Server.Bind != "127.0.0.1" {
		t.Fatalf("expected bind to reset to loopback, got %q", fc.Server.Bind)
	}
	if fc.Security.AllowEvaluate == nil || *fc.Security.AllowEvaluate {
		t.Fatalf("expected allowEvaluate to reset to false, got %+v", fc.Security.AllowEvaluate)
	}
	if fc.Security.Attach.Enabled == nil || *fc.Security.Attach.Enabled {
		t.Fatalf("expected attach.enabled to reset to false, got %+v", fc.Security.Attach.Enabled)
	}
	if !fc.Security.IDPI.Enabled {
		t.Fatalf("expected idpi to be enabled")
	}
}

func TestApplyRecommendedSecurityDefaults_LeavesAMissingTokenAlone(t *testing.T) {
	fc := &config.FileConfig{}

	ApplyRecommendedSecurityDefaults(fc)

	if fc.Server.Token != "" {
		t.Fatalf("applying security defaults generated a token %q; whether one may be added to an existing config is ProvisionFileToken's decision, and generating here bypasses the operator-config refusal", fc.Server.Token)
	}
}

func TestRestoreSecurityDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	allowEvaluate := true
	attachEnabled := true
	fc := &config.FileConfig{
		Server: config.ServerConfig{
			Port:  "9999",
			Bind:  "0.0.0.0",
			Token: "secret",
		},
		Security: config.SecurityConfig{
			AllowEvaluate: &allowEvaluate,
			Attach: config.AttachConfig{
				Enabled:    &attachEnabled,
				AllowHosts: []string{"chrome.internal"},
			},
			IDPI: &config.IDPIConfig{
				Enabled: false,
			},
		},
	}
	if err := config.SaveFileConfig(fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	result, err := RestoreSecurityDefaults(false)
	if err != nil {
		t.Fatalf("RestoreSecurityDefaults() error = %v", err)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("RestoreSecurityDefaults() path = %q, want %q", result.ConfigPath, configPath)
	}
	if !result.Written {
		t.Fatalf("RestoreSecurityDefaults() wrote nothing; changes = %+v", result.Changes)
	}

	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	loaded := &config.FileConfig{}
	if err := config.PatchConfigJSON(loaded, string(saved)); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}
	if loaded.Server.Port != "9999" {
		t.Fatalf("expected port to be preserved, got %q", loaded.Server.Port)
	}
	if loaded.Server.Token != "secret" {
		t.Fatalf("expected token to be preserved, got %q", loaded.Server.Token)
	}
	if loaded.Server.Bind != "127.0.0.1" {
		t.Fatalf("expected bind to be restored, got %q", loaded.Server.Bind)
	}
	if loaded.Security.Attach.Enabled == nil || *loaded.Security.Attach.Enabled {
		t.Fatalf("expected attach.enabled to be restored to false, got %+v", loaded.Security.Attach.Enabled)
	}
	if !loaded.Security.IDPI.Enabled {
		t.Fatalf("expected idpi to be enabled after restore")
	}
}

func TestRestoreSecurityDefaults_RefusesToProvisionIntoAnOperatorConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = RestoreSecurityDefaults(false)
	if !errors.Is(err, config.ErrOperatorConfigToken) {
		t.Fatalf("RestoreSecurityDefaults() error = %v, want the operator-config refusal; this path used to generate a credential into the operator's file and discard the error", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the operator's config changed on a refused restore:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRestoreSecurityDefaults_TokenOnlyChangeOnTheDefaultPathIsSaved(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PINCHTAB_CONFIG", "")
	configPath := filepath.Join(tmpHome, ".pinchtab", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}

	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	result, err := RestoreSecurityDefaults(false)
	if err != nil {
		t.Fatalf("RestoreSecurityDefaults() error = %v", err)
	}
	if !result.Written {
		t.Fatalf("RestoreSecurityDefaults() wrote nothing; changes = %+v", result.Changes)
	}
	if len(result.Changes) != 1 || result.Changes[0].Path != "server.token" || result.Changes[0].New != "<generated>" || result.Changes[0].Old != "" {
		t.Fatalf("a token-only restore reports %+v, want one server.token change marked generated", result.Changes)
	}

	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if loaded.Server.Token == "" {
		t.Fatalf("expected generated token to be persisted on the default path")
	}
}

// Every key the file gains is named, and the expected set is the file's own diff
// rather than a hand-kept list, so a recommended default added later cannot ship
// without appearing in the report.
func TestRestoreSecurityDefaultsReportsExactlyTheKeysTheFileGained(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(`{"server":{"port":"9999","bind":"0.0.0.0","token":"secret"},"security":{"allowEvaluate":true}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := flattenedFile(t, configPath)

	result, err := RestoreSecurityDefaults(false)
	if err != nil {
		t.Fatalf("RestoreSecurityDefaults() error = %v", err)
	}
	after := flattenedFile(t, configPath)

	want := map[string][2]string{}
	for path, value := range after {
		if before[path] != value {
			want[path] = [2]string{before[path], value}
		}
	}
	for path, value := range before {
		if _, kept := after[path]; !kept {
			want[path] = [2]string{value, ""}
		}
	}
	if len(want) < 4 {
		t.Fatalf("the file gained only %d keys; this check would prove little", len(want))
	}
	got := map[string][2]string{}
	for _, change := range result.Changes {
		got[change.Path] = [2]string{change.Old, change.New}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report names %v\nfile diff is %v", got, want)
	}
	for _, path := range []string{"security.idpi.enabled", "security.idpi.wrapContent", "security.idpi.scanTimeoutSec"} {
		if _, ok := got[path]; !ok {
			t.Errorf("%s was written silently: not in the report", path)
		}
	}
}

func TestDryRunWritesNothingAndPreviewsTheRealRun(t *testing.T) {
	for name, preset := range map[string]func(bool) (PresetResult, error){
		"up":   RestoreSecurityDefaults,
		"down": ApplyGuardsDownPreset,
	} {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			t.Setenv("PINCHTAB_CONFIG", configPath)
			if err := os.WriteFile(configPath, []byte(`{"server":{"port":"9999","bind":"0.0.0.0","token":"secret"},"security":{"allowEvaluate":true}}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(configPath)

			preview, err := preset(true)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Written || len(preview.Changes) == 0 {
				t.Fatalf("dry run = %+v, want unwritten changes", preview)
			}
			after, _ := os.ReadFile(configPath)
			if !bytes.Equal(before, after) {
				t.Fatalf("dry run changed the file:\n%s\n%s", before, after)
			}

			real, err := preset(false)
			if err != nil {
				t.Fatal(err)
			}
			if !real.Written || !reflect.DeepEqual(real.Changes, preview.Changes) {
				t.Fatalf("real run wrote %+v\npreview said %+v", real.Changes, preview.Changes)
			}
		})
	}
}

// A dry run that mints a credential is worse than no dry run: the token is reported
// as something that would be generated, and the file stays without one.
func TestDryRunReportsATokenItDoesNotProvision(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PINCHTAB_CONFIG", "")
	configPath := filepath.Join(tmpHome, ".pinchtab", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	fc := config.DefaultFileConfig()
	fc.Server.Token = ""
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatal(err)
	}

	preview, err := RestoreSecurityDefaults(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].Path != "server.token" || preview.Changes[0].New != "<generated>" {
		t.Fatalf("preview = %+v, want one server.token change marked generated", preview.Changes)
	}
	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Token != "" {
		t.Fatal("a dry run provisioned a token")
	}
}

func TestTheTokenValueNeverAppearsInAChange(t *testing.T) {
	changes, err := settingChanges([]byte(`{"server":{"token":"old-secret"}}`), []byte(`{"server":{"token":"new-secret"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Old != "<set>" || changes[0].New != "<generated>" {
		t.Fatalf("token change = %+v", changes)
	}
}

func flattenedFile(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		if obj, ok := v.(map[string]any); ok && len(obj) > 0 {
			for k, child := range obj {
				walk(strings.TrimPrefix(prefix+"."+k, "."), child)
			}
			return
		}
		raw, _ := json.Marshal(v)
		out[prefix] = string(raw)
	}
	walk("", doc)
	return out
}
