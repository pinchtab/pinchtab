package extract

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func loadSnapshot(t *testing.T, name string) []observe.A11yNode {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var snap struct {
		Nodes []observe.A11yNode `json:"nodes"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode testdata: %v", err)
	}
	return snap.Nodes
}

func TestResolve_FromSnapshotFixtures(t *testing.T) {
	t.Run("product", func(t *testing.T) {
		schema := mustSchema(t, `{"type":"object","required":["name","price","in_stock"],"properties":{
			"name":{"type":"string","description":"product name","x-pinchtab-hint":"role:heading"},
			"price":{"type":"number","description":"product price"},
			"rating":{"type":"number","description":"product rating out of 5"},
			"in_stock":{"type":"boolean","description":"in stock availability"},
			"description":{"type":"string","description":"product description"}}}`)
		got := Resolve(schema, loadSnapshot(t, "extract-product.json"), Options{})
		want := map[string]any{
			"name":     "Sony WH-1000XM5 Wireless Headphones",
			"price":    1299.0,
			"rating":   4.7,
			"in_stock": true,
		}
		for k, v := range want {
			if got.Data[k] != v {
				t.Errorf("data[%q] = %#v, want %#v (%+v)", k, got.Data[k], v, got.Fields[k])
			}
		}
		if len(got.Missing) != 0 {
			t.Errorf("unexpected missing: %v", got.Missing)
		}
	})

	t.Run("article", func(t *testing.T) {
		schema := mustSchema(t, `{"type":"object","required":["title","author","date"],"properties":{
			"title":{"type":"string","description":"article title","x-pinchtab-hint":"role:heading"},
			"author":{"type":"string","description":"author name"},
			"date":{"type":"string","description":"date published"}}}`)
		got := Resolve(schema, loadSnapshot(t, "extract-article.json"), Options{})
		want := map[string]any{
			"title":  "The Rise of Model-Free Extraction",
			"author": "Jane Doe",
			"date":   "2026-09-12",
		}
		for k, v := range want {
			if got.Data[k] != v {
				t.Errorf("data[%q] = %#v, want %#v (%+v)", k, got.Data[k], v, got.Fields[k])
			}
		}
	})
}

func productNodes() []observe.A11yNode {
	return []observe.A11yNode{
		{Ref: "e1", Role: "region", Name: "Product", Depth: 0},
		{Ref: "e2", Role: "heading", Name: "Product name", Text: "Sony WH-1000XM5 Wireless Headphones", Depth: 1},
		{Ref: "e3", Role: "text", Name: "Price", Text: "$1,299.00", Depth: 1},
		{Ref: "e4", Role: "checkbox", Name: "In stock", Checked: observe.CheckedTrue, Depth: 1},
		{Ref: "e5", Role: "text", Name: "Colour", Text: "Midnight Black", Depth: 1},
	}
}

func articleNodes() []observe.A11yNode {
	return []observe.A11yNode{
		{Ref: "e1", Role: "article", Name: "Story", Depth: 0},
		{Ref: "e2", Role: "heading", Name: "Article title", Text: "The Rise of Model-Free Extraction", Depth: 1},
		{Ref: "e3", Role: "text", Name: "Author", Text: "Jane Doe", Depth: 1},
		{Ref: "e4", Role: "text", Name: "Date published", Text: "2026-09-12", Depth: 1},
		{Ref: "e5", Role: "text", Name: "Body", Text: "Lorem ipsum dolor sit amet.", Depth: 1},
	}
}

func formNodes() []observe.A11yNode {
	return []observe.A11yNode{
		{Ref: "e1", Role: "form", Name: "Signup", Depth: 0},
		{Ref: "e2", Role: "textbox", Name: "Email address", Value: "user@example.com", Depth: 1},
		{Ref: "e3", Role: "textbox", Name: "Full name", Value: "Ada Lovelace", Depth: 1},
		{Ref: "e4", Role: "checkbox", Name: "Subscribe to newsletter", Checked: observe.CheckedFalse, Depth: 1},
	}
}

func mustSchema(t *testing.T, doc string) Schema {
	t.Helper()
	s, err := ParseSchema([]byte(doc))
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	return s
}

func TestResolve_TableFixtures(t *testing.T) {
	tests := []struct {
		name     string
		schema   string
		nodes    []observe.A11yNode
		wantData map[string]any
		wantRefs map[string]string
		missing  []string
	}{
		{
			name: "product page",
			schema: `{"type":"object","required":["name","price","in_stock"],"properties":{
				"name":{"type":"string","description":"product name"},
				"price":{"type":"number","description":"product price"},
				"in_stock":{"type":"boolean","description":"in stock availability"}}}`,
			nodes: productNodes(),
			wantData: map[string]any{
				"name":     "Sony WH-1000XM5 Wireless Headphones",
				"price":    1299.0,
				"in_stock": true,
			},
			wantRefs: map[string]string{"name": "e2", "price": "e3", "in_stock": "e4"},
		},
		{
			name: "article",
			schema: `{"type":"object","required":["title","author","date"],"properties":{
				"title":{"type":"string","description":"article title"},
				"author":{"type":"string","description":"author name"},
				"date":{"type":"string","description":"date published"}}}`,
			nodes: articleNodes(),
			wantData: map[string]any{
				"title":  "The Rise of Model-Free Extraction",
				"author": "Jane Doe",
				"date":   "2026-09-12",
			},
			wantRefs: map[string]string{"title": "e2", "author": "e3", "date": "e4"},
		},
		{
			name: "form values and checkbox",
			schema: `{"type":"object","required":["email","full_name","subscribe"],"properties":{
				"email":{"type":"string","description":"email address"},
				"full_name":{"type":"string","description":"full name"},
				"subscribe":{"type":"boolean","description":"subscribe to newsletter"}}}`,
			nodes: formNodes(),
			wantData: map[string]any{
				"email":     "user@example.com",
				"full_name": "Ada Lovelace",
				"subscribe": false,
			},
			wantRefs: map[string]string{"email": "e2", "full_name": "e3", "subscribe": "e4"},
		},
		{
			name: "required field absent",
			schema: `{"type":"object","required":["title","telephone"],"properties":{
				"title":{"type":"string","description":"article title"},
				"telephone":{"type":"string","description":"telephone contact number"}}}`,
			nodes:    articleNodes(),
			wantData: map[string]any{"title": "The Rise of Model-Free Extraction"},
			wantRefs: map[string]string{"title": "e2"},
			missing:  []string{"telephone"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := mustSchema(t, tc.schema)
			got := Resolve(schema, tc.nodes, Options{})

			for k, want := range tc.wantData {
				if got.Data[k] != want {
					t.Errorf("data[%q] = %#v, want %#v (field: %+v)", k, got.Data[k], want, got.Fields[k])
				}
			}
			for k, want := range tc.wantRefs {
				fr := got.Fields[k]
				if fr.Ref != want {
					t.Errorf("field[%q].Ref = %q, want %q (score %.3f, reason %q)", k, fr.Ref, want, fr.Score, fr.Reason)
				}
				if fr.Confidence == "" {
					t.Errorf("field[%q].Confidence is empty", k)
				}
			}
			if len(got.Missing) != len(tc.missing) || (len(got.Missing) > 0 && !reflect.DeepEqual(got.Missing, tc.missing)) {
				t.Errorf("missing = %v, want %v", got.Missing, tc.missing)
			}
			for _, m := range tc.missing {
				if _, present := got.Data[m]; present {
					t.Errorf("missing field %q must not appear in data", m)
				}
			}
		})
	}
}

func TestResolve_FieldsCarryRefScoreConfidence(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{
		"price":{"type":"number","description":"product price"}}}`)
	got := Resolve(schema, productNodes(), Options{})
	fr := got.Fields["price"]
	if fr.Ref == "" {
		t.Fatalf("expected a ref for price, got %+v", fr)
	}
	if fr.Score < 0.3 {
		t.Errorf("expected score >= threshold, got %.3f", fr.Score)
	}
	switch fr.Confidence {
	case "high", "medium", "low":
	default:
		t.Errorf("confidence band %q is not high/medium/low", fr.Confidence)
	}
}

func TestCoerceNumber(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"$1,299.00", 1299, true},
		{"−3.5 kg", -3.5, true},
		{"-3.5 kg", -3.5, true},
		{"call for price", 0, false},
		{"42", 42, true},
		{"1,000,000", 1000000, true},
		{"4.7 out of 5", 4.7, true},
		{"2 of 3", 2, true},
		{"", 0, false},
	}
	for _, tc := range tests {
		got, ok := coerceNumber(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("coerceNumber(%q) = %v,%v; want %v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolve_NotNumericLeavesFieldMissing(t *testing.T) {
	nodes := []observe.A11yNode{
		{Ref: "e1", Role: "region", Name: "Product", Depth: 0},
		{Ref: "e2", Role: "text", Name: "Price", Text: "call for price", Depth: 1},
	}
	schema := mustSchema(t, `{"type":"object","required":["price"],"properties":{
		"price":{"type":"number","description":"product price"}}}`)
	got := Resolve(schema, nodes, Options{})
	if _, present := got.Data["price"]; present {
		t.Fatalf("price must be missing, got %#v", got.Data["price"])
	}
	if got.Fields["price"].Reason != reasonNotNumeric {
		t.Errorf("reason = %q, want %q", got.Fields["price"].Reason, reasonNotNumeric)
	}
	if len(got.Missing) != 1 || got.Missing[0] != "price" {
		t.Errorf("missing = %v, want [price]", got.Missing)
	}
}

func TestResolve_HintSelectorBeatsName(t *testing.T) {
	nodes := []observe.A11yNode{
		{Ref: "e1", Role: "region", Name: "Product", Depth: 0},
		{Ref: "e2", Role: "text", Name: "Price", Text: "$1,299.00", Depth: 1},
		{Ref: "e3", Role: "text", Name: "Sale price", Text: "$999.00", Depth: 1},
	}
	withHint := mustSchema(t, `{"type":"object","properties":{
		"price":{"type":"number","description":"product price","x-pinchtab-hint":"role:text Sale price"}}}`)
	got := Resolve(withHint, nodes, Options{})
	if got.Fields["price"].Ref != "e3" {
		t.Fatalf("hint should select e3 (sale price), got %q (%.3f)", got.Fields["price"].Ref, got.Fields["price"].Score)
	}
	if got.Data["price"] != 999.0 {
		t.Errorf("price = %#v, want 999", got.Data["price"])
	}
	bare := mustSchema(t, `{"type":"object","properties":{
		"price":{"type":"number","x-pinchtab-hint":"Sale price"}}}`)
	if got := Resolve(bare, nodes, Options{}); got.Fields["price"].Ref != "e3" {
		t.Errorf("bare hint should select e3, got %q", got.Fields["price"].Ref)
	}
}

func TestResolve_Deterministic(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","required":["name","price","in_stock"],"properties":{
		"name":{"type":"string","description":"product name"},
		"price":{"type":"number","description":"product price"},
		"in_stock":{"type":"boolean","description":"in stock availability"}}}`)

	base := productNodes()
	first := Resolve(schema, base, Options{})
	second := Resolve(schema, base, Options{})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Resolve is not stable across runs:\n%+v\n%+v", first, second)
	}

	shuffled := make([]observe.A11yNode, len(base))
	copy(shuffled, base)
	rng := rand.New(rand.NewSource(7))
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	third := Resolve(schema, shuffled, Options{})
	if !reflect.DeepEqual(first, third) {
		t.Fatalf("Resolve differs under shuffled node order:\n%+v\n%+v", first, third)
	}
}

func TestParseSchema_UnsupportedConstructs(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		wantPath string
	}{
		{"object property", `{"type":"object","properties":{"price":{"type":"object"}}}`, "properties.price.type"},
		{"nested properties", `{"type":"object","properties":{"price":{"type":"string","properties":{"x":{"type":"string"}}}}}`, "properties.price.properties"},
		{"unknown type", `{"type":"object","properties":{"price":{"type":"decimal"}}}`, "properties.price.type"},
		{"non-object root", `{"type":"array","properties":{"price":{"type":"string"}}}`, "type"},
		{"css hint", `{"type":"object","properties":{"price":{"type":"number","x-pinchtab-hint":"css:.price"}}}`, "properties.price.x-pinchtab-hint"},
		{"xpath hint", `{"type":"object","properties":{"price":{"type":"number","x-pinchtab-hint":"xpath://span"}}}`, "properties.price.x-pinchtab-hint"},
		{"bare xpath hint", `{"type":"object","properties":{"price":{"type":"number","x-pinchtab-hint":"//span"}}}`, "properties.price.x-pinchtab-hint"},
		{"bare xpath scope", `{"type":"object","properties":{"rows":{"type":"array","x-pinchtab-scope":"(//table)[1]","items":{"type":"object","properties":{"id":{"type":"string"}}}}}}`, "properties.rows.x-pinchtab-scope"},
		{"css scope", `{"type":"object","properties":{"rows":{"type":"array","x-pinchtab-scope":"css:table","items":{"type":"object","properties":{"id":{"type":"string"}}}}}}`, "properties.rows.x-pinchtab-scope"},
		{"array without items", `{"type":"object","properties":{"tags":{"type":"array"}}}`, "properties.tags.items"},
		{"array of strings", `{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string"}}}}`, "properties.tags.items.type"},
		{"nested array", `{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{"tags":{"type":"array","items":{"type":"object","properties":{"x":{"type":"string"}}}}}}}}}`, "properties.rows.items.properties.tags.type"},
		{"negative maxItems", `{"type":"object","properties":{"rows":{"type":"array","maxItems":-1,"items":{"type":"object","properties":{"x":{"type":"string"}}}}}}`, "properties.rows.maxItems"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema([]byte(tc.doc))
			var ue *UnsupportedError
			if !errors.As(err, &ue) {
				t.Fatalf("expected *UnsupportedError, got %T (%v)", err, err)
			}
			if ue.Path != tc.wantPath {
				t.Errorf("path = %q, want %q (full: %q)", ue.Path, tc.wantPath, ue.Error())
			}
		})
	}
}

func TestParseSchema_ValidFlatSchema(t *testing.T) {
	s := mustSchema(t, `{"type":"object","required":["name"],"properties":{
		"name":{"type":"string","description":"product name"},
		"price":{"type":"number","x-pinchtab-hint":"role:text Price"},
		"in_stock":{"type":"boolean"}}}`)
	if len(s.Properties) != 3 {
		t.Fatalf("expected 3 properties, got %d", len(s.Properties))
	}
	wantOrder := []string{"in_stock", "name", "price"}
	for i, p := range s.Properties {
		if p.Name != wantOrder[i] {
			t.Errorf("property[%d] = %q, want %q", i, p.Name, wantOrder[i])
		}
	}
	var name Property
	for _, p := range s.Properties {
		if p.Name == "name" {
			name = p
		}
	}
	if !name.Required {
		t.Errorf("name should be required")
	}
}
