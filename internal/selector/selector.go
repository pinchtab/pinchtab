// Package selector provides a unified element targeting system.
//
// Instead of separate ref, css, xpath, text, and semantic fields,
// callers use a single selector string. The type is auto-detected
// from the value or an explicit prefix:
//
//	"e5"              → Ref   (element ref from snapshot)
//	"css:#login"      → CSS   (explicit prefix)
//	"#login"          → CSS   (auto-detected)
//	"xpath://div"     → XPath
//	"text:Submit"     → Text  (match by visible text)
//	"find:login btn"  → Semantic (natural-language query)
//
// Bare strings that look like CSS selectors (start with ., #, [,
// or contain tag-like patterns) are treated as CSS. Everything else
// without a prefix is treated as a ref if it matches the eN pattern,
// or as CSS otherwise.
package selector

import (
	"fmt"
	"strings"
)

// Kind represents the type of a selector.
type Kind string

const (
	KindNone     Kind = ""
	KindRef      Kind = "ref"
	KindCSS      Kind = "css"
	KindXPath    Kind = "xpath"
	KindText     Kind = "text"
	KindSemantic Kind = "semantic"
)

// Selector is a parsed, unified element selector.
type Selector struct {
	Kind  Kind   `json:"kind"`
	Value string `json:"value"`
}

// String returns the canonical string representation with prefix.
func (s Selector) String() string {
	switch s.Kind {
	case KindRef:
		return s.Value
	case KindCSS:
		return "css:" + s.Value
	case KindXPath:
		return "xpath:" + s.Value
	case KindText:
		return "text:" + s.Value
	case KindSemantic:
		return "find:" + s.Value
	default:
		return s.Value
	}
}

// IsEmpty returns true if the selector has no value.
func (s Selector) IsEmpty() bool {
	return s.Value == ""
}

// Parse interprets a selector string and returns a typed Selector.
//
// Explicit prefixes take priority:
//
//	"css:..."    → CSS
//	"xpath:..."  → XPath
//	"text:..."   → Text
//	"find:..."   → Semantic
//	"ref:..."    → Ref (optional explicit prefix)
//
// Without a prefix, auto-detection applies:
//
//	"e123"       → Ref (matches /^e\d+$/)
//	"#id"        → CSS
//	".class"     → CSS
//	"[attr]"     → CSS
//	"tag.class"  → CSS
//	"//xpath"    → XPath
//	everything else → CSS (safest default for backward compat)
func Parse(s string) Selector {
	s = strings.TrimSpace(s)
	if s == "" {
		return Selector{}
	}

	// Explicit prefixes
	if after, ok := cutPrefix(s, "css:"); ok {
		return Selector{Kind: KindCSS, Value: after}
	}
	if after, ok := cutPrefix(s, "xpath:"); ok {
		return Selector{Kind: KindXPath, Value: after}
	}
	if after, ok := cutPrefix(s, "text:"); ok {
		return Selector{Kind: KindText, Value: after}
	}
	if after, ok := cutPrefix(s, "find:"); ok {
		return Selector{Kind: KindSemantic, Value: after}
	}
	if after, ok := cutPrefix(s, "ref:"); ok {
		return Selector{Kind: KindRef, Value: after}
	}

	// Auto-detect: XPath
	if strings.HasPrefix(s, "//") || strings.HasPrefix(s, "(//") {
		return Selector{Kind: KindXPath, Value: s}
	}

	// Auto-detect: Ref (e.g. e5, e123)
	if IsRef(s) {
		return Selector{Kind: KindRef, Value: s}
	}

	// Everything else is CSS (backward compatible default)
	return Selector{Kind: KindCSS, Value: s}
}

// IsRef returns true if the string matches the element ref pattern (e.g. "e5", "e123").
func IsRef(s string) bool {
	if len(s) < 2 || s[0] != 'e' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FromRef creates a Selector from a ref string.
func FromRef(ref string) Selector {
	if ref == "" {
		return Selector{}
	}
	return Selector{Kind: KindRef, Value: ref}
}

// FromCSS creates a Selector from a CSS selector string.
func FromCSS(css string) Selector {
	if css == "" {
		return Selector{}
	}
	return Selector{Kind: KindCSS, Value: css}
}

// FromXPath creates a Selector from an XPath expression.
func FromXPath(xpath string) Selector {
	if xpath == "" {
		return Selector{}
	}
	return Selector{Kind: KindXPath, Value: xpath}
}

// FromText creates a Selector from a text content query.
func FromText(text string) Selector {
	if text == "" {
		return Selector{}
	}
	return Selector{Kind: KindText, Value: text}
}

// FromSemantic creates a Selector from a semantic/natural-language query.
func FromSemantic(query string) Selector {
	if query == "" {
		return Selector{}
	}
	return Selector{Kind: KindSemantic, Value: query}
}

// Validate returns an error if the selector is invalid.
func (s Selector) Validate() error {
	if s.IsEmpty() {
		return fmt.Errorf("empty selector")
	}
	switch s.Kind {
	case KindRef, KindCSS, KindXPath, KindText, KindSemantic:
		return nil
	default:
		return fmt.Errorf("unknown selector kind: %q", s.Kind)
	}
}

// cutPrefix is a helper for strings.CutPrefix (available in Go 1.20+).
func cutPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return s, false
}
