package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// declaredJSONPathFloor is the vacuity floor: the walk found this many leaf paths on
// FileConfig when it was written, so a rename that makes the walk stop descending fails
// here rather than reporting a clean parity between two empty sets.
const declaredJSONPathFloor = 150

// wireTwinExemptions are FileConfig paths that deliberately do NOT reach the wire type,
// with the reason each is excluded. Checked in BOTH directions: an exempt path that
// starts being marshalled must fail too, or the reason outlives the omission.
var wireTwinExemptions = map[string]string{
	"server.engine": "declared only so a config carrying the retired key gets a validation error instead of being silently ignored. Writing it back would re-create the key it exists to reject, so its absence from the wire type is the point rather than an oversight",

	"instanceDefaults.headless": "superseded by instanceDefaults.mode, which is the spelling the writer renders. ValidateFileConfig reports an error when both are set (mode takes precedence), so writing headless back would manufacture the conflict it warns about. Read from an old config, never written to a new one",
}

// FileConfig.MarshalJSON renders through fileConfigJSON, a hand-maintained twin. A field
// declared on FileConfig but missing from the twin is not a rendering nicety: the update
// path patches the existing file object with the marshalled map, so a key in neither the
// map nor the file cannot appear — `config patch` reports success and writes nothing.
// That is how sessions.agent went missing while the schema declared it and the loader
// honoured it.
//
// This walks both types rather than naming the fields, because the omission happened
// precisely because a human maintained the second copy by hand, and a hand-written list
// of what to check fails the same way.
func TestTheWireTwinCarriesEveryDeclaredField(t *testing.T) {
	declared := jsonLeafPaths(reflect.TypeOf(FileConfig{}))
	onWire := jsonLeafPaths(reflect.TypeOf(fileConfigJSON{}))

	if len(declared) < declaredJSONPathFloor {
		t.Fatalf("walked %d leaf paths on FileConfig, want at least %d; the walk stopped descending and this parity check would pass vacuously", len(declared), declaredJSONPathFloor)
	}
	if len(onWire) == 0 {
		t.Fatalf("walked no leaf paths on fileConfigJSON; the twin was renamed and this check compares nothing")
	}

	var missing []string
	for _, path := range declared {
		if onWire.has(path) {
			continue
		}
		if _, exempt := wireTwinExemptions[path]; exempt {
			continue
		}
		missing = append(missing, path)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("FileConfig declares %v but fileConfigJSON does not carry them, so MarshalJSON drops them and `config patch` reports success while writing nothing; add them to the twin, or exempt each in wireTwinExemptions with the reason it must not be written back", missing)
	}

	// The other direction on the exemption table.
	for path, reason := range wireTwinExemptions {
		if !declared.has(path) {
			t.Errorf("%s is exempted (%s) but FileConfig no longer declares it; drop the entry rather than leaving a reason with nothing to excuse", path, reason)
		}
		if onWire.has(path) {
			t.Errorf("%s is exempted from the wire type (%s) but fileConfigJSON now carries it; either it stopped being exempt, in which case delete the entry, or the field was added by mistake", path, reason)
		}
	}
}

// The specific member this card exists for, asserted by name beside the derived walk
// above. The derived check proves the CLASS is closed; this proves the instance, so a
// walk that silently stopped covering sessions cannot leave it green.
func TestTheAgentSessionBlockReachesTheWire(t *testing.T) {
	declared := jsonLeafPaths(reflect.TypeOf(FileConfig{}))
	onWire := jsonLeafPaths(reflect.TypeOf(fileConfigJSON{}))

	for _, path := range []string{
		"sessions.agent.enabled",
		"sessions.agent.mode",
		"sessions.agent.idleTimeoutSec",
		"sessions.agent.maxLifetimeSec",
	} {
		if !declared.has(path) {
			t.Fatalf("%s is no longer declared on FileConfig; this test is measuring nothing", path)
		}
		if !onWire.has(path) {
			t.Errorf("%s is declared but not on the wire type, so a config patch setting it reports success and writes nothing", path)
		}
	}
}

func TestTheWireTwinOmitsEveryUnsetPointerSettingTheStructOmits(t *testing.T) {
	declared := pointerLeafOmitempty(reflect.TypeFor[FileConfig]())
	onWire := pointerLeafOmitempty(reflect.TypeFor[fileConfigJSON]())
	if len(onWire) == 0 {
		t.Fatal("walked no pointer leaves on fileConfigJSON; this check compares nothing")
	}

	var drifted []string
	for path, wireOmits := range onWire {
		if declOmits, ok := declared[path]; ok && declOmits != wireOmits {
			drifted = append(drifted, path)
		}
	}
	sort.Strings(drifted)
	if len(drifted) > 0 {
		t.Errorf("omitempty differs between FileConfig and fileConfigJSON on %v, so an unset setting marshals as null in one and is absent in the other", drifted)
	}
}

func TestTheDefaultFileConfigMarshalsNoNull(t *testing.T) {
	out, err := json.Marshal(DefaultFileConfig())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "null") {
		t.Errorf("DefaultFileConfig marshals a null, which config init writes as an explicit unset key:\n%s", out)
	}
}

func TestAnExplicitEmptyAllowlistSurvivesMarshal(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Security.AllowedDomains = []string{}
	out, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"allowedDomains":[]`) {
		t.Errorf("an explicit empty allowedDomains was dropped, so reload falls back to the idpi alias:\n%s", out)
	}
}

type jsonPathSet []string

func (s jsonPathSet) has(path string) bool {
	for _, p := range s {
		if p == path {
			return true
		}
	}
	return false
}

func jsonLeafPaths(t reflect.Type) jsonPathSet {
	var paths jsonPathSet
	walkJSONLeaves(t, func(path string, _ reflect.StructField) {
		paths = append(paths, path)
	})
	sort.Strings(paths)
	return paths
}

func pointerLeafOmitempty(t reflect.Type) map[string]bool {
	leaves := map[string]bool{}
	walkJSONLeaves(t, func(path string, field reflect.StructField) {
		if field.Type.Kind() != reflect.Ptr {
			return
		}
		_, opts, _ := strings.Cut(field.Tag.Get("json"), ",")
		leaves[path] = slices.Contains(strings.Split(opts, ","), "omitempty")
	})
	return leaves
}

func walkJSONLeaves(t reflect.Type, visit func(path string, field reflect.StructField)) {
	var walk func(t reflect.Type, prefix string, depth int)
	walk = func(t reflect.Type, prefix string, depth int) {
		if depth > 8 {
			return
		}
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}

			fieldType := field.Type
			for fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct && fieldType.PkgPath() != "time" {
				walk(fieldType, path, depth+1)
				continue
			}
			visit(path, field)
		}
	}
	walk(t, "", 0)
}
