package dashboard

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

// The fields are derived by reflection rather than named, so a slice added to
// SecurityConfig later inherits the check.
func TestSensitiveConfigChangesIgnoresAbsentVersusEmptyContainers(t *testing.T) {
	current := config.DefaultFileConfig()
	next := current

	emptied := emptyContainerFields(t, reflect.ValueOf(&next.Security).Elem(), "security")
	emptied = append(emptied, emptyContainerFields(t, reflect.ValueOf(&next.Browser).Elem(), "browser")...)
	if len(emptied) == 0 {
		t.Fatal("no slice or map field found under the sections that gate elevation, so this guard checked nothing")
	}
	t.Logf("emptied %d container fields: %v", len(emptied), emptied)

	changes := sensitiveConfigChanges(&current, &next)
	if changes.requiresElevation {
		t.Errorf("sensitiveConfigChanges() demands elevation for %v, but only absent-versus-empty containers differ: %v", changes.names, emptied)
	}
}

func TestRestartReasonsIncludeStateDirAndDerivedProfilesDir(t *testing.T) {
	boot := config.DefaultFileConfig()
	boot.Server.StateDir = filepath.Join(t.TempDir(), "old")
	boot.Profiles.BaseDir = ""
	api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = boot

	next := boot
	next.Server.StateDir = filepath.Join(t.TempDir(), "new")
	reasons := api.restartReasonsFor(next)
	for _, want := range []string{"Server state directory (server.stateDir)", "Profiles directory"} {
		if !containsString(reasons, want) {
			t.Errorf("restartReasonsFor() = %v, want %q", reasons, want)
		}
	}
}

func TestRestartReasonsOmitStateDirWhenUnchanged(t *testing.T) {
	boot := config.DefaultFileConfig()
	boot.Server.StateDir = filepath.Join(t.TempDir(), "state")
	api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = boot

	if reasons := api.restartReasonsFor(boot); containsString(reasons, "Server state directory (server.stateDir)") {
		t.Fatalf("restartReasonsFor() = %v, unexpectedly reports unchanged state dir", reasons)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSensitiveConfigChangesStillReportsARealSecurityEdit(t *testing.T) {
	current := config.DefaultFileConfig()
	next := current
	next.Security.AllowedDomains = append(append([]string(nil), current.Security.AllowedDomains...), "example.com")

	changes := sensitiveConfigChanges(&current, &next)
	if !changes.requiresElevation {
		t.Fatal("sensitiveConfigChanges() allows an allowlist edit without elevation")
	}
	if len(changes.names) != 1 || changes.names[0] != "security" {
		t.Fatalf("changes.names = %v, want [security]", changes.names)
	}
}

func TestSameConfigSectionTreatsUnmarshalableValuesAsChanged(t *testing.T) {
	if sameConfigSection(func() {}, func() {}) {
		t.Error("sameConfigSection() reports two unmarshalable values as equal; a section it cannot read must reach the elevation gate")
	}
}

func TestRestartReasonsIgnoreAbsentVersusEmptyContainers(t *testing.T) {
	boot := config.DefaultFileConfig()
	api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = boot

	next := boot
	if len(emptyContainerFields(t, reflect.ValueOf(&next.Security).Elem(), "security")) == 0 {
		t.Fatal("no slice or map field found under SecurityConfig, so this guard checked nothing")
	}

	for _, reason := range api.restartReasonsFor(next) {
		if reason == "Security policy" {
			t.Error("restartReasonsFor() demands a restart for Security policy, but only absent-versus-empty containers differ")
		}
	}
}

func emptyContainerFields(t *testing.T, v reflect.Value, path string) []string {
	t.Helper()

	var touched []string
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			touched = append(touched, path)
		}
	case reflect.Map:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeMap(v.Type()))
			touched = append(touched, path)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			touched = append(touched, emptyContainerFields(t, v.Field(i), path+"."+jsonFieldName(field))...)
		}
	case reflect.Ptr:
		if !v.IsNil() {
			touched = append(touched, emptyContainerFields(t, v.Elem(), path)...)
		}
	}
	return touched
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	for i := range len(tag) {
		if tag[i] == ',' {
			tag = tag[:i]
			break
		}
	}
	if tag == "" || tag == "-" {
		return field.Name
	}
	return tag
}

func TestEmptyContainerFieldsKeepsTheJSONDocumentIdentical(t *testing.T) {
	before := config.DefaultFileConfig()
	after := before
	if len(emptyContainerFields(t, reflect.ValueOf(&after.Security).Elem(), "security")) == 0 {
		t.Fatal("no container field emptied, so the other guards in this file check nothing")
	}

	left, err := json.Marshal(before.Security)
	if err != nil {
		t.Fatalf("Marshal(before): %v", err)
	}
	right, err := json.Marshal(after.Security)
	if err != nil {
		t.Fatalf("Marshal(after): %v", err)
	}
	if string(left) != string(right) {
		t.Fatalf("emptying nil containers changed the JSON document, so these fixtures differ in settings and not only in representation:\n%s\n%s", left, right)
	}
}
