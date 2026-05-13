package schema

import (
	"encoding/json"
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
