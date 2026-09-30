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
//	"role:button Save" → Role/name locator
//	"label:Email"     → Form control by label text
//	"testid:submit"   → Test id locator
//	"last:button"     → Positional selector wrapper
//
// Prefixes match case-insensitively, so "CSS:#login" and "css:#login" are the
// same selector. Values are never case-folded.
//
// Bare strings that look like CSS selectors (start with ., #, [,
// or contain tag-like patterns) are treated as CSS. Everything else
// without a prefix is treated as a ref if it matches the eN pattern,
// or as CSS otherwise.
package selector

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind represents the type of a selector.
type Kind string

const (
	KindNone        Kind = ""
	KindRef         Kind = "ref"
	KindCSS         Kind = "css"
	KindXPath       Kind = "xpath"
	KindText        Kind = "text"
	KindSemantic    Kind = "semantic"
	KindRole        Kind = "role"
	KindLabel       Kind = "label"
	KindPlaceholder Kind = "placeholder"
	KindAlt         Kind = "alt"
	KindTitle       Kind = "title"
	KindTestID      Kind = "testid"
	KindFirst       Kind = "first"
	KindLast        Kind = "last"
	KindNth         Kind = "nth"
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
	case KindRole:
		return "role:" + s.Value
	case KindLabel:
		return "label:" + s.Value
	case KindPlaceholder:
		return "placeholder:" + s.Value
	case KindAlt:
		return "alt:" + s.Value
	case KindTitle:
		return "title:" + s.Value
	case KindTestID:
		return "testid:" + s.Value
	case KindFirst:
		return "first:" + s.Value
	case KindLast:
		return "last:" + s.Value
	case KindNth:
		return "nth:" + s.Value
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
// Explicit prefixes take priority and match case-insensitively:
//
//	"css:..."    → CSS
//	"xpath:..."  → XPath
//	"text:..."   → Text
//	"find:..."   → Semantic
//	"semantic:..." → Semantic
//	"role:..."   → Role/name locator
//	"label:..."  → Label locator
//	"placeholder:..." → Placeholder locator
//	"alt:..."    → Alt-text locator
//	"title:..."  → Title attribute locator
//	"testid:..." → Test id locator
//	"first:..."  → First match of nested selector
//	"last:..."   → Last match of nested selector
//	"nth:N:..."  → Nth match of nested selector
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

	if kind, value, ok := cutKnownPrefix(s); ok {
		return Selector{Kind: kind, Value: value}
	}

	if strings.HasPrefix(s, "//") || strings.HasPrefix(s, "(//") {
		return Selector{Kind: KindXPath, Value: s}
	}

	if IsRef(s) {
		return Selector{Kind: KindRef, Value: s}
	}

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

// Validate returns an error if the selector is invalid.
func (s Selector) Validate() error {
	if s.IsEmpty() {
		return fmt.Errorf("empty selector")
	}
	switch s.Kind {
	case KindRef, KindCSS, KindXPath, KindText, KindSemantic,
		KindRole, KindLabel, KindPlaceholder, KindAlt, KindTitle, KindTestID,
		KindFirst, KindLast, KindNth:
		return nil
	default:
		return fmt.Errorf("unknown selector kind: %q", s.Kind)
	}
}

// SemanticQuery returns the query string to send to the semantic matcher for
// selector-resolution paths. The existing text selector intentionally stays
// browser-side for backward-compatible action targeting.
func (s Selector) SemanticQuery() (string, bool) {
	if s.IsEmpty() {
		return "", false
	}
	switch s.Kind {
	case KindSemantic:
		return s.Value, strings.TrimSpace(s.Value) != ""
	case KindRole, KindLabel, KindPlaceholder, KindAlt, KindTitle, KindTestID:
		return s.String(), strings.TrimSpace(s.Value) != ""
	case KindFirst, KindLast:
		if rawSelectorCanUseSemantic(s.Value) {
			return s.String(), true
		}
	case KindNth:
		index, raw, err := ParseNth(s.Value)
		if err == nil && rawSelectorCanUseSemantic(raw) {
			return fmt.Sprintf("%s%d:%s", nthPrefix, index+semanticNthOffset, raw), true
		}
	}
	return "", false
}

// semanticNthOffset translates PinchTab's zero-based nth into the one-based nth the
// semantic matcher publishes. It is an adapter between two documented grammars, not an
// off-by-one to be simplified away: the matcher's README states that nth:<n> is 1-based,
// nth:1 selects the first ordered candidate and nth:0 is not the first match, while this
// project documents nth as zero-based for every selector kind. Both remain correct; the
// boundary converts. Removing it makes the documented nth:0 match nothing.
const semanticNthOffset = 1

const nthPrefix = "nth:"

// SemanticNthBase splits a positional nth wrapper over a semantic form into the
// caller's ZERO-based index and the bare query underneath it, for a caller that has to
// say how many matches the unwrapped selector had. It answers false for every other
// selector, including first:/last:, which cannot be out of range while any match exists.
func (s Selector) SemanticNthBase() (index int, base string, ok bool) {
	if s.Kind != KindNth {
		return 0, "", false
	}
	index, raw, err := ParseNth(s.Value)
	if err != nil || !rawSelectorCanUseSemantic(raw) {
		return 0, "", false
	}
	return index, raw, true
}

func rawSelectorCanUseSemantic(raw string) bool {
	sel := Parse(raw)
	switch sel.Kind {
	case KindRole, KindLabel, KindPlaceholder, KindAlt, KindTitle, KindTestID:
		return strings.TrimSpace(sel.Value) != ""
	default:
		return false
	}
}

// ParseNth splits the value of an "nth:" selector into its zero-based index and
// the nested selector. This package owns the selector grammar, so resolvers must
// use this rather than re-deriving the split: the eligibility checks here and
// the resolution path must agree on what a valid nth selector is.
func ParseNth(value string) (index int, nested string, err error) {
	rawIndex, rawSelector, ok := strings.Cut(value, ":")
	if !ok {
		return 0, "", fmt.Errorf("nth selector requires nth:<index>:<selector>")
	}
	rawSelector = strings.TrimSpace(rawSelector)
	if rawSelector == "" {
		return 0, "", fmt.Errorf("nth selector requires a nested selector")
	}
	index, convErr := strconv.Atoi(strings.TrimSpace(rawIndex))
	if convErr != nil || index < 0 {
		return 0, "", fmt.Errorf("nth selector index must be a zero-based non-negative integer")
	}
	return index, rawSelector, nil
}

// prefixKind binds one explicit prefix to the Kind it produces.
type prefixKind struct {
	Prefix string
	Kind   Kind
}

// prefixKinds is the one vocabulary of explicit selector prefixes. Parse and
// HasKnownPrefix both read it, so a new kind is a single edit that cannot be
// half-applied. Several prefixes may share a Kind: "find:" and "semantic:"
// both produce KindSemantic.
//
// The unprefixed forms Parse also accepts — "//div" and "(//div)" as XPath, a
// bare "e5" as a ref — are pattern matches rather than prefixes and stay out,
// so HasKnownPrefix keeps meaning what its name says.
var prefixKinds = []prefixKind{
	{"css:", KindCSS},
	{"xpath:", KindXPath},
	{"text:", KindText},
	{"find:", KindSemantic},
	{"semantic:", KindSemantic},
	{"role:", KindRole},
	{"label:", KindLabel},
	{"placeholder:", KindPlaceholder},
	{"alt:", KindAlt},
	{"title:", KindTitle},
	{"testid:", KindTestID},
	{"first:", KindFirst},
	{"last:", KindLast},
	{"nth:", KindNth},
	{"ref:", KindRef},
}

// HasKnownPrefix reports whether s starts with an explicit selector prefix
// Parse recognises. It answers false for the unprefixed forms Parse
// auto-detects, such as "//div" and "e5".
func HasKnownPrefix(s string) bool {
	_, _, ok := cutKnownPrefix(strings.TrimSpace(s))
	return ok
}

func cutKnownPrefix(s string) (Kind, string, bool) {
	for _, pk := range prefixKinds {
		if len(s) >= len(pk.Prefix) && strings.EqualFold(s[:len(pk.Prefix)], pk.Prefix) {
			return pk.Kind, s[len(pk.Prefix):], true
		}
	}
	return KindNone, s, false
}
