package config

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/catalog"
)

// documentMetadataSections are the two FileConfig members that are NOT configuration:
// they describe the document rather than the server. They are excluded here deliberately
// so nobody adds them to configSections to satisfy the walk below, and so nobody deletes
// the exclusion as an oversight.
var documentMetadataSections = map[string]bool{
	"$schema":       true,
	"configVersion": true,
}

// The criterion that makes this class of drift impossible. autoSolver and scheduler were
// both declared on FileConfig, honoured by the config file, documented, and exposed by
// the dashboard — and rejected by config set/get, because the section vocabulary was
// typed out by hand in four places and tied to nothing. This walks the struct instead,
// so a section added to FileConfig and nowhere else fails here rather than in an
// operator's terminal.
func TestEverySettableFileConfigSectionIsReachableFromBothResolvers(t *testing.T) {
	declared := declaredFileConfigSections(t)
	if len(declared) < 10 {
		t.Fatalf("found %d sections on FileConfig; this walk would be vacuous", len(declared))
	}

	table := map[string]bool{}
	for _, section := range configSections {
		table[section.name] = true
	}

	for _, name := range declared {
		if documentMetadataSections[name] {
			if table[name] {
				t.Errorf("%q is document metadata, not configuration, but configSections claims it", name)
			}
			continue
		}
		if !table[name] {
			t.Errorf("FileConfig declares section %q but configSections does not, so config set/get reject it — add it to the table rather than to a switch", name)
			continue
		}

		// Reachability is about the SECTION, not the field: drive a field name that
		// cannot exist and require the refusal to be about the field. An unknown-section
		// error here is the defect this test exists to catch.
		var fc FileConfig
		const bogusField = "definitelyNotAField"
		if err := SetConfigValue(&fc, name+"."+bogusField, "x"); err == nil {
			t.Errorf("set %s.%s was accepted; the section resolver validates nothing", name, bogusField)
		} else if strings.Contains(err.Error(), "unknown section") {
			t.Errorf("set %s.%s reports %v — the section is unreachable", name, bogusField, err)
		}
		if _, err := GetConfigValue(&fc, name+"."+bogusField); err == nil {
			t.Errorf("get %s.%s was accepted; the section resolver validates nothing", name, bogusField)
		} else if strings.Contains(err.Error(), "unknown section") {
			t.Errorf("get %s.%s reports %v — the section is unreachable", name, bogusField, err)
		}
	}

	// The other direction: a table entry naming a section FileConfig no longer declares
	// would leave the error message advertising a vocabulary that resolves to nothing.
	declaredSet := map[string]bool{}
	for _, name := range declared {
		declaredSet[name] = true
	}
	for _, section := range configSections {
		if !declaredSet[section.name] {
			t.Errorf("configSections lists %q, which FileConfig no longer declares", section.name)
		}
	}
}

// The schema walk must not be satisfiable by accepting everything, and the refusal has to
// keep naming the valid sections now that the list is generated rather than typed.
func TestABogusSectionIsStillRefusedAndTheRefusalNamesTheValidOnes(t *testing.T) {
	var fc FileConfig

	for _, path := range []string{"bogusSection.field", "autosolver.enabled", "Scheduler.enabled"} {
		setErr := SetConfigValue(&fc, path, "x")
		if setErr == nil {
			t.Errorf("set %s was accepted", path)
			continue
		}
		if !strings.Contains(setErr.Error(), "unknown section") {
			t.Errorf("set %s = %v, want an unknown-section refusal", path, setErr)
		}
		if _, getErr := GetConfigValue(&fc, path); getErr == nil {
			t.Errorf("get %s was accepted", path)
		}
	}

	err := SetConfigValue(&fc, "bogusSection.field", "x")
	// Generated from the table, so it can never advertise a vocabulary the resolvers
	// do not have — which is what the four hand-typed copies eventually did.
	for _, want := range []string{"server", "autoSolver", "scheduler"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name the valid section %q", err, want)
		}
	}
}

// The key-gated solver warning hands an operator a config path to set. Nothing verified
// those hand-written strings named anything real: renaming the JSON tag they point at
// left autosolver, catalog, config and handlers all green, and the warning would then
// advise a key the config no longer reads. This is the cross-reference, and it only
// became possible once the editor knew the autoSolver section.
func TestEveryKeyGatedSolverConfigKeyResolves(t *testing.T) {
	names := catalog.KeyGated()
	if len(names) == 0 {
		t.Fatal("catalog reports no key-gated solvers; this cross-reference would be vacuous")
	}

	var fc FileConfig
	for _, name := range names {
		gated, ok := autosolver.KeyGatedSolverNamed(name)
		if !ok {
			t.Errorf("catalog.KeyGated lists %q but autosolver.KeyGatedSolverNamed does not know it", name)
			continue
		}
		if gated.ConfigKey == "" {
			t.Errorf("key-gated solver %q names no config key, so its warning cannot tell an operator what to set", name)
			continue
		}
		if _, err := GetConfigValue(&fc, gated.ConfigKey); err != nil {
			t.Errorf("solver %q points operators at %q, which config get rejects: %v", name, gated.ConfigKey, err)
		}
		if err := SetConfigValue(&fc, gated.ConfigKey, "probe"); err != nil {
			t.Errorf("solver %q points operators at %q, which config set rejects: %v", name, gated.ConfigKey, err)
		}
	}
}

func declaredFileConfigSections(t *testing.T) []string {
	t.Helper()
	typ := reflect.TypeFor[FileConfig]()
	names := make([]string, 0, typ.NumField())
	for field := range typ.Fields() {
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			t.Errorf("FileConfig field %s has no json tag, so it cannot be addressed by path", field.Name)
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	return names
}

// unreachableConfigLeaves records the leaf paths a section declares that config set/get
// deliberately do NOT address, with the reason each is excluded. The walk below fails both
// ways: a leaf missing from the resolvers and from this table is a gap, and an entry here
// that now resolves is stale and must be deleted. Padding the table to silence the walk is
// therefore visible in review as a reason someone had to write.
var unreachableConfigLeaves = map[string]string{
	"server.engine": "removed setting, parsed only so an old config gets a validation error; making it settable would offer an operator a value nothing honours",

	"instanceDefaults.headless": "superseded by instanceDefaults.mode, which is the spelling the file writer renders; setting both is a validation error, and a settable headless would be dropped by MarshalJSON and read as accepted-then-discarded",
}

// unreachableConfigSubtrees are whole blocks excluded by one reason, so the table does not
// have to list every leaf of a retired structure.
var unreachableConfigSubtrees = map[string]string{
	"browsers.config.": "retired block, never applied anywhere and superseded by browser.targets; parsed only so validation can reject it with guidance and so existing files round-trip",
}

// The section walk one level down. The section vocabulary is derived from FileConfig and
// pinned above; the per-field vocabulary was still hand-written switches tied to nothing,
// so a field added to a section struct was honoured by the loader, documented by the
// schema, and silently unaddressable from the CLI. This walks the section structs' json
// tags instead: a leaf that resolves through neither resolver has to be either wired up or
// written down.
func TestEveryDeclaredSectionLeafIsReachableFromBothResolvers(t *testing.T) {
	leaves := declaredSectionLeaves(t)
	if len(leaves) < 100 {
		t.Fatalf("walked %d leaves across the section structs; this walk would be vacuous", len(leaves))
	}

	exempted := map[string]bool{}
	for _, leaf := range leaves {
		reason, key := exemptionFor(leaf.path)

		// The probe carries a value of the type the section struct declares, so a refusal
		// can only be about the PATH. Probing every leaf with one string made a setter that
		// parses before it dispatches answer "invalid boolean" for a string field, which
		// reads as a value complaint and hid a leaf no value could ever set.
		// One config per leaf, written before it is read: a map-keyed block (browser
		// targets) only has the key an operator just created, so reading a fresh config
		// would report "not found" for every one of its leaves.
		fc := newProbeConfig()
		setErr := SetConfigValue(fc, leaf.path, leaf.probe)
		_, getErr := GetConfigValue(fc, leaf.path)

		if reason == "" {
			if policy, refused := refusedByPolicy[leaf.path]; refused {
				if setErr == nil && getErr == nil {
					t.Errorf("%s is recorded as refused by policy (%q) but both resolvers now accept it; delete the entry", leaf.path, policy)
				}
				exempted[leaf.path] = true
				continue
			}
			if setErr != nil {
				t.Errorf("config set %s = %q was refused (%v): the leaf is declared on the section struct and honoured by the loader, so wire it into the setter, or record it in unreachableConfigLeaves / refusedByPolicy with a reason", leaf.path, leaf.probe, setErr)
			}
			if getErr != nil {
				t.Errorf("config get %s was refused (%v): wire it into the getter, or record it with a reason", leaf.path, getErr)
			}
			continue
		}

		exempted[key] = true
		if !isUnknownFieldRefusal(setErr) && !isUnknownFieldRefusal(getErr) {
			t.Errorf("%s resolves through both resolvers but is still recorded as unreachable (%q); delete the entry — a stale exemption hides the next gap", leaf.path, reason)
		}
	}

	// The exemption table cannot outlive what it excuses: an entry naming a path the walk
	// no longer produces would excuse nothing and read as coverage.
	for path := range unreachableConfigLeaves {
		if !exempted[path] {
			t.Errorf("unreachableConfigLeaves records %q, which is not a leaf any section struct declares", path)
		}
	}
	for path := range refusedByPolicy {
		if !exempted[path] {
			t.Errorf("refusedByPolicy records %q, which is not a leaf any section struct declares", path)
		}
	}
	for prefix := range unreachableConfigSubtrees {
		if !exempted[prefix] {
			t.Errorf("unreachableConfigSubtrees records %q, which matches no leaf any section struct declares", prefix)
		}
	}
}

// stateEncryptionKey is a secret and this card made it readable, so the reachability is
// pinned here while the masking stays pinned where the rule lives: isSensitiveConfigPath in
// cmd/pinchtab already lists this exact path, and restating its suffix rule here would be a
// second copy free to drift. What this package owns is the leaf answering at all.
//
// Masking is a DISPLAY property of `pinchtab config get`, not a property of the field: any
// later surface that publishes config values — an HTTP endpoint, a dashboard view — inherits
// none of it and has to mask on its own.
func TestTheStateEncryptionKeyLeafIsReadableSoItsMaskingMatters(t *testing.T) {
	const path = "security.stateEncryptionKey"

	fc := newProbeConfig()
	if err := SetConfigValue(fc, path, "sekret"); err != nil {
		t.Fatalf("set %s: %v", path, err)
	}
	value, err := GetConfigValue(fc, path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	if value != "sekret" {
		t.Fatalf("get %s = %q, want the value just set", path, value)
	}
}

func newProbeConfig() *FileConfig {
	return &FileConfig{}
}

// isUnknownFieldRefusal separates "this resolver does not know the path" from every other
// refusal. A typed rejection (a bad value, a removed setting) means the leaf RESOLVED,
// which is what this walk is about.
func isUnknownFieldRefusal(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown field")
}

func exemptionFor(path string) (reason, key string) {
	if reason, ok := unreachableConfigLeaves[path]; ok {
		return reason, path
	}
	for prefix, reason := range unreachableConfigSubtrees {
		if strings.HasPrefix(path, prefix) {
			return reason, prefix
		}
	}
	return "", ""
}

// declaredSectionLeaves walks each section in configSections through the struct FileConfig
// declares for it, so the paths come from the type rather than from a list somebody
// maintains. Map-valued blocks are walked through a synthetic key: their leaves are what
// the resolvers address, and the key itself is the operator's choice.
// refusedByPolicy are leaves both resolvers KNOW and deliberately refuse, which is a
// different answer from not knowing them: the operator is told what to use instead.
var refusedByPolicy = map[string]string{
	"browser.provider": "removed in favour of browsers.default; both resolvers answer with that redirection rather than an unknown-field refusal",

	"observability.activity.stateDir": "derived from server.stateDir so two instances cannot share an activity log directory; the setter refuses it by naming the key to set instead, and the getter still reads it",
}

type sectionLeaf struct {
	path  string
	probe string
}

func declaredSectionLeaves(t *testing.T) []sectionLeaf {
	t.Helper()

	fields := map[string]reflect.Type{}
	typ := reflect.TypeFor[FileConfig]()
	for field := range typ.Fields() {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		fields[name] = field.Type
	}

	var leaves []sectionLeaf
	for _, section := range configSections {
		sectionType, ok := fields[section.name]
		if !ok {
			t.Fatalf("configSections lists %q, which FileConfig does not declare", section.name)
		}
		appendLeafPaths(t, section.name, sectionType, &leaves)
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].path < leaves[j].path })
	return leaves
}

// probeValueFor renders a value of the leaf's declared type. Values are chosen to survive
// the validation the setters apply (a hostname for a domain list, a percentage for a
// threshold), because a probe rejected on its content would report a gap that is not one.
func probeValueFor(t *testing.T, path string, typ reflect.Type) string {
	t.Helper()
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Bool:
		return "true"
	case reflect.Int, reflect.Int64:
		return "7"
	case reflect.Float64:
		return "7"
	case reflect.Slice:
		return "example.com"
	case reflect.String:
		return probeStringFor(path)
	default:
		t.Fatalf("%s has unsupported leaf kind %s; teach the walk how to probe it", path, typ.Kind())
		return ""
	}
}

// probeStringFor supplies the few string leaves whose setter validates the value against a
// vocabulary. Everything else takes a plain marker.
func probeStringFor(path string) string {
	switch path {
	case "server.logLevel":
		return "info"
	case "browsers.default":
		return "chrome"
	case "instanceDefaults.mode":
		return "headless"
	case "browser.targets.probe.provider":
		return "chrome"
	}
	return "probe"
}

func appendLeafPaths(t *testing.T, prefix string, typ reflect.Type, leaves *[]sectionLeaf) {
	t.Helper()
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		for field := range typ.Fields() {
			tag := field.Tag.Get("json")
			if tag == "" || tag == "-" {
				t.Errorf("%s.%s has no json tag, so it cannot be addressed by path", prefix, field.Name)
				continue
			}
			appendLeafPaths(t, prefix+"."+strings.Split(tag, ",")[0], field.Type, leaves)
		}
	case reflect.Map:
		appendLeafPaths(t, prefix+".probe", typ.Elem(), leaves)
	default:
		*leaves = append(*leaves, sectionLeaf{path: prefix, probe: probeValueFor(t, prefix, typ)})
	}
}

// The other half of reachable: a value config set accepts has to survive the file writer.
// FileConfig renders through a hand-built JSON twin, so a leaf wired into the editor but
// missing from that twin reports success and writes nothing — accept-and-discard, which is
// worse than the unknown-field refusal it replaced. This drives every settable leaf through
// the same marshal/unmarshal the save path uses.
func TestEverySettableSectionLeafSurvivesTheFileWriter(t *testing.T) {
	leaves := declaredSectionLeaves(t)
	checked := 0

	for _, leaf := range leaves {
		if reason, _ := exemptionFor(leaf.path); reason != "" {
			continue
		}
		if _, refused := refusedByPolicy[leaf.path]; refused {
			continue
		}

		fc := newProbeConfig()
		if err := SetConfigValue(fc, leaf.path, leaf.probe); err != nil {
			continue // the reachability walk above owns this failure
		}
		want, err := GetConfigValue(fc, leaf.path)
		if err != nil {
			continue
		}

		encoded, err := json.Marshal(fc)
		if err != nil {
			t.Fatalf("marshal after setting %s: %v", leaf.path, err)
		}
		var reloaded FileConfig
		if err := json.Unmarshal(encoded, &reloaded); err != nil {
			t.Fatalf("reload after setting %s: %v", leaf.path, err)
		}

		got, err := GetConfigValue(&reloaded, leaf.path)
		if err != nil {
			t.Errorf("%s reads back as an error after a save/load round trip: %v", leaf.path, err)
			continue
		}
		if got != want {
			t.Errorf("config set %s = %q is accepted and then discarded: the value reads %q after the file writer round trip. Add the field to the JSON twin in config_file_json.go and to MarshalJSON, or the CLI reports success and writes nothing", leaf.path, want, got)
		}
		checked++
	}

	if checked < 100 {
		t.Fatalf("only %d settable leaves survived to the round-trip assertion; this guard would be vacuous", checked)
	}
}
