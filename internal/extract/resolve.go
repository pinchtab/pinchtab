package extract

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/semdesc"
	"github.com/pinchtab/semantic"
)

const (
	reasonNoMatch         = "no_match"
	reasonNotNumeric      = "not_numeric"
	reasonNotBoolean      = "not_boolean"
	reasonRefNotFound     = "ref_not_found"
	reasonScopeNotFound   = "scope_not_found"
	reasonNoRepeatedGroup = "no_repeated_group"
	reasonNoFieldMatch    = "no_field_match"
	reasonTooFewItems     = "too_few_items"
)

const (
	sourceValue   = "value"
	sourceText    = "text"
	sourceName    = "name"
	sourceChecked = "checked"
	sourceHint    = "hint"
)

const (
	defaultThreshold = 0.3
	defaultMaxItems  = 100
)

type Options struct {
	Threshold       float64
	LexicalWeight   float64
	EmbeddingWeight float64
	MaxItems        int
	Matcher         semantic.ElementMatcher
	ctx             context.Context
}

type FieldResult struct {
	Ref        string       `json:"ref,omitempty"`
	Score      float64      `json:"score"`
	Confidence string       `json:"confidence"`
	Source     string       `json:"source,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Items      []ItemResult `json:"items,omitempty"`
	Truncated  bool         `json:"truncated,omitempty"`
}

type ItemResult struct {
	Ref    string                 `json:"ref"`
	Fields map[string]FieldResult `json:"fields"`
}

type Result struct {
	Data    map[string]any         `json:"data"`
	Fields  map[string]FieldResult `json:"fields"`
	Missing []string               `json:"missing"`
}

type view struct {
	nodes []observe.A11yNode
	descs []semantic.ElementDescriptor
	index map[string]int
}

func newView(ordered []observe.A11yNode) view {
	index := make(map[string]int, len(ordered))
	for i, n := range ordered {
		index[n.Ref] = i
	}
	return view{nodes: ordered, descs: semdesc.Build(ordered), index: index}
}

func (v view) node(ref string) (observe.A11yNode, bool) {
	i, ok := v.index[ref]
	if !ok {
		return observe.A11yNode{}, false
	}
	return v.nodes[i], true
}

var (
	defaultMatcherOnce sync.Once
	defaultMatcher     semantic.ElementMatcher
)

func sharedMatcher() semantic.ElementMatcher {
	defaultMatcherOnce.Do(func() {
		defaultMatcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))
	})
	return defaultMatcher
}

func ResolveContext(ctx context.Context, schema Schema, nodes []observe.A11yNode, opts Options) Result {
	opts.ctx = ctx
	if opts.Threshold <= 0 {
		opts.Threshold = defaultThreshold
	}
	if opts.MaxItems <= 0 {
		opts.MaxItems = defaultMaxItems
	}
	if opts.Matcher == nil {
		opts.Matcher = sharedMatcher()
	}
	v := newView(nodes)
	if schema.scope.kind != targetNone {
		node, fr, ok := matchTarget(schema.scope, "", v, opts)
		if !ok {
			return unresolvedScope(schema, fr)
		}
		v = v.subtree(v.index[node.Ref])
	}
	return resolveObject(schema, v, opts)
}

func unresolvedScope(schema Schema, fr FieldResult) Result {
	fr.Reason = reasonScopeNotFound
	result := Result{Data: map[string]any{}, Fields: map[string]FieldResult{}}
	for _, prop := range schema.Properties {
		result.Fields[prop.Name] = fr
		if prop.Required {
			result.Missing = append(result.Missing, prop.Name)
		}
	}
	return result
}

func resolveObject(schema Schema, v view, opts Options) Result {
	result := Result{
		Data:   map[string]any{},
		Fields: map[string]FieldResult{},
	}
	for _, prop := range schema.Properties {
		var (
			fr    FieldResult
			value any
			ok    bool
		)
		if prop.Type == TypeArray {
			fr, value, ok = resolveArray(prop, v, opts)
		} else {
			fr, value, ok = resolveField(prop, v, opts)
		}
		result.Fields[prop.Name] = fr
		if ok {
			result.Data[prop.Name] = value
		} else if prop.Required {
			result.Missing = append(result.Missing, prop.Name)
		}
	}
	sort.Strings(result.Missing)
	return result
}

func resolveField(prop Property, v view, opts Options) (FieldResult, any, bool) {
	node, fr, ok := matchTarget(prop.hint, fieldQuery(prop), v, opts)
	if !ok {
		return fr, nil, false
	}
	if control, labelled := v.controlLabelledBy(node); prop.hint.kind != targetRef && labelled {
		node, fr.Ref = control, control.Ref
	}

	value, source, ok := readValue(prop.Type, node)
	fr.Source = source
	if !ok {
		fr.Reason = coercionReason(prop.Type)
		return fr, nil, false
	}
	return fr, value, true
}

func matchTarget(tg target, fallbackQuery string, v view, opts Options) (observe.A11yNode, FieldResult, bool) {
	if tg.kind == targetRef {
		node, found := v.node(tg.value)
		fr := FieldResult{Ref: tg.value, Score: 1, Confidence: semantic.CalibrateConfidence(1), Source: sourceHint}
		if !found {
			fr.Score = 0
			fr.Confidence = semantic.CalibrateConfidence(0)
			fr.Reason = reasonRefNotFound
			return observe.A11yNode{}, fr, false
		}
		return node, fr, true
	}

	query := fallbackQuery
	if tg.kind == targetQuery {
		query = tg.value
	}
	res, err := opts.Matcher.Find(opts.ctx, query, v.descs, semantic.FindOptions{
		Threshold:       opts.Threshold,
		TopK:            len(v.descs),
		LexicalWeight:   opts.LexicalWeight,
		EmbeddingWeight: opts.EmbeddingWeight,
	})
	best, ok := v.pickBest(res.Matches)
	score := roundScore(best.Score)
	fr := FieldResult{Score: score, Confidence: semantic.CalibrateConfidence(score)}
	if err != nil || !ok {
		fr.Reason = reasonNoMatch
		return observe.A11yNode{}, fr, false
	}
	fr.Ref = best.Ref
	node, found := v.node(best.Ref)
	if !found {
		fr.Reason = reasonNoMatch
		return observe.A11yNode{}, fr, false
	}
	return node, fr, true
}

func (v view) pickBest(matches []semantic.ElementMatch) (semantic.ElementMatch, bool) {
	best, ok := semantic.ElementMatch{}, false
	var bestScore float64
	for _, m := range matches {
		s := roundScore(m.Score)
		switch {
		case !ok, s > bestScore, s == bestScore && v.index[m.Ref] < v.index[best.Ref]:
			best, bestScore, ok = m, s, true
		}
	}
	return best, ok
}

var labelRoles = map[string]bool{"StaticText": true, "LabelText": true}

var valueControlRoles = map[string]bool{
	"textbox": true, "searchbox": true, "combobox": true,
	"listbox": true, "spinbutton": true, "slider": true,
}

func (v view) controlLabelledBy(label observe.A11yNode) (observe.A11yNode, bool) {
	text := strings.TrimSpace(label.Name)
	if !labelRoles[label.Role] || text == "" {
		return observe.A11yNode{}, false
	}
	at := v.index[label.Ref]
	nearest := -1
	for i, n := range v.nodes {
		if !valueControlRoles[n.Role] || !namedBy(n.Name, text) {
			continue
		}
		if nearest < 0 || distance(i, at) < distance(nearest, at) {
			nearest = i
		}
	}
	if nearest < 0 {
		return observe.A11yNode{}, false
	}
	return v.nodes[nearest], true
}

func namedBy(name, label string) bool {
	name = strings.TrimSpace(name)
	prefix := label + " "
	return strings.EqualFold(name, label) || len(name) > len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

func distance(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

func roundScore(s float64) float64 {
	return math.Round(s*1e6) / 1e6
}

func fieldQuery(prop Property) string {
	return strings.TrimSpace(splitCaseWords(prop.Name) + " " + prop.Description)
}

func readValue(t Type, node observe.A11yNode) (any, string, bool) {
	if t == TypeBoolean {
		if node.Checked == observe.CheckedTrue || node.Checked == observe.CheckedFalse || node.Checked == observe.CheckedMixed {
			if b, ok := coerceBool(node, ""); ok {
				return b, sourceChecked, true
			}
			return nil, sourceChecked, false
		}
	}

	raw, source := rawValue(node)
	if raw == "" {
		return nil, "", false
	}

	switch t {
	case TypeString:
		return strings.TrimSpace(raw), source, true
	case TypeNumber:
		if f, ok := coerceNumber(raw); ok {
			return f, source, true
		}
		return nil, source, false
	case TypeInteger:
		if f, ok := coerceNumber(raw); ok {
			return int64(f), source, true
		}
		return nil, source, false
	case TypeBoolean:
		if b, ok := coerceBool(node, raw); ok {
			return b, source, true
		}
		return nil, source, false
	}
	return nil, source, false
}

func rawValue(node observe.A11yNode) (string, string) {
	switch {
	case strings.TrimSpace(node.Value) != "":
		return node.Value, sourceValue
	case strings.TrimSpace(node.Text) != "":
		return node.Text, sourceText
	case strings.TrimSpace(node.Name) != "":
		return node.Name, sourceName
	}
	return "", ""
}

func coercionReason(t Type) string {
	switch t {
	case TypeNumber, TypeInteger:
		return reasonNotNumeric
	case TypeBoolean:
		return reasonNotBoolean
	default:
		return reasonNoMatch
	}
}
