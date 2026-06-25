package selector

import (
	"testing"
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
		{"nth:2:label:Email", "nth:2:label:Email", true},
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

func TestFromConstructors(t *testing.T) {
	if s := FromRef("e5"); s.Kind != KindRef || s.Value != "e5" {
		t.Errorf("FromRef(\"e5\"): %+v", s)
	}
	if s := FromCSS("#x"); s.Kind != KindCSS || s.Value != "#x" {
		t.Errorf("FromCSS(\"#x\"): %+v", s)
	}
	if s := FromXPath("//a"); s.Kind != KindXPath || s.Value != "//a" {
		t.Errorf("FromXPath(\"//a\"): %+v", s)
	}
	if s := FromText("hi"); s.Kind != KindText || s.Value != "hi" {
		t.Errorf("FromText(\"hi\"): %+v", s)
	}
	if s := FromSemantic("btn"); s.Kind != KindSemantic || s.Value != "btn" {
		t.Errorf("FromSemantic(\"btn\"): %+v", s)
	}

	empties := []struct {
		name string
		fn   func(string) Selector
	}{
		{"FromRef", FromRef},
		{"FromCSS", FromCSS},
		{"FromXPath", FromXPath},
		{"FromText", FromText},
		{"FromSemantic", FromSemantic},
	}
	for _, e := range empties {
		if s := e.fn(""); !s.IsEmpty() {
			t.Errorf("%s(\"\") should be empty, got %+v", e.name, s)
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
