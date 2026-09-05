package dashboard

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
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

func TestWhitespaceProfilesBaseDirUsesRuntimeExplicitValueSemantics(t *testing.T) {
	boot := config.DefaultFileConfig()
	boot.Server.StateDir = filepath.Join(t.TempDir(), "state")
	boot.Profiles.BaseDir = "   "
	api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = boot

	next := boot
	next.Profiles.BaseDir = ""
	if reasons := api.restartReasonsFor(next); !containsString(reasons, "Profiles directory") {
		t.Fatalf("restartReasonsFor() = %v, want Profiles directory: runtime treats whitespace as an explicit BaseDir while empty derives from stateDir", reasons)
	}
}

func TestActivityRecorderSnapshotChangesRequireRestart(t *testing.T) {
	mutations := map[string]func(*config.FileConfig){
		"enabled": func(fc *config.FileConfig) {
			value := !*fc.Observability.Activity.Enabled
			fc.Observability.Activity.Enabled = &value
		},
		"retention": func(fc *config.FileConfig) {
			value := *fc.Observability.Activity.RetentionDays + 1
			fc.Observability.Activity.RetentionDays = &value
		},
		"events": func(fc *config.FileConfig) {
			value := !*fc.Observability.Activity.Events.Server
			fc.Observability.Activity.Events.Server = &value
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			boot := config.DefaultFileConfig()
			next := cloneFileConfig(t, boot)
			mutate(&next)
			api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
			api.boot = boot
			if reasons := api.restartReasonsFor(next); !containsString(reasons, "Activity recording") {
				t.Fatalf("restartReasonsFor() = %v, want Activity recording for boot-snapshotted %s", reasons, name)
			}
		})
	}
}

func TestOtherBootSnapshotChangesRequireRestart(t *testing.T) {
	tests := map[string]struct {
		want   string
		mutate func(*config.FileConfig)
	}{
		"log level": {"Log level", func(fc *config.FileConfig) { fc.Server.LogLevel = "debug" }},
		"network recording": {"Network recording", func(fc *config.FileConfig) {
			value := 101
			fc.Server.NetworkBufferSize = &value
		}},
		"default profile": {"Profiles configuration", func(fc *config.FileConfig) { fc.Profiles.DefaultProfile += "-next" }},
		"quarantine policy": {"Profiles configuration", func(fc *config.FileConfig) {
			value := *fc.Profiles.QuarantineKeep + 1
			fc.Profiles.QuarantineKeep = &value
		}},
		"browser":           {"Browser configuration", func(fc *config.FileConfig) { fc.Browser.BrowserBinary += "-next" }},
		"instance defaults": {"Instance defaults", func(fc *config.FileConfig) { fc.InstanceDefaults.Timezone += "-next" }},
		"scheduler": {"Scheduler configuration", func(fc *config.FileConfig) {
			value := 2
			fc.Scheduler.WorkerCount = &value
		}},
		"auto solver": {"Auto-solver configuration", func(fc *config.FileConfig) {
			value := !*fc.AutoSolver.Enabled
			fc.AutoSolver.Enabled = &value
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			boot := config.DefaultFileConfig()
			next := cloneFileConfig(t, boot)
			tc.mutate(&next)
			api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
			api.boot = boot
			if reasons := api.restartReasonsFor(next); !containsString(reasons, tc.want) {
				t.Fatalf("restartReasonsFor() = %v, want %q", reasons, tc.want)
			}
		})
	}
}

// configInertFields is deliberately small. Every other reachable FileConfig leaf
// must prove its classification by mutation: either NextRuntimeConfig changes the
// published value (live), or restartReasonsFor names a frozen consumer (restart).
// Keeping inertness explicit prevents a new field inside an existing section from
// hiding behind a section-level label.
var configInertFields = map[string]string{
	"$schema":                               "JSON Schema editor metadata",
	"configVersion":                         "load-time compatibility metadata",
	"server.token":                          "PUT /api/config preserves the write-only boot credential and rejects token edits",
	"server.engine":                         "retained only so validation can reject the removed setting",
	"observability.activity.stateDir":       config.ActivityStateDirAdvisory,
	"observability.activity.sessionIdleSec": "reserved session-grouping setting; no running consumer currently reads it",
	"browsers.config":                       "retired overrides retained only so validation can reject them with migration guidance",
}

func TestEveryFileConfigSettingHasAnEffectiveDisposition(t *testing.T) {
	boot := config.DefaultFileConfig()
	seed := config.Load()
	runtimeFor := func(path string, fc config.FileConfig) *config.RuntimeConfig {
		isolated := cloneFileConfig(t, fc)
		prepareConfigCensusFixture(path, &isolated)
		if path == "instanceDefaults.headless" {
			isolated.InstanceDefaults.Headless = fc.InstanceDefaults.Headless
		}
		return config.NextRuntimeConfig(seed, &isolated)
	}
	seen := make(map[string]bool)
	forEachConfigLeaf(t, reflect.ValueOf(&boot).Elem(), "", func(path string, indexes []int) {
		seen[path] = true
		current := cloneFileConfig(t, boot)
		prepareConfigCensusFixture(path, &current)
		next := cloneFileConfig(t, current)
		mutateConfigLeaf(t, reflect.ValueOf(&next).Elem(), indexes)
		applyConfigCensusMutation(path, &next)

		base := runtimeFor(path, current)
		api := newConfigAPIForTest(base, nil, nil, nil, nil, "test", time.Now())
		api.boot = current
		restartReasons := api.restartReasonsFor(next)
		live := !reflect.DeepEqual(runtimeFor(path, current), runtimeFor(path, next))
		inertReason, inert := configInertFields[path]
		wantRestart := configRestartReason(path)
		liveEvidence, wantLive := configLiveEvidence(path)
		switch {
		case wantRestart != "":
			if !containsString(restartReasons, wantRestart) {
				t.Errorf("%s is frozen but restartReasonsFor returned %v, want %q; removing a restart clause must red this census", path, restartReasons, wantRestart)
			}
		case inert:
			if len(restartReasons) > 0 {
				t.Errorf("%s is marked inert but also produces restart reasons %v", path, restartReasons)
			}
		case wantLive:
			if liveEvidence == "" || !live {
				t.Errorf("%s is declared live (%s) but its mutation did not change the published RuntimeConfig", path, liveEvidence)
			}
		default:
			t.Errorf("%s is absent from the explicit consumer-semantics registry; classify it in configRestartReason, configLiveEvidence, or configInertFields", path)
		}
		if inert && inertReason == "" {
			t.Errorf("%s is marked inert without recording why", path)
		}
	})
	for path := range configInertFields {
		if !seen[path] {
			t.Errorf("configInertFields contains %q, which is no longer a FileConfig leaf; update the census with the type", path)
		}
	}
}

func configLiveEvidence(path string) (string, bool) {
	switch {
	case path == "server.trustProxyHeaders" || path == "server.trustedProxyHops" || path == "server.cookieSecure":
		return "front-door middleware resolves config.Live per request", true
	case path == "multiInstance.allocationPolicy" || path == "multiInstance.instancePortStart" || path == "multiInstance.instancePortEnd":
		return "Orchestrator.ApplyRuntimeConfig swaps allocator state", true
	case strings.HasPrefix(path, "timeouts."):
		return "request handlers resolve effective runtime timeouts", true
	case strings.HasPrefix(path, "sessions.dashboard."):
		return "browsersession.Manager.UpdateConfig applies dashboard session settings", true
	case strings.HasPrefix(path, "sessions.agent.") && path != "sessions.agent.enabled":
		return "session.Store.UpdateConfig applies agent session settings", true
	default:
		return "", false
	}
}

func configRestartReason(path string) string {
	switch {
	case strings.HasPrefix(path, "security."):
		return "Security policy"
	case path == "server.port" || path == "server.bind":
		return "Server address"
	case path == "server.stateDir":
		return "Server state directory (server.stateDir)"
	case path == "server.logLevel":
		return "Log level"
	case path == "server.networkBufferSize" || path == "server.retainNetworkBodies" || path == "server.retainNetworkBodyMaxBytes":
		return "Network recording"
	case path == "profiles.baseDir":
		return "Profiles directory"
	case path == "profiles.defaultProfile" || path == "profiles.quarantineKeep":
		return "Profiles configuration"
	case path == "multiInstance.strategy":
		return "Routing strategy"
	case strings.HasPrefix(path, "multiInstance.restart."):
		return "Restart policy"
	case path == "instanceDefaults.stealthLevel":
		return "Stealth level"
	case strings.HasPrefix(path, "instanceDefaults."):
		return "Instance defaults"
	case strings.HasPrefix(path, "browser.") || path == "browsers.default" || path == "browsers.available":
		return "Browser configuration"
	case strings.HasPrefix(path, "scheduler."):
		return "Scheduler configuration"
	case strings.HasPrefix(path, "autoSolver."):
		return "Auto-solver configuration"
	case path == "sessions.agent.enabled":
		return "Agent sessions"
	case path == "observability.activity.enabled" ||
		path == "observability.activity.retentionDays" ||
		strings.HasPrefix(path, "observability.activity.events."):
		return "Activity recording"
	default:
		return ""
	}
}

func prepareConfigCensusFixture(path string, fc *config.FileConfig) {
	switch path {
	case "server.trustedProxyHops":
		if fc.Server.TrustedProxyHops == nil {
			hops := config.DefaultTrustedProxyHops
			fc.Server.TrustedProxyHops = &hops
		}
	case "instanceDefaults.headless":
		fc.InstanceDefaults.Mode = ""
		value := false
		fc.InstanceDefaults.Headless = &value
	case "sessions.agent.enabled":
		value := false
		fc.Sessions.Agent.Enabled = &value
	}
}

func applyConfigCensusMutation(path string, fc *config.FileConfig) {
	switch path {
	case "instanceDefaults.mode":
		fc.InstanceDefaults.Mode = "headed"
	case "instanceDefaults.headless":
		fc.InstanceDefaults.Mode = ""
	}
}

func forEachConfigLeaf(t *testing.T, value reflect.Value, prefix string, visit func(string, []int)) {
	t.Helper()
	var walk func(reflect.Type, string, []int)
	walk = func(typ reflect.Type, path string, indexes []int) {
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ == reflect.TypeOf(time.Duration(0)) {
			visit(path, indexes)
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			name := jsonFieldName(field)
			if name == "-" {
				continue
			}
			nextPath := name
			if path != "" {
				nextPath = path + "." + name
			}
			walk(field.Type, nextPath, append(append([]int(nil), indexes...), i))
		}
	}
	walk(value.Type(), prefix, nil)
}

func cloneFileConfig(t *testing.T, source config.FileConfig) config.FileConfig {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var clone config.FileConfig
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func mutateConfigLeaf(t *testing.T, root reflect.Value, indexes []int) {
	t.Helper()
	value := root
	for _, index := range indexes {
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}
		value = value.Field(index)
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Bool:
		value.SetBool(!value.Bool())
	case reflect.String:
		value.SetString(value.String() + "census")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(value.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value.SetUint(value.Uint() + 1)
	case reflect.Slice:
		elem := reflect.New(value.Type().Elem()).Elem()
		if elem.Kind() == reflect.String {
			elem.SetString("census")
		}
		value.Set(reflect.Append(value, elem))
	case reflect.Map:
		if value.IsNil() {
			value.Set(reflect.MakeMap(value.Type()))
		}
		key := reflect.New(value.Type().Key()).Elem()
		if key.Kind() == reflect.String {
			key.SetString("census")
		}
		value.SetMapIndex(key, reflect.New(value.Type().Elem()).Elem())
	default:
		t.Fatalf("no census mutation for %s", value.Type())
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

func TestStealthLevelEditReportsOnlyStealthLevel(t *testing.T) {
	boot := config.DefaultFileConfig()
	next := cloneFileConfig(t, boot)
	next.InstanceDefaults.StealthLevel = "full"
	api := newConfigAPIForTest(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = boot
	reasons := api.restartReasonsFor(next)
	if !containsString(reasons, "Stealth level") || containsString(reasons, "Instance defaults") {
		t.Fatalf("restartReasonsFor() = %v, want Stealth level alone", reasons)
	}
}
