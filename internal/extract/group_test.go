package extract

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

const productListSchema = `{"type":"object","required":["products"],"properties":{
	"products":{"type":"array","items":{"type":"object","required":["name","price"],"properties":{
		"name":{"type":"string","description":"product name","x-pinchtab-hint":"role:heading"},
		"price":{"type":"number","description":"product price"},
		"rating":{"type":"number","description":"rating out of 5"}}}}}}`

const ordersSchema = `{"type":"object","properties":{
	"orders":{"type":"array","items":{"type":"object","properties":{
		"order":{"type":"integer","description":"order number"},
		"customer":{"type":"string","description":"customer name"},
		"total":{"type":"number","description":"order total"}}}}}}`

func items(t *testing.T, got Result, prop string) []map[string]any {
	t.Helper()
	list, ok := got.Data[prop].([]map[string]any)
	if !ok {
		t.Fatalf("data[%q] = %#v (field %+v), want an array", prop, got.Data[prop], got.Fields[prop])
	}
	return list
}

func TestResolveArray_ProductGridFixture(t *testing.T) {
	got := Resolve(mustSchema(t, productListSchema), loadSnapshot(t, "extract-list.json"), Options{})
	list := items(t, got, "products")
	want := []map[string]any{
		{"name": "Sony WH-1000XM5", "price": 399.0, "rating": 4.7},
		{"name": "Bose QuietComfort Ultra", "price": 429.0, "rating": 4.5},
		{"name": "Apple AirPods Max", "price": 549.0, "rating": 4.6},
		{"name": "Sennheiser Momentum 4", "price": 349.95, "rating": 4.4},
		{"name": "Bowers & Wilkins Px8", "price": 699.0, "rating": 4.3},
		{"name": "Beats Studio Pro", "price": 349.99, "rating": 4.2},
	}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("products =\n%v\nwant\n%v", list, want)
	}
	fr := got.Fields["products"]
	if fr.Ref != "e20" {
		t.Errorf("container ref = %q, want e20 (products region)", fr.Ref)
	}
	wantRefs := []string{"e21", "e28", "e35", "e42", "e49", "e56"}
	for i, ir := range fr.Items {
		if ir.Ref != wantRefs[i] {
			t.Errorf("item %d ref = %q, want %q", i, ir.Ref, wantRefs[i])
		}
	}
	if fr.Items[0].Fields["price"].Ref != "e24" || fr.Items[1].Fields["name"].Ref != "e29" {
		t.Errorf("per-item refs = %+v / %+v", fr.Items[0].Fields, fr.Items[1].Fields)
	}
	if fr.Truncated || len(got.Missing) != 0 {
		t.Errorf("truncated=%v missing=%v", fr.Truncated, got.Missing)
	}
}

func TestResolveArray_TableFixture(t *testing.T) {
	got := Resolve(mustSchema(t, ordersSchema), loadSnapshot(t, "extract-list.json"), Options{})
	list := items(t, got, "orders")
	want := []map[string]any{
		{"order": int64(1001), "customer": "Alice Johnson", "total": 399.0},
		{"order": int64(1002), "customer": "Bob Smith", "total": 429.0},
		{"order": int64(1003), "customer": "Carol White", "total": 549.0},
		{"order": int64(1004), "customer": "Dan Brown", "total": 349.95},
		{"order": int64(1005), "customer": "Eve Black", "total": 699.0},
	}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("orders =\n%v\nwant\n%v", list, want)
	}
	fr := got.Fields["orders"]
	if fr.Ref != "e63" && fr.Ref != "e64" {
		t.Errorf("container ref = %q, want the table or its body rowgroup", fr.Ref)
	}
	if fr.Items[0].Ref != "e72" || fr.Items[0].Fields["customer"].Ref != "e75" || fr.Items[4].Fields["total"].Ref != "e105" {
		t.Errorf("row refs = %+v", fr.Items)
	}
}

func searchNodes() []observe.A11yNode {
	return []observe.A11yNode{
		{Ref: "e1", Role: "main", Name: "Results", Depth: 0},
		{Ref: "e2", Role: "list", Depth: 1},
		{Ref: "e3", Role: "listitem", Depth: 2},
		{Ref: "e4", Role: "link", Name: "Result title", Text: "Go by Example", Depth: 3},
		{Ref: "e5", Role: "text", Name: "Snippet", Text: "Hands-on introduction to Go.", Depth: 3},
		{Ref: "e6", Role: "listitem", Depth: 2},
		{Ref: "e7", Role: "link", Name: "Result title", Text: "A Tour of Go", Depth: 3},
		{Ref: "e8", Role: "text", Name: "Snippet", Text: "Interactive tour.", Depth: 3},
		{Ref: "e9", Role: "listitem", Depth: 2},
		{Ref: "e10", Role: "link", Name: "Result title", Text: "Effective Go", Depth: 3},
		{Ref: "e11", Role: "listitem", Depth: 2},
		{Ref: "e12", Role: "link", Name: "Result title", Text: "Go FAQ", Depth: 3},
		{Ref: "e13", Role: "text", Name: "Snippet", Text: "Frequently asked questions.", Depth: 3},
	}
}

const searchSchema = `{"type":"object","properties":{
	"results":{"type":"array","items":{"type":"object","properties":{
		"title":{"type":"string","description":"result title"},
		"snippet":{"type":"string","x-pinchtab-hint":"role:text Snippet"}}}}}}`

func TestResolveArray_SearchResultsList(t *testing.T) {
	got := Resolve(mustSchema(t, searchSchema), searchNodes(), Options{})
	list := items(t, got, "results")
	want := []map[string]any{
		{"title": "Go by Example", "snippet": "Hands-on introduction to Go."},
		{"title": "A Tour of Go", "snippet": "Interactive tour."},
		{"title": "Effective Go"},
		{"title": "Go FAQ", "snippet": "Frequently asked questions."},
	}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("results =\n%v\nwant\n%v", list, want)
	}
	fr := got.Fields["results"]
	if fr.Ref != "e2" || fr.Items[2].Ref != "e9" || fr.Items[2].Fields["title"].Ref != "e10" {
		t.Errorf("refs = %q %+v", fr.Ref, fr.Items)
	}
}

func TestResolveArray_FieldAbsentFromOneItemStaysMissing(t *testing.T) {
	got := Resolve(mustSchema(t, searchSchema), searchNodes(), Options{})
	third := got.Fields["results"].Items[2]
	if _, present := items(t, got, "results")[2]["snippet"]; present {
		t.Fatalf("item 3 has no snippet node but resolved one: %+v", third.Fields["snippet"])
	}
	if third.Fields["snippet"].Reason != reasonNoMatch || third.Fields["snippet"].Ref != "" {
		t.Errorf("item 3 snippet = %+v, want no_match without a ref", third.Fields["snippet"])
	}
}

func TestResolveArray_AmbiguousGroupsPickBestResolving(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	products := Resolve(mustSchema(t, productListSchema), nodes, Options{})
	if products.Fields["products"].Ref != "e20" {
		t.Errorf("products chose %q over the products region e20", products.Fields["products"].Ref)
	}

	menu := mustSchema(t, `{"type":"object","properties":{
		"menu":{"type":"array","items":{"type":"object","properties":{
			"link":{"type":"string","x-pinchtab-hint":"role:link"}}}}}}`)
	got := Resolve(menu, nodes, Options{})
	if got.Fields["menu"].Ref != "e2" {
		t.Fatalf("menu chose %q over the nav list e2 (%+v)", got.Fields["menu"].Ref, got.Fields["menu"])
	}
	want := []map[string]any{{"link": "Home"}, {"link": "Shop"}, {"link": "About"}, {"link": "Contact"}}
	if list := items(t, got, "menu"); !reflect.DeepEqual(list, want) {
		t.Errorf("menu = %v, want %v", list, want)
	}
}

func TestResolveArray_ScopeForcesContainer(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	scoped := mustSchema(t, `{"type":"object","properties":{
		"links":{"type":"array","x-pinchtab-scope":"role:list","items":{"type":"object","properties":{
			"name":{"type":"string","x-pinchtab-hint":"role:link"}}}}}}`)
	got := Resolve(scoped, nodes, Options{})
	if got.Fields["links"].Ref != "e2" {
		t.Fatalf("scope should force the nav list e2, got %q (%+v)", got.Fields["links"].Ref, got.Fields["links"])
	}
	if list := items(t, got, "links"); len(list) != 4 {
		t.Errorf("links = %v, want 4 nav items", list)
	}

	for _, scope := range []string{"ref:e63", "ref:e64", "e63", "e64"} {
		byRef := mustSchema(t, `{"type":"object","properties":{
			"rows":{"type":"array","x-pinchtab-scope":"`+scope+`","items":{"type":"object","properties":{
				"order":{"type":"integer","description":"order number"}}}}}}`)
		got = Resolve(byRef, nodes, Options{})
		if got.Fields["rows"].Ref != "e64" || len(items(t, got, "rows")) != 5 {
			t.Errorf("scope %s = %+v data=%v, want the 5 rows under the table", scope, got.Fields["rows"], got.Data["rows"])
		}
	}

	absent := mustSchema(t, `{"type":"object","required":["rows"],"properties":{
		"rows":{"type":"array","x-pinchtab-scope":"ref:e999","items":{"type":"object","properties":{
			"order":{"type":"integer"}}}}}}`)
	got = Resolve(absent, nodes, Options{})
	if _, present := got.Data["rows"]; present {
		t.Fatalf("rows must be missing when the scope resolves nothing")
	}
	if got.Fields["rows"].Reason != reasonScopeNotFound || !reflect.DeepEqual(got.Missing, []string{"rows"}) {
		t.Errorf("field = %+v missing = %v", got.Fields["rows"], got.Missing)
	}
}

func TestResolveArray_MaxItemsCapsAndReportsTruncated(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	capped := mustSchema(t, `{"type":"object","properties":{
		"products":{"type":"array","maxItems":2,"items":{"type":"object","properties":{
			"name":{"type":"string","x-pinchtab-hint":"role:heading"}}}}}}`)
	got := Resolve(capped, nodes, Options{})
	if n := len(items(t, got, "products")); n != 2 || !got.Fields["products"].Truncated {
		t.Errorf("schema maxItems: len=%d truncated=%v", n, got.Fields["products"].Truncated)
	}

	got = Resolve(mustSchema(t, productListSchema), nodes, Options{MaxItems: 4})
	if n := len(items(t, got, "products")); n != 4 || !got.Fields["products"].Truncated {
		t.Errorf("Options.MaxItems: len=%d truncated=%v", n, got.Fields["products"].Truncated)
	}

	got = Resolve(mustSchema(t, productListSchema), nodes, Options{MaxItems: 6})
	if n := len(items(t, got, "products")); n != 6 || got.Fields["products"].Truncated {
		t.Errorf("exact cap: len=%d truncated=%v", n, got.Fields["products"].Truncated)
	}
}

func TestResolveArray_MinItemsLeavesPropertyMissing(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","required":["products"],"properties":{
		"products":{"type":"array","minItems":10,"items":{"type":"object","properties":{
			"name":{"type":"string","x-pinchtab-hint":"role:heading"}}}}}}`)
	got := Resolve(schema, loadSnapshot(t, "extract-list.json"), Options{})
	if _, present := got.Data["products"]; present {
		t.Fatalf("products must be missing below minItems")
	}
	if got.Fields["products"].Reason != reasonTooFewItems || len(got.Fields["products"].Items) != 0 {
		t.Errorf("field = %+v", got.Fields["products"])
	}
}

func TestResolveArray_NoRepeatedGroup(t *testing.T) {
	got := Resolve(mustSchema(t, productListSchema), productNodes(), Options{})
	if _, present := got.Data["products"]; present {
		t.Fatalf("flat product page has no repeated group")
	}
	if got.Fields["products"].Reason != reasonNoRepeatedGroup || !reflect.DeepEqual(got.Missing, []string{"products"}) {
		t.Errorf("field = %+v missing = %v", got.Fields["products"], got.Missing)
	}
}

func TestResolveArray_Deterministic(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	first := Resolve(mustSchema(t, productListSchema), nodes, Options{})
	second := Resolve(mustSchema(t, productListSchema), nodes, Options{})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("array resolution is not stable:\n%+v\n%+v", first, second)
	}
}

func TestView_ChildrenAndSubtreeFromDepth(t *testing.T) {
	v := newView(searchNodes())
	if end := v.subtreeEnd(v.index["e3"]); end != v.index["e6"] {
		t.Errorf("subtree of e3 ends at %d, want index of e6 %d", end, v.index["e6"])
	}
	var refs []string
	for _, c := range v.children(v.index["e2"]) {
		refs = append(refs, v.nodes[c].Ref)
	}
	if !reflect.DeepEqual(refs, []string{"e3", "e6", "e9", "e11"}) {
		t.Errorf("children of e2 = %v", refs)
	}
	if got := v.subtree(v.index["e6"]).nodes; len(got) != 3 || got[0].Ref != "e6" || got[2].Ref != "e8" {
		t.Errorf("subtree of e6 = %v", got)
	}
	var anc []string
	for _, a := range v.ancestors(v.index["e12"]) {
		anc = append(anc, v.nodes[a].Ref)
	}
	if !reflect.DeepEqual(anc, []string{"e11", "e2", "e1"}) {
		t.Errorf("ancestors of e12 = %v", anc)
	}
}

func TestResolveArray_GroupResolvingNoFieldIsNoRepeatedGroup(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{
		"results":{"type":"array","items":{"type":"object","properties":{
			"price":{"type":"number","x-pinchtab-hint":"role:link"}}}}}}`)
	got := Resolve(schema, searchNodes(), Options{})
	if _, present := got.Data["results"]; present {
		t.Fatalf("a group whose items resolve no field must not be returned: %v", got.Data["results"])
	}
	if got.Fields["results"].Reason != reasonNoRepeatedGroup || got.Fields["results"].Ref != "" {
		t.Errorf("field = %+v", got.Fields["results"])
	}
}

func TestResolveArray_ItemNodeItselfCarriesTheField(t *testing.T) {
	nodes := []observe.A11yNode{
		{Ref: "e1", Role: "list", Depth: 0},
		{Ref: "e2", Role: "link", Name: "Home", Depth: 1},
		{Ref: "e3", Role: "link", Name: "Shop", Depth: 1},
		{Ref: "e4", Role: "link", Name: "About", Depth: 1},
		{Ref: "e5", Role: "link", Name: "Contact", Depth: 1},
	}
	schema := mustSchema(t, `{"type":"object","properties":{
		"menu":{"type":"array","items":{"type":"object","properties":{
			"title":{"type":"string","x-pinchtab-hint":"role:link"}}}}}}`)
	got := Resolve(schema, nodes, Options{})
	want := []map[string]any{{"title": "Home"}, {"title": "Shop"}, {"title": "About"}, {"title": "Contact"}}
	if list := items(t, got, "menu"); !reflect.DeepEqual(list, want) {
		t.Fatalf("menu = %v, want %v (%+v)", list, want, got.Fields["menu"])
	}
	if fr := got.Fields["menu"]; fr.Ref != "e1" || fr.Items[1].Ref != "e3" || fr.Items[1].Fields["title"].Ref != "e3" {
		t.Errorf("refs = %+v", fr)
	}
}

func TestResolveArray_CapReachedExactlyWithTrailingEmptyItemIsNotTruncated(t *testing.T) {
	nodes := append(searchNodes(), observe.A11yNode{Ref: "e14", Role: "listitem", Depth: 2}, observe.A11yNode{Ref: "e15", Role: "text", Name: "Sponsored", Depth: 3})
	schema := mustSchema(t, `{"type":"object","properties":{
		"results":{"type":"array","maxItems":4,"items":{"type":"object","properties":{
			"title":{"type":"string","x-pinchtab-hint":"role:link"}}}}}}`)
	got := Resolve(schema, nodes, Options{})
	if n := len(items(t, got, "results")); n != 4 || got.Fields["results"].Truncated {
		t.Errorf("len=%d truncated=%v, want 4 items and no truncation when only an empty item follows", n, got.Fields["results"].Truncated)
	}
}

func TestResolve_RequestScopeConfinesTheWholeSchemaToOneSubtree(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	schema := mustSchema(t, `{"type":"object","properties":{
		"entries":{"type":"array","items":{"type":"object","properties":{
			"amount":{"type":"number","description":"price"}}}}}}`)

	unscoped := Resolve(schema, nodes, Options{})
	if n := len(items(t, unscoped, "entries")); n != 6 {
		t.Fatalf("unscoped entries = %d, want the 6-product grid so the scope below is what changes the answer", n)
	}

	scoped, err := schema.WithScope("role:table")
	if err != nil {
		t.Fatalf("WithScope: %v", err)
	}
	got := Resolve(scoped, nodes, Options{})
	if n := len(items(t, got, "entries")); n != 5 {
		t.Fatalf("scoped entries = %d, want the 5 table rows (%+v)", n, got.Fields["entries"])
	}
	if ref := got.Fields["entries"].Ref; ref != "e63" && ref != "e64" {
		t.Errorf("scoped container ref = %q, want the table or its body rowgroup", ref)
	}
}

func TestResolve_RequestScopeThatMatchesNothingLeavesEveryFieldUnresolved(t *testing.T) {
	schema, err := mustSchema(t, `{"type":"object","required":["name"],"properties":{
		"name":{"type":"string","description":"product name"},
		"price":{"type":"number","description":"product price"}}}`).WithScope("ref:e999")
	if err != nil {
		t.Fatalf("WithScope: %v", err)
	}
	got := Resolve(schema, productNodes(), Options{})
	if len(got.Data) != 0 {
		t.Errorf("data = %v, want nothing outside a scope that does not exist", got.Data)
	}
	for _, name := range []string{"name", "price"} {
		if got.Fields[name].Reason != reasonScopeNotFound {
			t.Errorf("field %q reason = %q, want %q", name, got.Fields[name].Reason, reasonScopeNotFound)
		}
	}
	if !reflect.DeepEqual(got.Missing, []string{"name"}) {
		t.Errorf("missing = %v, want the required field", got.Missing)
	}
}

func TestSchemaWithScopeRefusesABrowserSelectorNamingTheScope(t *testing.T) {
	_, err := mustSchema(t, productListSchema).WithScope("css:table")
	var ue *UnsupportedError
	if !errors.As(err, &ue) || ue.Path != "scope" {
		t.Fatalf("err = %v (%T), want an UnsupportedError at path scope", err, err)
	}
}

func TestRequestScopeAcceptsABareRefLikeTheFieldsTablePrintsIt(t *testing.T) {
	nodes := loadSnapshot(t, "extract-list.json")
	schema := mustSchema(t, `{"type":"object","properties":{
		"entries":{"type":"array","items":{"type":"object","properties":{
			"amount":{"type":"number","description":"price"}}}}}}`)
	for _, scope := range []string{"ref:e64", "e64"} {
		scoped, err := schema.WithScope(scope)
		if err != nil {
			t.Fatalf("WithScope(%q): %v", scope, err)
		}
		if n := len(items(t, Resolve(scoped, nodes, Options{}), "entries")); n != 5 {
			t.Errorf("scope %q entries = %d, want the 5 table rows", scope, n)
		}
	}
}

func TestHintAcceptsABareRef(t *testing.T) {
	schema := mustSchema(t, `{"type":"object","properties":{
		"colour":{"type":"string","x-pinchtab-hint":"e5"}}}`)
	got := Resolve(schema, productNodes(), Options{})
	if got.Fields["colour"].Ref != "e5" || got.Data["colour"] != "Midnight Black" {
		t.Errorf("colour = %+v data=%v, want the e5 node's text", got.Fields["colour"], got.Data["colour"])
	}
}
