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
		{"xpath://div[@id='main']", KindXPath, "//div[@id='main']"},
		{"text:Submit", KindText, "Submit"},
		{"text:Log in", KindText, "Log in"},
		{"find:login button", KindSemantic, "login button"},
		{"find:the search input field", KindSemantic, "the search input field"},
		{"ref:e5", KindRef, "e5"},
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
		// Refs
		{"e5", KindRef, "e5"},
		{"e123", KindRef, "e123"},
		{"e0", KindRef, "e0"},

		// CSS auto-detect
		{"#login", KindCSS, "#login"},
		{".btn", KindCSS, ".btn"},
		{"[type=file]", KindCSS, "[type=file]"},
		{"button.submit", KindCSS, "button.submit"},
		{"div > span", KindCSS, "div > span"},
		{"input[name='email']", KindCSS, "input[name='email']"},

		// XPath auto-detect
		{"//div[@class='main']", KindXPath, "//div[@class='main']"},
		{"(//button)[1]", KindXPath, "(//button)[1]"},

		// Bare tag names → CSS (backward compat)
		{"button", KindCSS, "button"},
		{"embed", KindCSS, "embed"},
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
	s = Parse("   ")
	if !s.IsEmpty() {
		t.Error("Parse(\"   \") should be empty")
	}
}

func TestIsRef(t *testing.T) {
	refs := []string{"e0", "e5", "e42", "e123"}
	for _, r := range refs {
		if !IsRef(r) {
			t.Errorf("IsRef(%q) = false, want true", r)
		}
	}

	nonRefs := []string{"", "e", "E5", "ex5", "e5x", "embed", "email", "#e5", "ref:e5"}
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
		{Selector{KindCSS, "#login"}, "css:#login"},
		{Selector{KindXPath, "//div"}, "xpath://div"},
		{Selector{KindText, "Submit"}, "text:Submit"},
		{Selector{KindSemantic, "login button"}, "find:login button"},
	}
	for _, tt := range tests {
		if got := tt.sel.String(); got != tt.want {
			t.Errorf("Selector{%s, %q}.String() = %q, want %q", tt.sel.Kind, tt.sel.Value, got, tt.want)
		}
	}
}

func TestSelector_Validate(t *testing.T) {
	valid := []Selector{
		{KindRef, "e5"},
		{KindCSS, "#login"},
		{KindXPath, "//div"},
		{KindText, "Submit"},
		{KindSemantic, "login button"},
	}
	for _, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", s, err)
		}
	}

	if err := (Selector{}).Validate(); err == nil {
		t.Error("Validate(empty) should fail")
	}
	if err := (Selector{Kind: "bogus", Value: "x"}).Validate(); err == nil {
		t.Error("Validate(bogus kind) should fail")
	}
}

func TestFromConstructors(t *testing.T) {
	if s := FromRef("e5"); s.Kind != KindRef || s.Value != "e5" {
		t.Errorf("FromRef: %+v", s)
	}
	if s := FromCSS("#x"); s.Kind != KindCSS || s.Value != "#x" {
		t.Errorf("FromCSS: %+v", s)
	}
	if s := FromXPath("//a"); s.Kind != KindXPath || s.Value != "//a" {
		t.Errorf("FromXPath: %+v", s)
	}
	if s := FromText("hi"); s.Kind != KindText || s.Value != "hi" {
		t.Errorf("FromText: %+v", s)
	}
	if s := FromSemantic("btn"); s.Kind != KindSemantic || s.Value != "btn" {
		t.Errorf("FromSemantic: %+v", s)
	}

	// Empty constructors
	if s := FromRef(""); !s.IsEmpty() {
		t.Error("FromRef(\"\") should be empty")
	}
	if s := FromCSS(""); !s.IsEmpty() {
		t.Error("FromCSS(\"\") should be empty")
	}
}

func TestParse_Roundtrip(t *testing.T) {
	inputs := []string{
		"e5",
		"css:#login",
		"xpath://div[@id='x']",
		"text:Submit Order",
		"find:the big red button",
	}
	for _, input := range inputs {
		s := Parse(input)
		rt := Parse(s.String())
		if rt.Kind != s.Kind || rt.Value != s.Value {
			t.Errorf("roundtrip failed: %q → %+v → %q → %+v", input, s, s.String(), rt)
		}
	}
}
