package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// This file guards the single-ownership of the autoSolver defaults. They used to be
// written out three times — RuntimeConfig, DefaultFileConfig and the core's
// DefaultConfig — with no test comparing them, and they had already drifted on
// Enabled. Do not "simplify" the census below into comparing two structs that happen
// to be the same type: the three copies use different types and different units, and
// the point is that every field is reached, so a newly added autoSolver default cannot
// land unguarded.

// autoSolverDefaultRow declares one field's default once, as data, and how to read it
// out of each of the three representations. want is the recorded baseline, so editing
// the shared owner reddens every row rather than passing silently.
type autoSolverDefaultRow struct {
	field   string
	want    any
	runtime func(AutoSolverConfig) any
	file    func(AutoSolverFileConfig) any
	// core reads the same value out of autosolver.Config, converting units. nil for
	// the fields the core has no counterpart for.
	core func(autosolver.Config) any
}

var autoSolverDefaultRows = []autoSolverDefaultRow{
	{
		field:   "Enabled",
		want:    false,
		runtime: func(c AutoSolverConfig) any { return c.Enabled },
		file:    func(c AutoSolverFileConfig) any { return derefBool(c.Enabled) },
		core:    func(c autosolver.Config) any { return c.Enabled },
	},
	{
		field:   "AutoTrigger",
		want:    true,
		runtime: func(c AutoSolverConfig) any { return c.AutoTrigger },
		file:    func(c AutoSolverFileConfig) any { return derefBool(c.AutoTrigger) },
	},
	{
		field:   "TriggerOnNavigate",
		want:    true,
		runtime: func(c AutoSolverConfig) any { return c.TriggerOnNavigate },
		file:    func(c AutoSolverFileConfig) any { return derefBool(c.TriggerOnNavigate) },
	},
	{
		field:   "TriggerOnAction",
		want:    true,
		runtime: func(c AutoSolverConfig) any { return c.TriggerOnAction },
		file:    func(c AutoSolverFileConfig) any { return derefBool(c.TriggerOnAction) },
	},
	{
		field:   "MaxAttempts",
		want:    8,
		runtime: func(c AutoSolverConfig) any { return c.MaxAttempts },
		file:    func(c AutoSolverFileConfig) any { return derefInt(c.MaxAttempts) },
		core:    func(c autosolver.Config) any { return c.MaxAttempts },
	},
	{
		field:   "SolverTimeoutSec",
		want:    30,
		runtime: func(c AutoSolverConfig) any { return c.SolverTimeoutSec },
		file:    func(c AutoSolverFileConfig) any { return derefInt(c.SolverTimeoutSec) },
		core:    func(c autosolver.Config) any { return int(c.SolverTimeout / time.Second) },
	},
	{
		field:   "RetryBaseDelayMs",
		want:    500,
		runtime: func(c AutoSolverConfig) any { return c.RetryBaseDelayMs },
		file:    func(c AutoSolverFileConfig) any { return derefInt(c.RetryBaseDelayMs) },
		core:    func(c autosolver.Config) any { return int(c.RetryBaseDelay / time.Millisecond) },
	},
	{
		field:   "RetryMaxDelayMs",
		want:    10000,
		runtime: func(c AutoSolverConfig) any { return c.RetryMaxDelayMs },
		file:    func(c AutoSolverFileConfig) any { return derefInt(c.RetryMaxDelayMs) },
		core:    func(c autosolver.Config) any { return int(c.RetryMaxDelay / time.Millisecond) },
	},
	{
		field:   "Solvers",
		want:    []string{autosolver.CloudflareSolverName, autosolver.SemanticSolverName},
		runtime: func(c AutoSolverConfig) any { return c.Solvers },
		file:    func(c AutoSolverFileConfig) any { return c.Solvers },
		core:    func(c autosolver.Config) any { return c.Solvers },
	},
	{
		field:   "LLMFallback",
		want:    false,
		runtime: func(c AutoSolverConfig) any { return c.LLMFallback },
		file:    func(c AutoSolverFileConfig) any { return derefBool(c.LLMFallback) },
		core:    func(c autosolver.Config) any { return c.LLMFallback },
	},
}

// autoSolverFieldsWithoutDefault are the AutoSolverConfig fields the defaults
// deliberately leave zero, each with the reason. They are listed rather than skipped
// so the census reaches every field.
var autoSolverFieldsWithoutDefault = map[string]string{
	"LLMProvider":   "no provider is chosen for the operator; an empty value means the LLM fallback stays unconfigured",
	"CapsolverKey":  "an external solver API key; a default would be a fabricated credential",
	"TwoCaptchaKey": "an external solver API key; a default would be a fabricated credential",
	"Credentials":   "user-supplied login/signup/form values; never defaulted",
}

func derefBool(p *bool) any {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func derefInt(p *int) any {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// runtimeAutoSolverDefaults loads the config with a file that sets nothing in the
// autoSolver section, so the values read back are the ones LoadConfig assigns rather
// than a re-derivation of them in the test.
func runtimeAutoSolverDefaults(t *testing.T) AutoSolverConfig {
	t.Helper()
	clearConfigEnvVars(t)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"server":{"port":"9867"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)
	return Load().AutoSolver
}

func TestAutoSolverDefaultsAgreeAcrossEveryCopy(t *testing.T) {
	runtime := runtimeAutoSolverDefaults(t)
	file := DefaultFileConfig().AutoSolver
	core := autosolver.DefaultConfig()

	for _, row := range autoSolverDefaultRows {
		t.Run(row.field, func(t *testing.T) {
			if got := row.runtime(runtime); !reflect.DeepEqual(got, row.want) {
				t.Errorf("RuntimeConfig default = %v, want %v", got, row.want)
			}
			if got := row.file(file); !reflect.DeepEqual(got, row.want) {
				t.Errorf("DefaultFileConfig default = %v, want %v", got, row.want)
			}
			if row.core == nil {
				return
			}
			if got := row.core(core); !reflect.DeepEqual(got, row.want) {
				t.Errorf("autosolver.DefaultConfig default = %v, want %v", got, row.want)
			}
		})
	}
}

// The census: every AutoSolverConfig field is either compared above or listed as
// deliberately undefaulted. A field in neither table fails by name, which is what
// stops a newly added default from being silently unguarded.
func TestEveryAutoSolverFieldIsAccountedFor(t *testing.T) {
	compared := make(map[string]bool, len(autoSolverDefaultRows))
	for _, row := range autoSolverDefaultRows {
		compared[row.field] = true
	}

	runtime := runtimeAutoSolverDefaults(t)
	typ := reflect.TypeFor[AutoSolverConfig]()
	value := reflect.ValueOf(runtime)
	if typ.NumField() == 0 {
		t.Fatal("AutoSolverConfig has no fields — this census would pass vacuously")
	}

	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		switch {
		case compared[name]:
			delete(compared, name)
		case autoSolverFieldsWithoutDefault[name] != "":
			if !value.Field(i).IsZero() {
				t.Errorf("%s is listed as having no default (%s) but the loaded default is %v",
					name, autoSolverFieldsWithoutDefault[name], value.Field(i).Interface())
			}
		default:
			t.Errorf("AutoSolverConfig.%s is in neither autoSolverDefaultRows nor autoSolverFieldsWithoutDefault; add it to one", name)
		}
	}

	for name := range compared {
		t.Errorf("autoSolverDefaultRows names %q, which is not a field of AutoSolverConfig", name)
	}
}

// The core's Enabled default used to be true while both config copies said false.
// normalizedAutoSolverConfig overwrites Enabled unconditionally, so the divergence was
// observable only on its h.Config == nil early return — the one configuration-free
// path, which would have silently enabled solving while every configured path left it
// off. Aligned to false; this records the decision so it is not "restored" later.
func TestCoreEnabledDefaultMatchesTheConfigDefault(t *testing.T) {
	if autosolver.DefaultConfig().Enabled {
		t.Error("autosolver.DefaultConfig().Enabled is true again; the only path that observes it is the config-free one, where enabling solving contradicts every configured path")
	}
}

// No solver name may be spelled as a literal in internal/config: internal/autosolver
// owns that vocabulary, and this package's default list is where two of the three
// copies used to restate it.
func TestNoSolverNameLiteralsInThisPackage(t *testing.T) {
	var offenders []string
	for name, body := range srccensus.Load(t, ".", 20).Sources() {
		for _, literal := range []string{`"` + autosolver.CloudflareSolverName + `"`, `"` + autosolver.SemanticSolverName + `"`} {
			if strings.Contains(body, literal) {
				offenders = append(offenders, name+" contains "+literal)
			}
		}
	}
	if len(offenders) > 0 {
		slices.Sort(offenders)
		t.Errorf("solver names must come from internal/autosolver's constants: %v", offenders)
	}
}

func TestLoadedDefaultsProjectOntoTheStarterFileConfig(t *testing.T) {
	clearConfigEnvVars(t)
	t.Setenv("PINCHTAB_TOKEN", "")
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig with no file: %v", err)
	}

	loaded := reflect.ValueOf(starterFileConfig(FileConfigFromRuntime(cfg)))
	starter := reflect.ValueOf(DefaultFileConfig())
	for i := 0; i < loaded.NumField(); i++ {
		section := loaded.Type().Field(i).Name
		if got, want := loaded.Field(i).Interface(), starter.Field(i).Interface(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: loaded defaults project to %+v, DefaultFileConfig has %+v", section, got, want)
		}
	}
}
