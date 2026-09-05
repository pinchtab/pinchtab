package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// The guards-down preset used to hand-list its capability toggles and had silently drifted
// from the canonical table — omitting stateExport. It now derives from routes, so this census
// walks CapabilityEndpoints() and drives the real preset: every gated capability must be
// enabled EXCEPT the one recorded exclusion, and each must resolve through routes.Meta rather
// than a synthesised path. A capability that gates routes but routes.Meta cannot describe
// (an endpoint added without a table entry) fails here rather than being silently skipped.
func TestGuardsDownEnablesEveryCapabilityExceptTheRecordedExclusion(t *testing.T) {
	gated := routes.CapabilityEndpoints()
	if len(gated) < 2 {
		t.Fatalf("only %d gated capabilities; this census would pass vacuously", len(gated))
	}

	fc := config.DefaultFileConfig()
	if err := BuildGuardsDownConfig(&fc); err != nil {
		t.Fatalf("BuildGuardsDownConfig() error = %v", err)
	}

	excludedSeen := false
	for cap := range gated {
		meta, ok := routes.Meta(cap)
		if !ok {
			t.Errorf("capability %q gates routes but routes.Meta does not describe it, so guards-down cannot derive its setting", cap)
			continue
		}
		got, err := config.GetConfigValue(&fc, meta.Setting)
		if err != nil {
			t.Errorf("reading %s for capability %q: %v", meta.Setting, cap, err)
			continue
		}
		if cap == guardsDownExcludedCapability {
			excludedSeen = true
			if got == "true" {
				t.Errorf("guards-down enabled the recorded-exclusion capability %q (%s); disk state export must stay a deliberate opt-out, not a side effect of turning guards off", cap, meta.Setting)
			}
			continue
		}
		if got != "true" {
			t.Errorf("guards-down left capability %q (%s) at %q, not enabled; a capability it omits is silently kept off", cap, meta.Setting, got)
		}
	}
	if !excludedSeen {
		t.Fatalf("the recorded exclusion %q is not among the gated capabilities; the exclusion is stale", guardsDownExcludedCapability)
	}
}

func TestApplyGuardsDownPreset(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = "guarded-token"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	result, err := ApplyGuardsDownPreset(false)
	if err != nil {
		t.Fatalf("ApplyGuardsDownPreset() error = %v", err)
	}
	if !result.Written {
		t.Fatal("expected guards down preset to change config")
	}
	if result.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", result.ConfigPath, configPath)
	}
	cfg := config.Load()

	if cfg.Bind != "127.0.0.1" {
		t.Fatalf("Bind = %q, want 127.0.0.1", cfg.Bind)
	}
	if cfg.Token != "guarded-token" {
		t.Fatalf("Token = %q, want existing token to remain", cfg.Token)
	}
	if !cfg.AllowEvaluate || !cfg.AllowMacro || !cfg.AllowScreencast || !cfg.AllowDownload || !cfg.AllowCookies || !cfg.AllowUpload {
		t.Fatalf("expected sensitive endpoints enabled, got %+v", cfg)
	}
	if !cfg.AttachEnabled {
		t.Fatal("expected attach endpoint enabled")
	}
	if got := strings.Join(cfg.AttachAllowHosts, ","); got != "127.0.0.1,localhost,::1" {
		t.Fatalf("AttachAllowHosts = %q", got)
	}
	if got := strings.Join(cfg.AttachAllowSchemes, ","); got != "ws,wss" {
		t.Fatalf("AttachAllowSchemes = %q", got)
	}
	if cfg.IDPI.Enabled || cfg.IDPI.StrictMode || cfg.IDPI.ScanContent || cfg.IDPI.WrapContent {
		t.Fatalf("expected IDPI protections disabled, got %+v", cfg.IDPI)
	}
}

func TestGuardsDownPostureActiveMirrorsThePreset(t *testing.T) {
	fc := config.DefaultFileConfig()
	if err := BuildGuardsDownConfig(&fc); err != nil {
		t.Fatalf("BuildGuardsDownConfig() error = %v", err)
	}
	if !GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), &fc)) {
		t.Fatal("GuardsDownPostureActive() = false right after applying the preset")
	}
	if GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), ptr(config.DefaultFileConfig()))) {
		t.Fatal("GuardsDownPostureActive() = true for the default config")
	}
	for cap := range routes.CapabilityEndpoints() {
		if cap == guardsDownExcludedCapability {
			continue
		}
		meta, _ := routes.Meta(cap)
		relaxed := fc
		if err := config.SetConfigValue(&relaxed, meta.Setting, "false"); err != nil {
			t.Fatalf("set %s: %v", meta.Setting, err)
		}
		if GuardsDownPostureActive(config.NextRuntimeConfig(config.Load(), &relaxed)) {
			t.Errorf("GuardsDownPostureActive() = true with capability %q disabled; the detector must track every preset capability", cap)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// securityConfigPaths walks SecurityConfig by its json tags so a field added later
// is classified by this census before it can be wiped: every path is either one
// the preset names, or one the preset must leave untouched.
func securityConfigPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	var walk func(prefix string, typ reflect.Type)
	walk = func(prefix string, typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			path := prefix + "." + name
			ft := field.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				walk(path, ft)
				continue
			}
			paths = append(paths, path)
		}
	}
	walk("security", reflect.TypeOf(config.SecurityConfig{}))
	if len(paths) < 20 {
		t.Fatalf("walked only %d security paths; the census would prove little", len(paths))
	}
	return paths
}

// sentinelSecurityConfig sets every leaf of SecurityConfig to a value no default
// carries, so "preserved" and "reset" are both observable for every field.
func sentinelSecurityConfig() config.SecurityConfig {
	var fill func(v reflect.Value)
	fill = func(v reflect.Value) {
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Ptr:
				elem := reflect.New(f.Type().Elem())
				if elem.Elem().Kind() == reflect.Struct {
					fill(elem.Elem())
				} else {
					setSentinel(elem.Elem())
				}
				f.Set(elem)
			case reflect.Struct:
				fill(f)
			default:
				setSentinel(f)
			}
		}
	}
	var sec config.SecurityConfig
	fill(reflect.ValueOf(&sec).Elem())
	return sec
}

func setSentinel(v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int:
		v.SetInt(7777)
	case reflect.String:
		v.SetString("sentinel-value")
	case reflect.Slice:
		v.Set(reflect.ValueOf([]string{"sentinel.example", "10.9.8.0/24"}))
	default:
		panic("unhandled security field kind " + v.Kind().String())
	}
}

func TestSecurityUpNamesEveryFieldItResetsAndPreservesTheRest(t *testing.T) {
	named := map[string]string{}
	settings, err := recommendedSecuritySettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range settings {
		named[s.path] = s.value
	}
	if _, ok := named["security.allowEvaluate"]; !ok {
		t.Fatal("the capability half is not derived: allowEvaluate is missing from the named resets")
	}

	fc := config.DefaultFileConfig()
	fc.Security = sentinelSecurityConfig()
	before := map[string]string{}
	for _, path := range securityConfigPaths(t) {
		v, err := config.GetConfigValue(&fc, path)
		if err != nil {
			t.Fatalf("read %s before: %v", path, err)
		}
		before[path] = v
	}
	if err := ApplyRecommendedSecurityDefaults(&fc); err != nil {
		t.Fatal(err)
	}
	for path, was := range before {
		got, err := config.GetConfigValue(&fc, path)
		if err != nil {
			t.Fatalf("read %s after: %v", path, err)
		}
		if want, reset := named[path]; reset {
			if got != want {
				t.Errorf("%s is a named reset but reads %q, want %q", path, got, want)
			}
			continue
		}
		if got != was {
			t.Errorf("%s is not named by security up yet changed %q -> %q; it must be named or preserved", path, was, got)
		}
	}
}

// The nine keys the wholesale assignment used to destroy, proven on the file the
// operator keeps: the change report names only recommended settings, and each key
// reads back unchanged afterwards.
func TestSecurityUpPreservesOperatorSecurityData(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(`{
  "server": {"bind": "0.0.0.0", "token": "secret"},
  "security": {
    "allowEvaluate": true,
    "allowFileScheme": true,
    "allowedDomains": ["intranet.example"],
    "downloadAllowedDomains": ["files.example"],
    "trustedProxyCIDRs": ["10.0.0.0/8"],
    "trustedResolveCIDRs": ["192.168.0.0/16"],
    "stateEncryptionKey": "operator-secret-key",
    "attach": {"enabled": true, "allowHosts": ["chrome.internal"], "allowSchemes": ["ws"]},
    "idpi": {"enabled": false, "customPatterns": ["ignore previous"], "shieldThreshold": 42}
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved := map[string]string{
		"security.allowFileScheme":        "true",
		"security.allowedDomains":         "intranet.example",
		"security.downloadAllowedDomains": "files.example",
		"security.trustedProxyCIDRs":      "10.0.0.0/8",
		"security.trustedResolveCIDRs":    "192.168.0.0/16",
		"security.stateEncryptionKey":     "operator-secret-key",
		"security.attach.allowSchemes":    "ws",
		"security.idpi.customPatterns":    "ignore previous",
		"security.idpi.shieldThreshold":   "42",
	}

	result, err := RestoreSecurityDefaults(false)
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	settings, _ := recommendedSecuritySettings()
	for _, s := range settings {
		named[s.path] = true
	}
	moved := 0
	for _, change := range result.Changes {
		if !named[change.Path] {
			t.Errorf("security up wrote %s (%s -> %s), which it never named", change.Path, change.Old, change.New)
		}
		moved++
	}
	if moved < 4 {
		t.Fatalf("only %d recommended settings moved; the relaxed fixture should move more", moved)
	}
	loaded, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range preserved {
		got, err := config.GetConfigValue(loaded, path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if got != want {
			t.Errorf("%s = %q after security up, want the operator's %q", path, got, want)
		}
	}
	if hosts, _ := config.GetConfigValue(loaded, "security.attach.allowHosts"); strings.Contains(hosts, "chrome.internal") {
		t.Errorf("attach.allowHosts kept a non-local host: %q; that reset is deliberate and named", hosts)
	}
}

func writePresetFixture(t *testing.T, body string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

const invalidPreservedValue = "not-a-cidr/99"

func TestPresetsRefuseWhatTheyBreakAndReportWhatTheyFind(t *testing.T) {
	valid := `{"server": {"token": "secret"}}`
	invalidPreserved := `{"server": {"token": "secret"}, "security": {"trustedProxyCIDRs": ["` + invalidPreservedValue + `"]}}`
	presets := map[string]func(bool) (PresetResult, error){
		"security up":   RestoreSecurityDefaults,
		"security down": ApplyGuardsDownPreset,
	}
	for name, preset := range presets {
		t.Run(name+" over a valid config", func(t *testing.T) {
			writePresetFixture(t, valid)
			result, err := preset(false)
			if err != nil || !result.Written || len(result.PreExisting) != 0 {
				t.Fatalf("result %+v err %v; a valid config must be written with nothing reported", result, err)
			}
		})
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s over an invalid preserved value dryRun=%t", name, dryRun), func(t *testing.T) {
				configPath := writePresetFixture(t, invalidPreserved)
				before, _ := os.ReadFile(configPath)
				result, err := preset(dryRun)
				if err != nil {
					t.Fatalf("a pre-existing error in a preserved key must not block the preset: %v", err)
				}
				if result.Written == dryRun || !result.Changed() {
					t.Fatalf("written=%t changed=%t under dryRun=%t", result.Written, result.Changed(), dryRun)
				}
				if len(result.PreExisting) != 1 || !strings.Contains(result.PreExisting[0].Error(), "security.trustedProxyCIDRs") {
					t.Fatalf("pre-existing errors %v; the operator must learn the file was already invalid", result.PreExisting)
				}
				after, _ := os.ReadFile(configPath)
				if !strings.Contains(string(after), invalidPreservedValue) {
					t.Fatalf("the invalid preserved value was destroyed: %s", after)
				}
				if dryRun && string(after) != string(before) {
					t.Fatalf("dry run wrote the file")
				}
			})
		}
	}

	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("a preset writing an invalid value refuses dryRun=%t", dryRun), func(t *testing.T) {
			configPath := writePresetFixture(t, valid)
			before, _ := os.ReadFile(configPath)
			_, err := runPreset(dryRun, func(fc *config.FileConfig, _ string) error {
				return config.SetConfigValue(fc, "server.port", "not-a-port")
			})
			if err == nil || !strings.Contains(err.Error(), "server.port") || !strings.Contains(err.Error(), "nothing was written") {
				t.Fatalf("err = %v; a value the preset writes that fails validation must refuse by name", err)
			}
			after, _ := os.ReadFile(configPath)
			if string(after) != string(before) {
				t.Fatalf("the refused preset changed the file")
			}
		})
	}

	t.Run("an introduced error is refused even beside a pre-existing one", func(t *testing.T) {
		writePresetFixture(t, invalidPreserved)
		_, err := runPreset(false, func(fc *config.FileConfig, _ string) error {
			return config.SetConfigValue(fc, "server.port", "not-a-port")
		})
		if err == nil || !strings.Contains(err.Error(), "server.port") || strings.Contains(err.Error(), "trustedProxyCIDRs") {
			t.Fatalf("err = %v; only the error the preset introduced refuses, the pre-existing one is reported", err)
		}
	})
}
