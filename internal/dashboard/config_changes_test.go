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

type configSectionDisposition string

const (
	configAppliesLive  configSectionDisposition = "applies live"
	configNeedsRestart configSectionDisposition = "needs restart"
	configIsInert      configSectionDisposition = "deliberately inert"
)

type configSectionCensusRow struct {
	disposition configSectionDisposition
	evidence    string
	mutate      func(*config.FileConfig)
	wantReason  string
}

// fileConfigSectionCensus is the review point for PUT /api/config semantics.
// Live rows are published by persistAndApply through NextRuntimeConfig; restart
// rows name a representative frozen setting whose clause is exercised below;
// the two metadata fields are persisted but intentionally have no runtime effect.
// Profiles.BaseDir is the only effective section value derived from another
// section: when empty it follows Server.StateDir. effectiveProfilesDir is therefore
// deliberately part of the restart check instead of comparing the literal fields.
var fileConfigSectionCensus = map[string]configSectionCensusRow{
	"Schema":           {configIsInert, "$schema is editor metadata only", nil, ""},
	"ConfigVersion":    {configIsInert, "configVersion selects file compatibility while loading", nil, ""},
	"Server":           {configNeedsRestart, "listener and state stores are constructed at boot", func(c *config.FileConfig) { c.Server.StateDir += "-moved" }, "Server state directory (server.stateDir)"},
	"Browser":          {configAppliesLive, "ApplyFileConfigToRuntime publishes browser settings for subsequent browser work", nil, ""},
	"InstanceDefaults": {configNeedsRestart, "the running default instance keeps its boot stealth level", func(c *config.FileConfig) { c.InstanceDefaults.StealthLevel = "full" }, "Stealth level"},
	"Security": {configNeedsRestart, "the front-door security policy is assembled at boot", func(c *config.FileConfig) {
		c.Security.AllowedDomains = append(c.Security.AllowedDomains, "census.invalid")
	}, "Security policy"},
	"Profiles":      {configNeedsRestart, "profile storage is opened at boot", func(c *config.FileConfig) { c.Profiles.BaseDir += "-moved" }, "Profiles directory"},
	"MultiInstance": {configNeedsRestart, "routing strategy and restart supervisor are constructed at boot", func(c *config.FileConfig) { c.MultiInstance.Strategy = "explicit" }, "Routing strategy"},
	"Timeouts":      {configAppliesLive, "ApplyFileConfigToRuntime publishes request timeouts", nil, ""},
	"Scheduler":     {configAppliesLive, "ApplyRuntimeConfig updates the orchestrator scheduler", nil, ""},
	"Observability": {configAppliesLive, "activity consumers resolve the published runtime config", nil, ""},
	"Sessions":      {configNeedsRestart, "enabling the agent route family requires boot-time route registration", func(c *config.FileConfig) { enabled := true; c.Sessions.Agent.Enabled = &enabled }, "Agent sessions"},
	"AutoSolver":    {configAppliesLive, "ApplyFileConfigToRuntime publishes solver settings", nil, ""},
	"Browsers":      {configAppliesLive, "ApplyFileConfigToRuntime publishes provider selection", nil, ""},
}

func TestEveryFileConfigSectionHasAnEffectiveDisposition(t *testing.T) {
	typ := reflect.TypeOf(config.FileConfig{})
	seen := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		seen[field.Name] = true
		row, ok := fileConfigSectionCensus[field.Name]
		if !ok {
			t.Errorf("FileConfig.%s is unclassified: add it to fileConfigSectionCensus as live, restart-required, or deliberately inert; an unclassified section can make PUT /api/config report an unapplied save as applied", field.Name)
			continue
		}
		if row.evidence == "" {
			t.Errorf("FileConfig.%s has no evidence for its %q classification", field.Name, row.disposition)
		}
		switch row.disposition {
		case configAppliesLive, configIsInert:
			if row.mutate != nil || row.wantReason != "" {
				t.Errorf("FileConfig.%s is %q but declares a restart mutation/reason", field.Name, row.disposition)
			}
		case configNeedsRestart:
			if row.mutate == nil || row.wantReason == "" {
				t.Errorf("FileConfig.%s is restart-required but does not exercise a reason clause", field.Name)
			}
		default:
			t.Errorf("FileConfig.%s has unknown disposition %q", field.Name, row.disposition)
		}
	}
	for name := range fileConfigSectionCensus {
		if !seen[name] {
			t.Errorf("fileConfigSectionCensus contains %q, which is no longer a FileConfig field; update the census with the type", name)
		}
	}
}

func TestEveryRestartRequiredConfigSectionHasAWorkingClause(t *testing.T) {
	for name, row := range fileConfigSectionCensus {
		if row.disposition != configNeedsRestart {
			continue
		}
		t.Run(name, func(t *testing.T) {
			boot := config.DefaultFileConfig()
			// The sessions clause is directional, so establish its disabled boot state.
			if name == "Sessions" {
				disabled := false
				boot.Sessions.Agent.Enabled = &disabled
			}
			next := boot
			row.mutate(&next)

			api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
			api.boot = boot
			if reasons := api.restartReasonsFor(next); !containsString(reasons, row.wantReason) {
				t.Fatalf("restartReasonsFor() = %v, want %q for FileConfig.%s; its frozen setting would otherwise be saved and reported applied", reasons, row.wantReason, name)
			}
		})
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
