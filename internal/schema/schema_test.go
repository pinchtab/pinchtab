package schema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestConfigSchemaMetadata(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal(ConfigJSON, &raw); err != nil {
		t.Fatalf("ConfigJSON is not valid JSON: %v", err)
	}

	if raw["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Fatalf("schema dialect = %q, want draft-07", raw["$schema"])
	}
	if raw["$id"] != config.ConfigSchemaURL {
		t.Fatalf("schema $id = %q, want %q", raw["$id"], config.ConfigSchemaURL)
	}

	properties, ok := raw["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties missing or malformed")
	}
	schemaProp, ok := properties["$schema"].(map[string]any)
	if !ok {
		t.Fatal("schema does not describe config $schema property")
	}
	if schemaProp["default"] != config.ConfigSchemaURL {
		t.Fatalf("config $schema default = %q, want %q", schemaProp["default"], config.ConfigSchemaURL)
	}
}

func TestConfigJSONForURL(t *testing.T) {
	const schemaURL = "https://raw.githubusercontent.com/pinchtab/pinchtab/v1.2.3/schema/config.json"

	data, err := ConfigJSONForURL(schemaURL)
	if err != nil {
		t.Fatalf("ConfigJSONForURL() error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("rendered schema is not valid JSON: %v", err)
	}
	if raw["$id"] != schemaURL {
		t.Fatalf("schema $id = %q, want %q", raw["$id"], schemaURL)
	}

	properties := raw["properties"].(map[string]any)
	schemaProp := properties["$schema"].(map[string]any)
	if schemaProp["default"] != schemaURL {
		t.Fatalf("config $schema default = %q, want %q", schemaProp["default"], schemaURL)
	}
}

// Every JSON tag of the browser-config structs must exist in the schema —
// additionalProperties:false means a missing property rejects valid configs
// in $schema-aware tooling (H10a regression).
func TestConfigSchemaCoversBrowserConfigFields(t *testing.T) {
	var raw struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(ConfigJSON, &raw); err != nil {
		t.Fatalf("ConfigJSON is not valid JSON: %v", err)
	}

	cases := []struct {
		definition string
		structType reflect.Type
	}{
		{"browser", reflect.TypeOf(config.BrowserConfig{})},
		{"browserTarget", reflect.TypeOf(config.BrowserTargetConfig{})},
		{"browserProxy", reflect.TypeOf(config.BrowserProxyConfig{})},
	}
	for _, tc := range cases {
		def, ok := raw.Definitions[tc.definition]
		if !ok {
			t.Errorf("schema missing definition %q", tc.definition)
			continue
		}
		for _, tag := range jsonTags(tc.structType) {
			if _, ok := def.Properties[tag]; !ok {
				t.Errorf("definitions.%s missing property %q (from %s)", tc.definition, tag, tc.structType.Name())
			}
		}
	}

	// Geo nests under browserProxy.properties.geo.properties.
	var proxyDef struct {
		Properties struct {
			Geo struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"geo"`
		} `json:"properties"`
	}
	var defsOnly map[string]json.RawMessage
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(ConfigJSON, &root); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(root["definitions"], &defsOnly); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(defsOnly["browserProxy"], &proxyDef); err != nil {
		t.Fatal(err)
	}
	for _, tag := range jsonTags(reflect.TypeOf(config.BrowserProxyGeoConfig{})) {
		if _, ok := proxyDef.Properties.Geo.Properties[tag]; !ok {
			t.Errorf("browserProxy.geo missing property %q", tag)
		}
	}
}

func jsonTags(t reflect.Type) []string {
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name != "" {
			tags = append(tags, name)
		}
	}
	return tags
}

// The retired browsers.config block must stay out of the schema so editors
// flag it; validation rejects it with browser.targets guidance.
func TestConfigSchemaOmitsRetiredBrowsersConfig(t *testing.T) {
	var raw struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(ConfigJSON, &raw); err != nil {
		t.Fatalf("ConfigJSON is not valid JSON: %v", err)
	}
	if _, ok := raw.Definitions["browsers"].Properties["config"]; ok {
		t.Fatal("definitions.browsers must not advertise the retired config block")
	}
}
