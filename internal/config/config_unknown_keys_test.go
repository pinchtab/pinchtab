package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfigFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	return path
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// The detection this replaces was dead code: encoding/json hands the whole
// document to FileConfig's custom UnmarshalJSON, so DisallowUnknownFields never
// applied. A strict decode also stops at the first unknown field, which is why
// the walk must report a LIST, and reports paths rather than leaves — a bare
// "allowedDomains" is declared in one section and not another.
func TestUnknownFileConfigKeysNamesEveryUnknownKeyByPath(t *testing.T) {
	keys := UnknownFileConfigKeys([]byte(`{
	  "server": {"porte": "18901", "token": "tok-sim-user-1234567890"},
	  "bogusSection": {"a": 1},
	  "browser": {"profilesDir": "/tmp/p"}
	}`))

	for _, want := range []string{"server.porte", "bogusSection", "browser.profilesDir"} {
		if !hasString(keys, want) {
			t.Errorf("unknown keys %v missing %q", keys, want)
		}
	}
	if len(keys) != 3 {
		t.Errorf("unknown keys = %v, want exactly the three unknown ones", keys)
	}
}

func TestUnknownFileConfigKeysAcceptsADocumentOfDeclaredKeys(t *testing.T) {
	keys := UnknownFileConfigKeys([]byte(`{
	  "$schema": "https://pinchtab.com/schema.json",
	  "server": {"port": "9867", "token": "tok-sim-user-1234567890", "logLevel": "debug"},
	  "security": {"allowedDomains": ["example.com"], "idpi": {"enabled": true}},
	  "profiles": {"baseDir": "/tmp/profiles"},
	  "autoSolver": {"enabled": true, "solvers": ["cloudflare"]},
	  "browser": {"targets": {"my-chrome": {"provider": "chrome", "binary": "/usr/bin/chrome"}}}
	}`))

	if len(keys) != 0 {
		t.Errorf("a config of declared keys reported %v as unknown", keys)
	}
}

func TestUnknownFileConfigKeysAcceptsRetiredEnableActionGuards(t *testing.T) {
	keys := UnknownFileConfigKeys([]byte(`{"security":{"enableActionGuards":false}}`))
	if len(keys) != 0 {
		t.Fatalf("retired compatibility key reported as unknown: %v", keys)
	}
}

// browser.targets keys are operator-chosen, so a target name can never be an
// unknown key — but a misspelled field INSIDE one still must be.
func TestUnknownFileConfigKeysTreatsMapKeysAsOpenAndStillChecksTheirValues(t *testing.T) {
	keys := UnknownFileConfigKeys([]byte(`{"browser": {"targets": {"anything-goes": {"binarie": "/usr/bin/chrome"}}}}`))

	if hasString(keys, "browser.targets.anything-goes") {
		t.Errorf("an operator-named target was reported as unknown: %v", keys)
	}
	if !hasString(keys, "browser.targets.anything-goes.binarie") {
		t.Errorf("unknown keys %v missing the misspelled field inside the target", keys)
	}
}

// The one supported alias no FileConfig field declares. A strict pass reports it
// as a typo on every config that uses it, so the exemption is the false positive
// this change would otherwise ship — and it is derived from the very types
// NormalizeFileConfigAliasesFromJSON reads, so the two cannot drift.
func TestSupportedAliasIsNotReportedAsUnknown(t *testing.T) {
	doc := []byte(`{"security": {"idpi": {"allowedDomains": ["example.com"]}}}`)

	if keys := UnknownFileConfigKeys(doc); len(keys) != 0 {
		t.Errorf("the supported security.idpi.allowedDomains alias was reported as unknown: %v", keys)
	}

	fc := &FileConfig{}
	NormalizeFileConfigAliasesFromJSON(fc, doc)
	if len(fc.Security.AllowedDomains) != 1 || fc.Security.AllowedDomains[0] != "example.com" {
		t.Fatalf("the alias no longer normalises to security.allowedDomains (%v); the exemption above would then be excusing a key nothing honours", fc.Security.AllowedDomains)
	}
}

// The exemptions must stay DERIVED from the types NormalizeFileConfigAliasesFromJSON
// reads, not restated as a literal beside them: an alias added to the normaliser
// would then still be reported as a typo. Replacing the derivation with a
// hand-written list reds this the moment the two disagree.
func TestAliasExemptionsAreDerivedFromTheNormalisersOwnTypes(t *testing.T) {
	exempt := aliasFileConfigPaths()
	if len(exempt) == 0 {
		t.Fatal("no alias paths derived; the exemption set is empty and this test guards nothing")
	}

	declared := declaredPaths("", reflect.TypeOf(aliasRawConfig{}))
	if len(declared) != len(exempt) {
		t.Errorf("exempt set %v does not match the alias types' declared paths %v", exempt, declared)
	}
	for _, path := range declared {
		if !exempt[path] {
			t.Errorf("alias type declares %q but the walk does not exempt it", path)
		}
	}
	if !exempt["security.idpi.allowedDomains"] {
		t.Errorf("the alias that no FileConfig field declares is missing from the derived set: %v", exempt)
	}
}

func TestLoadConfigReportsUnknownKeysAsAWarnDiagnostic(t *testing.T) {
	writeConfigFixture(t, `{"server":{"port":"18901","porte":"18901","token":"tok-sim-user-1234567890"},"bogusSection":{"a":1}}`)

	cfg, diags, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != "18901" {
		t.Fatalf("the declared keys must still load: port = %q", cfg.Port)
	}

	var found *LoadDiagnostic
	for i := range diags {
		if strings.Contains(diags[i].Message, "unrecognized fields") {
			found = &diags[i]
		}
	}
	if found == nil {
		t.Fatalf("no unrecognized-fields diagnostic among %d diagnostics", len(diags))
	}
	if found.Level != slog.LevelWarn {
		t.Errorf("diagnostic level = %v, want %v", found.Level, slog.LevelWarn)
	}
	rendered := renderDiagnosticAttrs(found.Attrs)
	for _, want := range []string{"server.porte", "bogusSection"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("diagnostic attrs %q do not name %q", rendered, want)
		}
	}
}

func renderDiagnosticAttrs(attrs []any) string {
	var parts []string
	for _, attr := range attrs {
		if err, ok := attr.(error); ok {
			parts = append(parts, err.Error())
			continue
		}
		if text, ok := attr.(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// InspectConfigFile is what doctor reads, and it used to drop the unknown-key
// result readAndParseConfigFile had already computed.
func TestInspectConfigFileCarriesUnknownKeys(t *testing.T) {
	writeConfigFixture(t, `{"server":{"porte":"18901","token":"tok-sim-user-1234567890"},"bogusSection":{"a":1}}`)

	status := InspectConfigFile()
	if !status.Found {
		t.Fatal("fixture config not found")
	}
	if len(status.UnknownKeys) != 2 {
		t.Fatalf("status.UnknownKeys = %v, want both unknown keys", status.UnknownKeys)
	}
	for _, want := range []string{"server.porte", "bogusSection"} {
		if !hasString(status.UnknownKeys, want) {
			t.Errorf("status.UnknownKeys %v missing %q", status.UnknownKeys, want)
		}
	}
}

// The error text is what the warning renders, so it must carry the paths.
func TestUnknownConfigKeysErrorNamesThePaths(t *testing.T) {
	one := (&UnknownConfigKeysError{Keys: []string{"server.porte"}}).Error()
	if !strings.Contains(one, "server.porte") {
		t.Errorf("single-key error = %q", one)
	}
	many := (&UnknownConfigKeysError{Keys: []string{"server.porte", "bogusSection"}}).Error()
	if !strings.Contains(many, "server.porte") || !strings.Contains(many, "bogusSection") {
		t.Errorf("multi-key error = %q", many)
	}
}

// The walk checks the document against FileConfig's own tags, which is only the
// right key set while FileConfig's UnmarshalJSON decodes a defined-type shadow of
// ITSELF. This package also carries fileConfigJSON, a separate shape used for
// WRITING; if the reader ever switched to that, the walk would start reporting
// keys the loader honours and accepting keys it ignores.
func TestReaderDecodesAFileConfigShadowSoTheWalkChecksTheRightKeys(t *testing.T) {
	src, err := os.ReadFile("config_file_marshal.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func (fc *FileConfig) UnmarshalJSON(")
	if start < 0 {
		t.Fatal("FileConfig.UnmarshalJSON not found; the unknown-key walk's premise cannot be checked")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not delimit UnmarshalJSON")
	}
	unmarshaler := body[start : start+end]

	if !strings.Contains(unmarshaler, "type rawFileConfig FileConfig") {
		t.Error("UnmarshalJSON no longer decodes a defined-type shadow of FileConfig; UnknownFileConfigKeys walks FileConfig's tags and must be pointed at whatever shape the reader now uses")
	}
	if strings.Contains(unmarshaler, "fileConfigJSON") {
		t.Error("UnmarshalJSON decodes the write-side fileConfigJSON shape; the unknown-key walk reads FileConfig's tags, so the two key sets can now disagree")
	}
}
