package selector

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestParse_ExplicitPrefixes(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
		value string
	}{
		{"css:#login", KindCSS, "#login"},
		{"css:.btn.primary", KindCSS, ".btn.primary"},
		{"css:div > span", KindCSS, "div > span"},
		{"css:input[type=text]", KindCSS, "input[type=text]"},
		{"css:*", KindCSS, "*"},

		{"xpath://div[@id='main']", KindXPath, "//div[@id='main']"},
		{"xpath:(//button)[1]", KindXPath, "(//button)[1]"},
		{"xpath://a[contains(@href,'login')]", KindXPath, "//a[contains(@href,'login')]"},

		{"text:Submit", KindText, "Submit"},
		{"text:Log in", KindText, "Log in"},
		{"text:", KindText, ""},
		{"text:with:colon", KindText, "with:colon"},

		{"find:login button", KindSemantic, "login button"},
		{"semantic:login button", KindSemantic, "login button"},
		{"find:the search input field", KindSemantic, "the search input field"},
		{"find:", KindSemantic, ""},

		{"role:button Save", KindRole, "button Save"},
		{"label:Email", KindLabel, "Email"},
		{"placeholder:Search", KindPlaceholder, "Search"},
		{"alt:Product photo", KindAlt, "Product photo"},
		{"title:Close", KindTitle, "Close"},
		{"testid:submit-button", KindTestID, "submit-button"},
		{"first:button", KindFirst, "button"},
		{"last:text:Submit", KindLast, "text:Submit"},
		{"nth:2:role:button Save", KindNth, "2:role:button Save"},

		{"ref:e5", KindRef, "e5"},
		{"ref:e0", KindRef, "e0"},
		{"ref:e99999", KindRef, "e99999"},
		// ref: prefix with non-standard value (still accepted as ref)
		{"ref:something", KindRef, "something"},
	}
	for _, tt := range tests {
		s := Parse(tt.input)
		if s.Kind != tt.kind {
			t.Errorf("Parse(%q).Kind = %q, want %q", tt.input, s.Kind, tt.kind)
		}
		if s.Value != tt.value {
			t.Errorf("Parse(%q).Value = %q, want %q", tt.input, s.Value, tt.value)
		}
	}
}

func TestParse_AutoDetect(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
		value string
	}{
		{"e0", KindRef, "e0"},
		{"e5", KindRef, "e5"},
		{"e42", KindRef, "e42"},
		{"e123", KindRef, "e123"},
		{"e99999", KindRef, "e99999"},

		{"#login", KindCSS, "#login"},
		{"#my-id", KindCSS, "#my-id"},

		{".btn", KindCSS, ".btn"},
		{".btn.primary", KindCSS, ".btn.primary"},

		{"[type=file]", KindCSS, "[type=file]"},
		{"[data-testid='foo']", KindCSS, "[data-testid='foo']"},

		{"button.submit", KindCSS, "button.submit"},
		{"div > span", KindCSS, "div > span"},
		{"input[name='email']", KindCSS, "input[name='email']"},
		{"ul li:first-child", KindCSS, "ul li:first-child"},
		{"a:hover", KindCSS, "a:hover"},

		{"//div[@class='main']", KindXPath, "//div[@class='main']"},
		{"//a", KindXPath, "//a"},

		{"(//button)[1]", KindXPath, "(//button)[1]"},
		{"(//div[@class='x'])[last()]", KindXPath, "(//div[@class='x'])[last()]"},

		// Bare tag names → CSS (backward compat)
		{"button", KindCSS, "button"},
		{"div", KindCSS, "div"},
		{"input", KindCSS, "input"},

		// Words that start with 'e' but are NOT refs
		{"embed", KindCSS, "embed"},
		{"email", KindCSS, "email"},
		{"element", KindCSS, "element"},
	}
	for _, tt := range tests {
		s := Parse(tt.input)
		if s.Kind != tt.kind {
			t.Errorf("Parse(%q).Kind = %q, want %q", tt.input, s.Kind, tt.kind)
		}
		if s.Value != tt.value {
			t.Errorf("Parse(%q).Value = %q, want %q", tt.input, s.Value, tt.value)
		}
	}
}

func TestParse_Empty(t *testing.T) {
	s := Parse("")
	if !s.IsEmpty() {
		t.Error("Parse(\"\") should be empty")
	}
	if s.Kind != KindNone {
		t.Errorf("Parse(\"\").Kind = %q, want %q", s.Kind, KindNone)
	}
}

func TestParse_WhitespaceOnly(t *testing.T) {
	for _, ws := range []string{" ", "   ", "\t", "\n", " \t\n "} {
		s := Parse(ws)
		if !s.IsEmpty() {
			t.Errorf("Parse(%q) should be empty", ws)
		}
	}
}

func TestParse_WhitespaceTrimming(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
		value string
	}{
		{"  e5  ", KindRef, "e5"},
		{" #login ", KindCSS, "#login"},
		{"\tcss:.btn\t", KindCSS, ".btn"},
		{" xpath://div ", KindXPath, "//div"},
		{" text:Submit ", KindText, "Submit"},
		{" find:login btn ", KindSemantic, "login btn"},
		{" ref:e42 ", KindRef, "e42"},
	}
	for _, tt := range tests {
		s := Parse(tt.input)
		if s.Kind != tt.kind {
			t.Errorf("Parse(%q).Kind = %q, want %q", tt.input, s.Kind, tt.kind)
		}
		if s.Value != tt.value {
			t.Errorf("Parse(%q).Value = %q, want %q", tt.input, s.Value, tt.value)
		}
	}
}

func TestParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  Kind
		value string
	}{
		{
			name:  "prefix with colon in value",
			input: "text:Click here: now",
			kind:  KindText,
			value: "Click here: now",
		},
		{
			name:  "css prefix with complex selector",
			input: "css:div.container > ul > li:nth-child(2n+1)",
			kind:  KindCSS,
			value: "div.container > ul > li:nth-child(2n+1)",
		},
		{
			name:  "xpath with predicates",
			input: "xpath://div[contains(@class,'active') and @data-visible='true']",
			kind:  KindXPath,
			value: "//div[contains(@class,'active') and @data-visible='true']",
		},
		{
			name:  "single character e is not a ref",
			input: "e",
			kind:  KindCSS,
			value: "e",
		},
		{
			name:  "e followed by non-digit",
			input: "eX",
			kind:  KindCSS,
			value: "eX",
		},
		{
			name:  "E uppercase is not a ref",
			input: "E5",
			kind:  KindCSS,
			value: "E5",
		},
		{
			name:  "e with mixed chars",
			input: "e5x",
			kind:  KindCSS,
			value: "e5x",
		},
		{
			name:  "unknown prefix treated as CSS",
			input: "bogus:something",
			kind:  KindCSS,
			value: "bogus:something",
		},
		{
			name:  "just a colon",
			input: ":",
			kind:  KindCSS,
			value: ":",
		},
		{
			name:  "css prefix empty value",
			input: "css:",
			kind:  KindCSS,
			value: "",
		},
		{
			name:  "ref prefix empty value",
			input: "ref:",
			kind:  KindRef,
			value: "",
		},
		{
			name:  "very long ref",
			input: "e1234567890",
			kind:  KindRef,
			value: "e1234567890",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Parse(tt.input)
			if s.Kind != tt.kind {
				t.Errorf("Parse(%q).Kind = %q, want %q", tt.input, s.Kind, tt.kind)
			}
			if s.Value != tt.value {
				t.Errorf("Parse(%q).Value = %q, want %q", tt.input, s.Value, tt.value)
			}
		})
	}
}

func TestIsRef(t *testing.T) {
	refs := []string{"e0", "e5", "e42", "e123", "e9999", "e1234567890"}
	for _, r := range refs {
		if !IsRef(r) {
			t.Errorf("IsRef(%q) = false, want true", r)
		}
	}

	nonRefs := []string{
		"", "e", "E5", "ex5", "e5x", "embed", "email", "element",
		"#e5", "ref:e5", "e-5", "e 5", "e.5", "5e", "ee5",
		"E0", "e", " e5", "e5 ",
	}
	for _, r := range nonRefs {
		if IsRef(r) {
			t.Errorf("IsRef(%q) = true, want false", r)
		}
	}
}

func TestSelector_String(t *testing.T) {
	tests := []struct {
		sel  Selector
		want string
	}{
		{Selector{KindRef, "e5"}, "e5"},
		{Selector{KindRef, "e0"}, "e0"},
		{Selector{KindCSS, "#login"}, "css:#login"},
		{Selector{KindCSS, ".btn"}, "css:.btn"},
		{Selector{KindCSS, "div > span"}, "css:div > span"},
		{Selector{KindXPath, "//div"}, "xpath://div"},
		{Selector{KindXPath, "(//button)[1]"}, "xpath:(//button)[1]"},
		{Selector{KindText, "Submit"}, "text:Submit"},
		{Selector{KindText, "with:colon"}, "text:with:colon"},
		{Selector{KindSemantic, "login button"}, "find:login button"},
		{Selector{KindRole, "button Save"}, "role:button Save"},
		{Selector{KindLabel, "Email"}, "label:Email"},
		{Selector{KindPlaceholder, "Search"}, "placeholder:Search"},
		{Selector{KindAlt, "Logo"}, "alt:Logo"},
		{Selector{KindTitle, "Close"}, "title:Close"},
		{Selector{KindTestID, "submit"}, "testid:submit"},
		{Selector{KindFirst, "button"}, "first:button"},
		{Selector{KindLast, "text:Submit"}, "last:text:Submit"},
		{Selector{KindNth, "1:button"}, "nth:1:button"},
		{Selector{KindNone, ""}, ""},
		{Selector{KindNone, "something"}, "something"},
	}
	for _, tt := range tests {
		if got := tt.sel.String(); got != tt.want {
			t.Errorf("Selector{%s, %q}.String() = %q, want %q", tt.sel.Kind, tt.sel.Value, got, tt.want)
		}
	}
}

func TestSelector_IsEmpty(t *testing.T) {
	if !(Selector{}).IsEmpty() {
		t.Error("zero-value Selector should be empty")
	}
	if !(Selector{Kind: KindCSS, Value: ""}).IsEmpty() {
		t.Error("Selector with empty Value should be empty")
	}
	if (Selector{Kind: KindRef, Value: "e5"}).IsEmpty() {
		t.Error("Selector with value should not be empty")
	}
}

func TestSelector_Validate(t *testing.T) {
	valid := []Selector{
		{KindRef, "e5"},
		{KindCSS, "#login"},
		{KindXPath, "//div"},
		{KindText, "Submit"},
		{KindSemantic, "login button"},
		{KindRole, "button Save"},
		{KindLabel, "Email"},
		{KindPlaceholder, "Search"},
		{KindAlt, "Logo"},
		{KindTitle, "Close"},
		{KindTestID, "submit"},
		{KindFirst, "button"},
		{KindLast, "button"},
		{KindNth, "1:button"},
	}
	for _, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", s, err)
		}
	}

	if err := (Selector{}).Validate(); err == nil {
		t.Error("Validate(empty) should fail")
	}
	if err := (Selector{Kind: KindCSS}).Validate(); err == nil {
		t.Error("Validate(kind=css, value='') should fail")
	}
	if err := (Selector{Kind: "bogus", Value: "x"}).Validate(); err == nil {
		t.Error("Validate(bogus kind) should fail")
	}
	if err := (Selector{Kind: Kind("unknown"), Value: "x"}).Validate(); err == nil {
		t.Error("Validate(unknown kind) should fail")
	}
}

func TestSelector_SemanticQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"find:login button", "login button", true},
		{"semantic:login button", "login button", true},
		{"text:Submit", "", false},
		{"role:button Save", "role:button Save", true},
		{"label:Email", "label:Email", true},
		{"placeholder:Search", "placeholder:Search", true},
		{"alt:Logo", "alt:Logo", true},
		{"title:Close", "title:Close", true},
		{"testid:submit", "testid:submit", true},
		{"first:text:Submit", "", false},
		{"last:role:button Save", "last:role:button Save", true},
		{"nth:2:label:Email", "nth:3:label:Email", true},
		{"first:button", "", false},
		{"last:css:button", "", false},
		{"nth:2:button", "", false},
		{"nth:-1:text:Submit", "", false},
		{"css:button", "", false},
		{"e1", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := Parse(tt.input).SemanticQuery()
			if ok != tt.ok || got != tt.want {
				t.Fatalf("SemanticQuery() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPositionalWrapperOverSemanticFormReachesTheMatcherWithItsIndex(t *testing.T) {
	semanticPrefixes := []string{}
	for _, pk := range prefixKinds {
		if rawSelectorCanUseSemantic(pk.Prefix + "Save") {
			semanticPrefixes = append(semanticPrefixes, pk.Prefix)
		}
	}
	if len(semanticPrefixes) == 0 {
		t.Fatal("no prefix routes to the semantic matcher, so this guard checked nothing")
	}

	for _, prefix := range semanticPrefixes {
		bare := prefix + "Save"
		for _, tc := range []struct{ wrapped, want string }{
			{"first:" + bare, "first:" + bare},
			{"last:" + bare, "last:" + bare},
			{"nth:0:" + bare, "nth:1:" + bare},
			{"nth:2:" + bare, "nth:3:" + bare},
		} {
			query, ok := Parse(tc.wrapped).SemanticQuery()
			if !ok {
				t.Errorf("%s no longer routes to the semantic matcher, so the wrapper it carries is never applied", tc.wrapped)
				continue
			}
			if query != tc.want {
				t.Errorf("SemanticQuery(%q) = %q, want %q: the wrapper must reach the matcher with its index, translated into the one-based nth the matcher publishes", tc.wrapped, query, tc.want)
			}
		}
	}

	for _, prefix := range []string{"css:", "xpath:", "text:"} {
		bare := prefix + "Save"
		for _, wrapped := range []string{bare, "first:" + bare, "last:" + bare, "nth:2:" + bare} {
			if _, ok := Parse(wrapped).SemanticQuery(); ok {
				t.Errorf("%s routes to the semantic matcher, so it no longer indexes in document order browser-side as docs/commands.md promises for this kind", wrapped)
			}
		}
	}
}

func TestParse_Roundtrip(t *testing.T) {
	inputs := []string{
		"e5",
		"e0",
		"e99999",
		"css:#login",
		"css:.btn.primary",
		"css:div > span",
		"xpath://div[@id='x']",
		"xpath:(//button)[1]",
		"text:Submit Order",
		"text:with:colon:in:value",
		"find:the big red button",
		"semantic:the big red button",
		"role:button Save",
		"label:Email",
		"placeholder:Search",
		"alt:Logo",
		"title:Close",
		"testid:submit",
		"first:button",
		"last:text:Submit",
		"nth:1:role:button Save",
	}
	for _, input := range inputs {
		s := Parse(input)
		rt := Parse(s.String())
		if rt.Kind != s.Kind || rt.Value != s.Value {
			t.Errorf("roundtrip failed: %q → %+v → %q → %+v", input, s, s.String(), rt)
		}
	}
}

func TestParse_PrefixPriority(t *testing.T) {
	s := Parse("css://div")
	if s.Kind != KindCSS {
		t.Errorf("Parse(\"css://div\").Kind = %q, want css", s.Kind)
	}
	if s.Value != "//div" {
		t.Errorf("Parse(\"css://div\").Value = %q, want \"//div\"", s.Value)
	}

	s = Parse("ref:embed")
	if s.Kind != KindRef {
		t.Errorf("Parse(\"ref:embed\").Kind = %q, want ref", s.Kind)
	}
	if s.Value != "embed" {
		t.Errorf("Parse(\"ref:embed\").Value = %q, want \"embed\"", s.Value)
	}

	s = Parse("text:#login")
	if s.Kind != KindText {
		t.Errorf("Parse(\"text:#login\").Kind = %q, want text", s.Kind)
	}
	if s.Value != "#login" {
		t.Errorf("Parse(\"text:#login\").Value = %q, want \"#login\"", s.Value)
	}

	s = Parse("xpath:e5")
	if s.Kind != KindXPath {
		t.Errorf("Parse(\"xpath:e5\").Kind = %q, want xpath", s.Kind)
	}
}

// The nth grammar is owned here; the bridge resolver and SemanticQuery both
// depend on this split agreeing.
func TestParseNth(t *testing.T) {
	index, raw, err := ParseNth("2:role:button Save")
	if err != nil {
		t.Fatalf("ParseNth returned error: %v", err)
	}
	if index != 2 || raw != "role:button Save" {
		t.Fatalf("got index=%d raw=%q, want 2 and a role selector", index, raw)
	}

	if _, _, err := ParseNth("0:button"); err != nil {
		t.Errorf("zero index should be valid, got %v", err)
	}
	if _, _, err := ParseNth("-1:button"); err == nil {
		t.Error("expected negative index to fail")
	}
	if _, _, err := ParseNth("button"); err == nil {
		t.Error("expected missing nested selector to fail")
	}
	if _, _, err := ParseNth("2:   "); err == nil {
		t.Error("expected blank nested selector to fail")
	}
}

// TestPrefixTableDrivesBothParseAndHasKnownPrefix is the guard that keeps the
// two readers of prefixKinds from drifting: every table entry must parse to its
// own Kind and be recognised by the predicate, in lower, upper and mixed case.
func TestPrefixTableDrivesBothParseAndHasKnownPrefix(t *testing.T) {
	if len(prefixKinds) == 0 {
		t.Fatal("prefixKinds is empty; the guard would pass vacuously")
	}

	for _, pk := range prefixKinds {
		for _, spelling := range []string{
			pk.Prefix,
			strings.ToUpper(pk.Prefix),
			strings.ToUpper(pk.Prefix[:1]) + pk.Prefix[1:],
		} {
			input := spelling + "value"
			t.Run(input, func(t *testing.T) {
				if !HasKnownPrefix(input) {
					t.Errorf("HasKnownPrefix(%q) = false, want true", input)
				}
				got := Parse(input)
				if got.Kind != pk.Kind {
					t.Errorf("Parse(%q).Kind = %q, want %q", input, got.Kind, pk.Kind)
				}
				if got.Value != "value" {
					t.Errorf("Parse(%q).Value = %q, want %q", input, got.Value, "value")
				}
			})
		}
	}
}

func TestPrefixTableCoversEveryKindWithAPrefix(t *testing.T) {
	tabled := map[Kind]bool{}
	for _, pk := range prefixKinds {
		tabled[pk.Kind] = true
	}
	for _, kind := range declaredKinds(t) {
		if !tabled[kind] {
			t.Errorf("kind %q is declared but has no prefix in prefixKinds, so Parse can never produce it and HasKnownPrefix cannot see it. Give it a prefix, or if it is deliberately unprefixed say so here", kind)
		}
	}
	if got := Parse("semantic:login button"); got.Kind != KindSemantic {
		t.Errorf(`Parse("semantic:...").Kind = %q, want %q`, got.Kind, KindSemantic)
	}
	if got := Parse("find:login button"); got.Kind != KindSemantic {
		t.Errorf(`Parse("find:...").Kind = %q, want %q`, got.Kind, KindSemantic)
	}
}

// Read from the declarations rather than listed here: the change this guard exists
// to catch is a kind added to the grammar, and a hand-written list is one the same
// commit would have to remember to update — which is the omission being guarded
// against. KindNone is the absence of a kind and takes no prefix.
func declaredKinds(t *testing.T) []Kind {
	t.Helper()

	pkg := srccensus.Load(t, ".", 1)
	_, files := pkg.Syntax()
	var kinds []Kind
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok {
				return true
			}
			if typ, isIdent := spec.Type.(*ast.Ident); !isIdent || typ.Name != "Kind" {
				return true
			}
			for _, value := range spec.Values {
				lit, isLit := value.(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					continue
				}
				if kind, err := strconv.Unquote(lit.Value); err == nil && kind != "" {
					kinds = append(kinds, Kind(kind))
				}
			}
			return true
		})
	}
	if len(kinds) < 2 {
		t.Fatalf("found %d declared kinds in %s; the scan matched almost nothing and the coverage check would pass vacuously", len(kinds), pkg.Dir())
	}
	return kinds
}

func TestHasKnownPrefixExcludesTheAutoDetectedForms(t *testing.T) {
	for _, in := range []string{"//div", "(//div)", "e5", "#id", ".class", "submit", "unknownprefix:value", ""} {
		if HasKnownPrefix(in) {
			t.Errorf("HasKnownPrefix(%q) = true, want false", in)
		}
	}

	if got := Parse("//div"); got.Kind != KindXPath || got.Value != "//div" {
		t.Errorf(`Parse("//div") = %+v, want xpath //div`, got)
	}
	if got := Parse("(//div)"); got.Kind != KindXPath || got.Value != "(//div)" {
		t.Errorf(`Parse("(//div)") = %+v, want xpath (//div)`, got)
	}
	if got := Parse("e5"); got.Kind != KindRef || got.Value != "e5" {
		t.Errorf(`Parse("e5") = %+v, want ref e5`, got)
	}

	if !HasKnownPrefix("  text:hello") {
		t.Error(`HasKnownPrefix("  text:hello") = false; leading space must be trimmed as Parse trims it`)
	}
}

func TestParseMixedCasePrefixesMatchTheirLowercaseForm(t *testing.T) {
	pairs := [][2]string{
		{"CSS:#id", "css:#id"},
		{"Text:hello", "text:hello"},
		{"XPath://div", "xpath://div"},
		{"Find:login button", "find:login button"},
		{"Role:button Save", "role:button Save"},
		{"TestID:submit", "testid:submit"},
		{"NTH:2:div", "nth:2:div"},
		{"Ref:e5", "ref:e5"},
	}
	for _, pair := range pairs {
		mixed, lower := Parse(pair[0]), Parse(pair[1])
		if mixed != lower {
			t.Errorf("Parse(%q) = %+v, want it identical to Parse(%q) = %+v", pair[0], mixed, pair[1], lower)
		}
	}

	if got := Parse("CSS:#id"); got.Kind != KindCSS || got.Value != "#id" {
		t.Errorf(`Parse("CSS:#id") = %+v, want css #id`, got)
	}
}

// TestTheSemanticNthOffsetIsAnAdapterNotAnOffByOne states why the +1 exists, so it is
// not "simplified" away by someone who sees the arithmetic and assumes a bug. The two
// bases are both documented: this project publishes nth as zero-based for every
// selector kind, and the semantic matcher's README publishes nth as one-based, where
// nth:0 is not the first match. Deleting the offset makes the documented nth:0 select
// nothing on the whole semantic family.
func TestTheSemanticNthOffsetIsAnAdapterNotAnOffByOne(t *testing.T) {
	if semanticNthOffset != 1 {
		t.Fatalf("semanticNthOffset = %d, want 1: PinchTab's public nth is zero-based and the matcher's is one-based", semanticNthOffset)
	}

	query, ok := Parse("nth:0:role:button Save").SemanticQuery()
	if !ok {
		t.Fatal("nth over a semantic form must reach the matcher")
	}
	if query != "nth:1:role:button Save" {
		t.Errorf("the public first match nth:0 arrives as %q; the matcher treats nth:0 as out of range, so it must arrive as nth:1", query)
	}
}

func TestMatcherNthQueryTranslatesEveryNestedForm(t *testing.T) {
	for input, want := range map[string]string{
		"nth:0:role:button Save": "nth:1:role:button Save",
		"nth:0:text:Save":        "nth:1:text:Save",
		"nth:2:Save":             "nth:3:Save",
		"NTH:1:css:#save":        "nth:2:css:#save",
	} {
		if got, ok := Parse(input).MatcherNthQuery(); !ok || got != want {
			t.Errorf("MatcherNthQuery(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"role:button Save", "first:text:Save", "nth:x:text:Save", "nth:-1:text:Save"} {
		if got, ok := Parse(input).MatcherNthQuery(); ok {
			t.Errorf("MatcherNthQuery(%q) = %q, want no translation", input, got)
		}
	}
}

func TestSemanticNthBaseReportsTheCallersOwnIndex(t *testing.T) {
	for _, tc := range []struct {
		input     string
		wantIndex int
		wantBase  string
		wantOK    bool
	}{
		{"nth:0:role:button Save", 0, "role:button Save", true},
		{"nth:2:label:Email", 2, "label:Email", true},
		{"first:role:button", 0, "", false},
		{"last:role:button", 0, "", false},
		{"nth:2:css:button", 0, "", false},
		{"role:button", 0, "", false},
	} {
		index, base, ok := Parse(tc.input).SemanticNthBase()
		if ok != tc.wantOK || index != tc.wantIndex || base != tc.wantBase {
			t.Errorf("SemanticNthBase(%q) = (%d, %q, %v), want (%d, %q, %v)", tc.input, index, base, ok, tc.wantIndex, tc.wantBase, tc.wantOK)
		}
	}
}
