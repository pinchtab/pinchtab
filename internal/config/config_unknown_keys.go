package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// UnknownConfigKeysError names every key in a config file that no struct tag
// declares. It carries the paths so callers can list them rather than re-parse a
// message.
type UnknownConfigKeysError struct {
	Keys []string
}

func (e *UnknownConfigKeysError) Error() string {
	if len(e.Keys) == 1 {
		return fmt.Sprintf("unrecognized config key %q", e.Keys[0])
	}
	return fmt.Sprintf("unrecognized config keys %s", strings.Join(e.Keys, ", "))
}

// UnknownFileConfigKeys reports the dotted paths in a config document that
// FileConfig does not declare — "server.porte", "bogusSection".
//
// It walks the document against the struct types instead of using
// json.Decoder.DisallowUnknownFields, for two reasons the alternative cannot
// meet: FileConfig has a custom UnmarshalJSON, which makes the decoder's strict
// setting a no-op for the whole document, and a strict decode stops at the FIRST
// unknown field, so it can never name more than one. Reporting a path rather
// than a leaf matters too — "allowedDomains" alone is ambiguous, since it is
// declared in one section and not another.
func UnknownFileConfigKeys(data []byte) []string {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	keys := collectUnknownKeys("", root, reflect.TypeOf(FileConfig{}), acceptedUndeclaredFileConfigPaths())
	sort.Strings(keys)
	return keys
}

func collectUnknownKeys(prefix string, raw map[string]json.RawMessage, structType reflect.Type, exempt map[string]bool) []string {
	var unknown []string
	for name, value := range raw {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		field, ok := jsonFieldByName(structType, name)
		if !ok {
			if !exempt[path] {
				unknown = append(unknown, path)
			}
			continue
		}
		unknown = append(unknown, collectUnknownKeysUnder(path, value, field.Type, exempt)...)
	}
	return unknown
}

// collectUnknownKeysUnder recurses into a declared field's value. A map-typed
// field accepts arbitrary keys (browser targets, the retired browsers.config),
// so its keys are never unknown while its VALUES are still checked against the
// element type.
func collectUnknownKeysUnder(path string, value json.RawMessage, fieldType reflect.Type, exempt map[string]bool) []string {
	fieldType = derefType(fieldType)
	switch fieldType.Kind() {
	case reflect.Struct:
		nested, ok := decodeObject(value)
		if !ok {
			return nil
		}
		return collectUnknownKeys(path, nested, fieldType, exempt)
	case reflect.Map:
		entries, ok := decodeObject(value)
		if !ok {
			return nil
		}
		var unknown []string
		for key, entry := range entries {
			unknown = append(unknown, collectUnknownKeysUnder(path+"."+key, entry, fieldType.Elem(), exempt)...)
		}
		return unknown
	default:
		return nil
	}
}

func decodeObject(value json.RawMessage) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return nil, false
	}
	return object, true
}

// jsonFieldByName resolves a document key to a struct field the way
// encoding/json does: exact json tag first, then a case-insensitive match.
func jsonFieldByName(structType reflect.Type, name string) (reflect.StructField, bool) {
	if structType.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	var fallback reflect.StructField
	found := false
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := jsonTagName(field)
		if tag == "-" {
			continue
		}
		if tag == name {
			return field, true
		}
		if !found && strings.EqualFold(tag, name) {
			fallback, found = field, true
		}
	}
	return fallback, found
}

func jsonTagName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}
	return name
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// aliasFileConfigPaths lists the keys a config file may legitimately carry that
// FileConfig does not declare, derived from the raw types
// NormalizeFileConfigAliasesFromJSON reads. Deriving them rather than listing
// them is the point: an alias added to the normaliser is exempt here without a
// second edit, and today's alias — security.idpi.allowedDomains — is otherwise
// reported as a typo on every config that uses it.
func aliasFileConfigPaths() map[string]bool {
	exempt := map[string]bool{}
	for _, path := range declaredPaths("", reflect.TypeOf(aliasRawConfig{})) {
		exempt[path] = true
	}
	return exempt
}

func acceptedUndeclaredFileConfigPaths() map[string]bool {
	exempt := aliasFileConfigPaths()
	// Retired in PIN-315. Existing config files must continue to load, but the
	// key is deliberately absent from FileConfig, the editor, and the schema.
	exempt["security.enableActionGuards"] = true
	return exempt
}

func declaredPaths(prefix string, structType reflect.Type) []string {
	structType = derefType(structType)
	if structType.Kind() != reflect.Struct {
		return nil
	}
	var paths []string
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" || jsonTagName(field) == "-" {
			continue
		}
		path := jsonTagName(field)
		if prefix != "" {
			path = prefix + "." + path
		}
		paths = append(paths, path)
		paths = append(paths, declaredPaths(path, field.Type)...)
	}
	return paths
}
