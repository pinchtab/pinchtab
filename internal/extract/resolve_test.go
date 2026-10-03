package extract

import (
	"reflect"
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

func TestFieldQuery_SplitsCaseBoundariesIntoWords(t *testing.T) {
	cases := map[string]string{
		"itemPrice":    "item price",
		"OrderStatus":  "order status",
		"URLPath":      "url path",
		"giftWrap":     "gift wrap",
		"item2Price":   "item2 price",
		"item_price":   "item_price",
		"order-status": "order-status",
		"gift wrap":    "gift wrap",
		"price":        "price",
	}
	for name, want := range cases {
		if got := fieldQuery(Property{Name: name}); got != want {
			t.Errorf("fieldQuery(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolve_CamelCaseKeyMatchesLikeItsSnakeTwin(t *testing.T) {
	nodes := []observe.A11yNode{
		{Ref: "e1", Role: "form", Depth: 0},
		{Ref: "e2", Role: "checkbox", Name: "Gift wrap", Checked: observe.CheckedTrue, Depth: 1},
		{Ref: "e3", Role: "textbox", Name: "Coupon code", Value: "SAVE10", Depth: 1},
	}
	for _, keys := range [][2]string{{"giftWrap", "couponCode"}, {"gift_wrap", "coupon_code"}, {"GiftWrap", "CouponCode"}} {
		schema := mustSchema(t, `{"type":"object","properties":{"`+keys[0]+`":{"type":"boolean"},"`+keys[1]+`":{"type":"string"}}}`)
		got := Resolve(schema, nodes, Options{})
		if got.Data[keys[0]] != true || got.Fields[keys[0]].Ref != "e2" {
			t.Errorf("%s = %#v %+v, want true from e2", keys[0], got.Data[keys[0]], got.Fields[keys[0]])
		}
		if got.Data[keys[1]] != "SAVE10" || got.Fields[keys[1]].Ref != "e3" {
			t.Errorf("%s = %#v %+v, want SAVE10 from e3", keys[1], got.Data[keys[1]], got.Fields[keys[1]])
		}
	}
}

func TestResolveArray_CamelCaseColumnKeyMatchesItsHeader(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{"products":{"type":"array","items":{"type":"object","properties":{
		"name":{"type":"string"},"itemPrice":{"type":"number"}}}}}}`)
	got := Resolve(schema, stockTableNodes(), Options{})
	want := []map[string]any{
		{"name": "Pen", "itemPrice": 1.5},
		{"name": "Book", "itemPrice": 7.0},
		{"name": "Lamp", "itemPrice": 19.99},
	}
	if list := items(t, got, "products"); !reflect.DeepEqual(list, want) {
		t.Fatalf("products = %v, want %v", list, want)
	}
}
