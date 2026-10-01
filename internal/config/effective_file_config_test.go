package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEffectiveFileConfigFillsUnsetKeysFromRuntimeDefaults(t *testing.T) {
	fc := FileConfig{}
	fc.Server.Port = "9999"
	fc.Security.AllowEvaluate = new(true)
	fc.InstanceDefaults.MaxTabs = new(0)
	fc.Security.IDPI = &IDPIConfig{Enabled: true}

	got := EffectiveFileConfig(fc)
	defaults := defaultRuntimeConfig()

	if got.Server.Port != "9999" {
		t.Errorf("server.port = %q, want the file value", got.Server.Port)
	}
	if got.Server.Bind != defaults.Bind {
		t.Errorf("server.bind = %q, want default %q", got.Server.Bind, defaults.Bind)
	}
	if got.Security.AllowEvaluate == nil || !*got.Security.AllowEvaluate {
		t.Error("security.allowEvaluate lost the explicit true")
	}
	if got.Security.AllowMacro == nil || *got.Security.AllowMacro {
		t.Error("security.allowMacro should be filled with the default false")
	}
	if got.InstanceDefaults.MaxTabs == nil || *got.InstanceDefaults.MaxTabs != 0 {
		t.Error("instanceDefaults.maxTabs lost the explicit zero")
	}
	if got.Timeouts.ActionSec != int(defaults.ActionTimeout.Seconds()) {
		t.Errorf("timeouts.actionSec = %d, want default", got.Timeouts.ActionSec)
	}
	if got.Security.IDPI == nil || !got.Security.IDPI.Enabled {
		t.Error("security.idpi lost the explicit enabled")
	}
	if len(got.Security.Attach.AllowSchemes) == 0 {
		t.Error("security.attach.allowSchemes should be filled with the default list")
	}
	if got.Server.StateDir != "" || got.Browser.ExtensionPaths != nil {
		t.Error("host-derived paths must stay as the file has them")
	}
}

func TestSavingTheEffectiveConfigOfAnUntouchedFileLeavesItByteIdentical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte("{\n  \"server\": {\n    \"port\": \"9999\"\n  }\n}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	onDisk := savedConfigBaseline()
	if err := json.Unmarshal(original, onDisk); err != nil {
		t.Fatal(err)
	}

	effective := EffectiveFileConfig(*onDisk)
	if err := SaveFileConfig(&effective, path); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("an untouched GET→PUT round-trip rewrote the file:\n%s", got)
	}
}

func TestEffectiveDefaultsAndTheSaveBaselineAgree(t *testing.T) {
	hostDerivedOrStartupStamped := map[string]bool{
		"$schema":                true,
		"configVersion":          true,
		"security.idpi":          true,
		"server.stateDir":        true,
		"browser.extensionPaths": true,
	}
	effective := EffectiveFileConfig(FileConfig{})
	left, err := configAsMap(&effective)
	if err != nil {
		t.Fatal(err)
	}
	right, err := configAsMap(minimalSaveBaseline())
	if err != nil {
		t.Fatal(err)
	}
	a, b := flattenConfigMap("", left), flattenConfigMap("", right)
	for key := range hostDerivedOrStartupStamped {
		delete(a, key)
		delete(b, key)
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			t.Errorf("%s: effective default %s, save baseline %s", key, value, other)
		}
	}
	for key, value := range b {
		if _, ok := a[key]; !ok {
			t.Errorf("%s: save baseline has %s, effective default has nothing", key, value)
		}
	}
}

func flattenConfigMap(prefix string, value any, into ...map[string]string) map[string]string {
	out := map[string]string{}
	if len(into) > 0 {
		out = into[0]
	}
	obj, ok := value.(map[string]any)
	if !ok {
		raw, _ := json.Marshal(value)
		out[prefix] = string(raw)
		return out
	}
	for key, nested := range obj {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		flattenConfigMap(path, nested, out)
	}
	return out
}

func TestAnUntouchedFileAndItsSpelledOutEffectiveConfigLoadTheSameRuntime(t *testing.T) {
	explicitnessMarkers := map[string]bool{"HeadlessSet": true}
	untouched := savedConfigBaseline()
	spelled, err := json.Marshal(EffectiveFileConfig(*untouched))
	if err != nil {
		t.Fatal(err)
	}
	onDisk := savedConfigBaseline()
	if err := json.Unmarshal(spelled, onDisk); err != nil {
		t.Fatal(err)
	}
	fromUntouched, fromSpelled := defaultRuntimeConfig(), defaultRuntimeConfig()
	applyFileConfig(fromUntouched, untouched)
	applyFileConfig(fromSpelled, onDisk)
	a, b := reflect.ValueOf(fromUntouched).Elem(), reflect.ValueOf(fromSpelled).Elem()
	for i := 0; i < a.NumField(); i++ {
		name := a.Type().Field(i).Name
		if explicitnessMarkers[name] || !a.Type().Field(i).IsExported() {
			continue
		}
		left, _ := json.Marshal(a.Field(i).Interface())
		right, _ := json.Marshal(b.Field(i).Interface())
		if string(left) != string(right) {
			t.Errorf("%s: untouched file loads %s, spelled-out effective config loads %s", name, left, right)
		}
	}
}
