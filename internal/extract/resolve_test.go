package extract

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func wrappedLabelFormNodes(sizeControlName string) []observe.A11yNode {
	return []observe.A11yNode{
		{Ref: "e3", Role: "form", Depth: 0},
		{Ref: "e4", Role: "LabelText", Depth: 1},
		{Ref: "e5", Role: "StaticText", Name: "Name", Depth: 2},
		{Ref: "e6", Role: "textbox", Name: "Name", Value: "Ada", Depth: 2},
		{Ref: "e10", Role: "LabelText", Depth: 1},
		{Ref: "e11", Role: "StaticText", Name: "Size", Depth: 2},
		{Ref: "e12", Role: "combobox", Name: sizeControlName, Value: "M", Depth: 2},
		{Ref: "e15", Role: "StaticText", Name: "Price $9.99", Depth: 1},
	}
}

func TestResolve_WrappedLabelReadsTheControlValue(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{
		"name":{"type":"string"},
		"size":{"type":"string"}}}`)
	for _, sizeName := range []string{"Size", "Size M"} {
		got := Resolve(schema, wrappedLabelFormNodes(sizeName), Options{})
		want := map[string]struct{ value, ref string }{
			"name": {"Ada", "e6"},
			"size": {"M", "e12"},
		}
		for field, w := range want {
			fr := got.Fields[field]
			if got.Data[field] != w.value || fr.Ref != w.ref || fr.Source != sourceValue {
				t.Errorf("combobox named %q: %s = %#v %+v, want %q from %s via value", sizeName, field, got.Data[field], fr, w.value, w.ref)
			}
		}
	}
}

func TestResolve_UnlabelledStaticTextStaysOnTheText(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{"price":{"type":"string","description":"price"}}}`)
	got := Resolve(schema, wrappedLabelFormNodes("Size"), Options{})
	if got.Data["price"] != "Price $9.99" || got.Fields["price"].Ref != "e15" {
		t.Errorf("price = %#v %+v, want Price $9.99 from e15", got.Data["price"], got.Fields["price"])
	}
}

func TestResolve_RefHintIsNotRedirectedToTheLabelledControl(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string","x-pinchtab-hint":"ref:e5"}}}`)
	got := Resolve(schema, wrappedLabelFormNodes("Size"), Options{})
	if got.Data["name"] != "Name" || got.Fields["name"].Ref != "e5" {
		t.Errorf("name = %#v %+v, want the hinted label text from e5", got.Data["name"], got.Fields["name"])
	}
}
