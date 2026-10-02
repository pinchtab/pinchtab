package extract

import (
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/semantic"
)

const (
	minRepeats          = 3
	minContainerRepeats = 2
	sampleItems         = 3
)

var containerRoles = map[string]bool{"list": true, "table": true, "rowgroup": true, "grid": true, "feed": true}

type group struct {
	v         view
	container int
	items     []int
	columns   []int
	score     float64
	resolver  itemResolver
}

func resolveArray(prop Property, v view, opts Options) (FieldResult, any, bool) {
	g, fr, ok := chooseGroup(prop, v, opts)
	if !ok {
		return fr, nil, false
	}
	limit := opts.MaxItems
	if prop.MaxItems > 0 && prop.MaxItems < limit {
		limit = prop.MaxItems
	}
	data := []map[string]any{}
	for _, item := range g.items {
		res := g.resolver.resolve(item)
		if len(res.Data) == 0 {
			continue
		}
		if len(data) == limit {
			fr.Truncated = true
			break
		}
		data = append(data, res.Data)
		fr.Items = append(fr.Items, ItemResult{Ref: g.v.nodes[item].Ref, Fields: res.Fields})
	}
	if len(data) < prop.MinItems {
		return FieldResult{Ref: fr.Ref, Score: fr.Score, Confidence: fr.Confidence, Reason: reasonTooFewItems}, nil, false
	}
	return fr, data, true
}

func chooseGroup(prop Property, v view, opts Options) (group, FieldResult, bool) {
	var candidates []group
	scoped := prop.scope.kind != targetNone
	if scoped {
		node, fr, ok := matchTarget(prop.scope, "", v, opts)
		if !ok {
			fr.Reason = reasonScopeNotFound
			return group{}, fr, false
		}
		candidates = scopedGroups(v.subtree(v.index[node.Ref]))
		if len(candidates) == 0 {
			return group{}, FieldResult{Ref: node.Ref, Confidence: semantic.CalibrateConfidence(0), Reason: reasonNoRepeatedGroup}, false
		}
	} else {
		candidates = detectGroups(v)
	}

	for i := range candidates {
		candidates[i].resolver = newItemResolver(candidates[i], *prop.Items, opts)
		candidates[i].score = scoreGroup(candidates[i], *prop.Items)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if len(a.items) != len(b.items) {
			return len(a.items) > len(b.items)
		}
		return a.container < b.container
	})
	if len(candidates) == 0 || (!scoped && candidates[0].score == 0) {
		return group{}, FieldResult{Confidence: semantic.CalibrateConfidence(0), Reason: reasonNoRepeatedGroup}, false
	}
	best := candidates[0]
	return best, FieldResult{Ref: best.v.nodes[best.container].Ref, Score: best.score, Confidence: semantic.CalibrateConfidence(best.score)}, true
}

func scopedGroups(v view) []group {
	groups := detectGroups(v)
	for _, g := range groups {
		if g.container == 0 {
			return groups
		}
	}
	if g, ok := groupAt(v, 0, 1); ok {
		groups = append(groups, g)
	}
	return groups
}

func detectGroups(v view) []group {
	var groups []group
	for i := range v.nodes {
		min := minRepeats
		if containerRoles[role(v.nodes[i])] {
			min = minContainerRepeats
		}
		if g, ok := groupAt(v, i, min); ok {
			groups = append(groups, g)
		}
	}
	return groups
}

func groupAt(v view, container, min int) (group, bool) {
	if v.isHeaderRow(container) {
		return group{}, false
	}
	children := v.children(container)
	dominant, count := dominantRole(v, children)
	if count < min || count == 0 {
		return group{}, false
	}
	g := group{v: v, container: container}
	for _, c := range children {
		if role(v.nodes[c]) == dominant && !v.isHeaderRow(c) {
			g.items = append(g.items, c)
		}
	}
	if len(g.items) == 0 {
		return group{}, false
	}
	if dominant == "row" {
		g.columns = headerColumns(v, container)
	}
	return g, true
}

func (v view) isHeaderRow(i int) bool {
	if role(v.nodes[i]) != "row" {
		return false
	}
	for _, c := range v.children(i) {
		if role(v.nodes[c]) == "columnheader" {
			return true
		}
	}
	return false
}

func dominantRole(v view, children []int) (string, int) {
	counts := map[string]int{}
	best, bestCount := "", 0
	for _, c := range children {
		r := role(v.nodes[c])
		counts[r]++
		if counts[r] > bestCount {
			best, bestCount = r, counts[r]
		}
	}
	return best, bestCount
}

func headerColumns(v view, container int) []int {
	table := container
	for _, a := range v.ancestors(container) {
		if r := role(v.nodes[a]); r == "table" || r == "grid" {
			table = a
			break
		}
	}
	end := v.subtreeEnd(table)
	for i := table; i < end; i++ {
		if v.isHeaderRow(i) {
			return v.children(i)
		}
	}
	return nil
}

func scoreGroup(g group, schema Schema) float64 {
	sample := g.items
	if len(sample) > sampleItems {
		sample = sample[:sampleItems]
	}
	resolved := 0
	for _, item := range sample {
		resolved += len(g.resolver.resolve(item).Data)
	}
	return roundScore(float64(resolved) / float64(len(sample)*len(schema.Properties)))
}

type itemResolver struct {
	schema  Schema
	v       view
	opts    Options
	columns map[string]columnField
}

type columnField struct {
	index int
	field FieldResult
}

func newItemResolver(g group, schema Schema, opts Options) itemResolver {
	v := g.v
	r := itemResolver{schema: schema, v: v, opts: opts}
	if g.columns == nil {
		return r
	}
	header := make([]observe.A11yNode, len(g.columns))
	position := make(map[string]int, len(g.columns))
	for i, c := range g.columns {
		header[i] = v.nodes[c]
		position[v.nodes[c].Ref] = i
	}
	headerView := newView(header)
	r.columns = map[string]columnField{}
	for _, prop := range schema.Properties {
		node, fr, ok := matchTarget(prop.hint, fieldQuery(prop), headerView, opts)
		if !ok {
			continue
		}
		r.columns[prop.Name] = columnField{index: position[node.Ref], field: fr}
	}
	return r
}

func (r itemResolver) resolve(item int) Result {
	if r.columns == nil {
		return resolveObject(r.schema, r.v.subtree(item), r.opts)
	}
	cells := r.v.children(item)
	result := Result{Data: map[string]any{}, Fields: map[string]FieldResult{}}
	for _, prop := range r.schema.Properties {
		fr, value, ok := r.resolveCell(prop, cells)
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

func (r itemResolver) resolveCell(prop Property, cells []int) (FieldResult, any, bool) {
	col, ok := r.columns[prop.Name]
	if !ok {
		return FieldResult{Confidence: semantic.CalibrateConfidence(0), Reason: reasonNoMatch}, nil, false
	}
	if col.index >= len(cells) {
		return FieldResult{Score: col.field.Score, Confidence: col.field.Confidence, Reason: reasonNoMatch}, nil, false
	}
	cell := r.v.nodes[cells[col.index]]
	fr := FieldResult{Ref: cell.Ref, Score: col.field.Score, Confidence: col.field.Confidence}
	value, source, ok := readValue(prop.Type, cell)
	fr.Source = source
	if !ok {
		fr.Reason = coercionReason(prop.Type)
		return fr, nil, false
	}
	return fr, value, true
}

func role(n observe.A11yNode) string {
	return strings.ToLower(strings.TrimSpace(n.Role))
}

func (v view) subtreeEnd(i int) int {
	depth := v.nodes[i].Depth
	end := i + 1
	for end < len(v.nodes) && v.nodes[end].Depth > depth {
		end++
	}
	return end
}

func (v view) children(i int) []int {
	var children []int
	for j, end := i+1, v.subtreeEnd(i); j < end; j = v.subtreeEnd(j) {
		children = append(children, j)
	}
	return children
}

func (v view) subtree(i int) view {
	return newView(v.nodes[i:v.subtreeEnd(i)])
}

func (v view) ancestors(i int) []int {
	var ancestors []int
	depth := v.nodes[i].Depth
	for j := i - 1; j >= 0; j-- {
		if v.nodes[j].Depth < depth {
			ancestors = append(ancestors, j)
			depth = v.nodes[j].Depth
		}
	}
	return ancestors
}
