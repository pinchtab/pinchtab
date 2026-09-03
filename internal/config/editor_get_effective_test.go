package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/safelog"
)

func writeConfigForGet(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	t.Setenv("PINCHTAB_TOKEN", "")
	return path
}

func effectiveValue(t *testing.T, path string) string {
	t.Helper()

	value, err := EffectiveConfigValue(path)
	if err != nil {
		t.Fatalf("EffectiveConfigValue(%q) error = %v", path, err)
	}
	return value
}

func TestEffectiveConfigValueDerivesProfilesBaseDirFromTheConfiguredStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "mystate")
	writeConfigForGet(t, `{"server":{"stateDir":`+quoteJSON(stateDir)+`}}`)

	want := filepath.Join(stateDir, "profiles")
	if got := effectiveValue(t, "profiles.baseDir"); got != want {
		t.Errorf("profiles.baseDir = %q, want %q derived from the configured stateDir", got, want)
	}
	if got := effectiveValue(t, "profiles.baseDir"); strings.HasPrefix(got, userConfigDir()) {
		t.Errorf("profiles.baseDir = %q, which sits under the default state dir rather than the configured one", got)
	}
}

func TestEffectiveConfigValueReportsTheDocumentedLogLevelDefault(t *testing.T) {
	writeConfigForGet(t, `{}`)

	want := safelog.LevelName(safelog.DefaultLevel)
	if got := effectiveValue(t, "server.logLevel"); got != want {
		t.Errorf("server.logLevel = %q, want the default the runtime records at, %q", got, want)
	}
}

func TestEffectiveConfigValueKeepsWhatTheFileSets(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "mystate")
	writeConfigForGet(t, `{
		"server": {"stateDir": `+quoteJSON(stateDir)+`, "logLevel": "warn", "trustProxyHeaders": true},
		"profiles": {"baseDir": "/tmp/explicit-profiles"}
	}`)

	for _, tc := range []struct{ path, want string }{
		{"server.logLevel", "warn"},
		{"server.trustProxyHeaders", "true"},
		{"profiles.baseDir", "/tmp/explicit-profiles"},
		{"server.stateDir", stateDir},
	} {
		if got := effectiveValue(t, tc.path); got != tc.want {
			t.Errorf("%s = %q, want the file value %q", tc.path, got, tc.want)
		}
	}
}

// blankIsTheAnswer holds the keys whose blank answer is correct: an unset optional,
// an override with no default, or a secret nobody configured.
var blankIsTheAnswer = map[string]string{
	"server.token":                            "no token configured and none in PINCHTAB_TOKEN",
	"browser.binary":                          "unset means discover the browser instead of using a fixed path",
	"browser.extraFlags":                      "no extra flags",
	"browser.defaultTarget":                   "unset means the default provider, not a named target",
	"browser.cloak.fingerprintSeed":           "cloak override, unset",
	"browser.cloak.platform":                  "cloak override, unset",
	"browser.cloak.locale":                    "cloak override, unset",
	"browser.cloak.timezone":                  "cloak override, unset",
	"browser.cloak.webrtcIP":                  "cloak override, unset",
	"browser.cloak.fontsDir":                  "cloak override, unset",
	"browser.cloak.storageQuotaMB":            "cloak override, unset",
	"browser.cloak.disableDefaultStealthArgs": "cloak override, unset",
	"browser.remoteDebuggingPort":             "unset means a debugging port is chosen when the browser launches, so there is no configured value to report",
	"security.stateEncryptionKey":             "no state encryption key configured, and the CLI masks this leaf when there is one",
	"browser.proxy.server":                    "no proxy configured",
	"browser.proxy.bypassList":                "no proxy configured",
	"browser.proxy.username":                  "no proxy configured",
	"browser.proxy.password":                  "no proxy configured",
	"browser.proxy.geo.timezone":              "no proxy geo override",
	"browser.proxy.geo.locale":                "no proxy geo override",
	"browser.proxy.geo.webrtcIP":              "no proxy geo override",
	"browser.proxy.geo.countryISO":            "no proxy geo override",
	"browser.fallbackOrder":                   "no fallback configured; an implicit launch then tries the primary target only",
	"instanceDefaults.timezone":               "unset means the host timezone",
	"instanceDefaults.userAgent":              "unset means the browser's own user agent",
	"security.allowedDomains":                 "empty list; an absent allowlist is not a restriction, so every host is permitted. A localhost-only default used to be printed here, and honouring it would have refused every external navigation",
	"security.downloadAllowedDomains":         "empty list, downloads follow the main allowlist",
	"security.trustedProxyCIDRs":              "empty list",
	"security.trustedResolveCIDRs":            "empty list",
	"security.idpi.customPatterns":            "empty list",
	"autoSolver.llmProvider":                  "no LLM provider configured",
	"autoSolver.external.capsolverKey":        "no external solver key configured",
	"autoSolver.external.twoCaptchaKey":       "no external solver key configured",
	"autoSolver.credentials.login.user":       "no stored credential",
	"autoSolver.credentials.login.password":   "no stored credential",
	"autoSolver.credentials.signup.name":      "no stored credential",
	"autoSolver.credentials.signup.email":     "no stored credential",
	"autoSolver.credentials.signup.password":  "no stored credential",
	"autoSolver.credentials.form.field1":      "no stored credential",
	"autoSolver.credentials.form.field2":      "no stored credential",
	"autoSolver.credentials.form.email":       "no stored credential",
}

// defaultLivesAtItsConsumer holds the keys that still answer blank although a real
// default applies. Each names where that default is written; reporting them needs
// that owner to expose it, which is why they are recorded here rather than spelled
// out a second time in this package. A key must move out of this table, not gain a
// hardcoded value in the print path.
var defaultLivesAtItsConsumer = map[string]string{
	"server.cookieSecure": "there is no single value: the cookie layer decides per request from that request's scheme, so any printed answer would be wrong for half the requests",

	"server.networkBufferSize": "the per-tab buffer default is DefaultNetworkBufferSize in the network buffer itself, applied when the configured size is zero; the config layer never settles it, so reporting a number here would be this package's own guess",

	"browser.version": "left unset by default so the persona probes the launched binary for its real version (stealth.ResolveBrowserVersion); until the operator pins browser.version explicitly there is no single settled value at the config layer, and printing the fallback literal here would advertise a version the running browser may not be",
}

// TestEveryAddressableConfigKeyAnswersOrIsAccountedFor is the standing census behind
// this behaviour: a key whose effective value is derived, or defaulted somewhere other
// than the config struct, answers blank and nobody notices. Adding such a key now
// fails here until it either answers or is recorded with a reason.
func TestEveryAddressableConfigKeyAnswersOrIsAccountedFor(t *testing.T) {
	writeConfigForGet(t, `{}`)

	paths := addressableConfigPaths(t)
	if len(paths) < 100 {
		t.Fatalf("found only %d addressable config paths, so this census is not walking FileConfig", len(paths))
	}

	blank := make(map[string]bool, len(paths))
	for _, path := range paths {
		value, err := EffectiveConfigValue(path)
		if err != nil {
			t.Fatalf("EffectiveConfigValue(%q) error = %v", path, err)
		}
		if strings.TrimSpace(value) != "" {
			continue
		}
		blank[path] = true
		if _, ok := blankIsTheAnswer[path]; ok {
			continue
		}
		if reason, ok := defaultLivesAtItsConsumer[path]; ok {
			t.Logf("known hole: %s answers blank because %s", path, reason)
			continue
		}
		t.Errorf("config get %s answers with a blank line; either report the value in effect or record why blank is the answer", path)
	}

	for _, table := range []map[string]string{blankIsTheAnswer, defaultLivesAtItsConsumer} {
		for path := range table {
			if !slicesContains(paths, path) {
				t.Errorf("%s is recorded as blank-answering but is no longer an addressable key; drop the entry", path)
				continue
			}
			if !blank[path] {
				t.Errorf("%s now answers with a value; drop its entry so the census keeps meaning what it says", path)
			}
		}
	}
}

func addressableConfigPaths(t *testing.T) []string {
	t.Helper()

	fc := DefaultFileConfig()
	var out []string
	for _, path := range configLeafPaths(reflect.TypeOf(FileConfig{}), "") {
		if _, err := GetConfigValue(&fc, path); err != nil {
			continue
		}
		out = append(out, path)
	}
	return out
}

func configLeafPaths(typ reflect.Type, prefix string) []string {
	var out []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		path := tag
		if prefix != "" {
			path = prefix + "." + tag
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct {
			out = append(out, configLeafPaths(fieldType, path)...)
			continue
		}
		out = append(out, path)
	}
	return out
}

func slicesContains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

// TestTheProfilesPathRuleHasOneOwner keeps the effective-value reader honest about
// where it gets its answer: joining the state dir with "profiles" a second time would
// make config get agree with the runtime today and drift from it silently later.
func TestTheProfilesPathRuleHasOneOwner(t *testing.T) {
	owners := 0
	for _, dir := range []string{".", "workflow", filepath.Join("..", "..", "cmd", "pinchtab")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s): %v", dir, err)
		}
		files := 0
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			files++
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", name, err)
			}
			for _, line := range strings.Split(string(src), "\n") {
				if strings.Contains(line, "//") {
					continue
				}
				if strings.Contains(line, `filepath.Join(`) && strings.Contains(line, `"profiles"`) {
					owners++
					t.Logf("profiles path rule in %s/%s: %s", dir, name, strings.TrimSpace(line))
				}
			}
		}
		if files == 0 {
			t.Fatalf("no non-test Go files found in %s, so this census read nothing", dir)
		}
	}
	if owners != 1 {
		t.Errorf("found %d places joining the state dir with \"profiles\", want exactly 1 (finalizeProfileConfig)", owners)
	}
}
