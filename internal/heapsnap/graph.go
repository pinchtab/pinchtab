package heapsnap

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const rootNode = 0

type graph struct {
	classKey  []int64
	selfSize  []int64
	edgeCount []int32
	firstEdge []int32
	to        []int32
}

type edgeLayout struct {
	fields   int
	typeAt   int
	toAt     int
	weak     int64
	shortcut int64
}

func (h *Header) edgeLayout() (edgeLayout, error) {
	l := edgeLayout{fields: len(h.Meta.EdgeFields), typeAt: -1, toAt: -1, weak: -1, shortcut: -1}
	for i, f := range h.Meta.EdgeFields {
		switch f {
		case "type":
			l.typeAt = i
		case "to_node":
			l.toAt = i
		}
	}
	if l.typeAt < 0 || l.toAt < 0 {
		return l, &ParseError{Section: SectionMeta, Err: fmt.Errorf("edge_fields %v lack type or to_node", h.Meta.EdgeFields)}
	}
	if len(h.Meta.EdgeTypes) <= l.typeAt {
		return l, &ParseError{Section: SectionMeta, Err: errors.New("edge_types has no entry for the type field")}
	}
	var names []string
	if err := json.Unmarshal(h.Meta.EdgeTypes[l.typeAt], &names); err != nil {
		return l, &ParseError{Section: SectionMeta, Err: fmt.Errorf("edge_types type enum: %w", err)}
	}
	for i, name := range names {
		switch name {
		case "weak":
			l.weak = int64(i)
		case "shortcut":
			l.shortcut = int64(i)
		}
	}
	return l, nil
}

func classKeyFor(kind string, typ, name int64) int64 {
	if kind == "object" || kind == "native" {
		return name
	}
	return -(typ + 1)
}

func (g *graph) addNode(key, size, edges int64) error {
	if edges < 0 || edges > math.MaxInt32 {
		return &ParseError{Section: SectionNodes, Err: fmt.Errorf("edge_count %d out of range", edges)}
	}
	g.classKey = append(g.classKey, key)
	g.selfSize = append(g.selfSize, size)
	g.edgeCount = append(g.edgeCount, int32(edges))
	return nil
}

type edgeReader struct {
	g        *graph
	layout   edgeLayout
	nodeSize int
	field    int
	typ      int64
	target   int64
	owner    int
	left     int32
}

func (g *graph) edgeReader(layout edgeLayout, nodeFields int) *edgeReader {
	g.firstEdge = make([]int32, len(g.selfSize)+1)
	return &edgeReader{g: g, layout: layout, nodeSize: nodeFields, owner: -1}
}

func (r *edgeReader) value(v int64) error {
	switch r.field {
	case r.layout.typeAt:
		r.typ = v
	case r.layout.toAt:
		r.target = v
	}
	r.field++
	if r.field < r.layout.fields {
		return nil
	}
	r.field = 0
	return r.edge()
}

func (r *edgeReader) edge() error {
	g := r.g
	n := len(g.selfSize)
	for r.left == 0 {
		r.owner++
		if r.owner >= n {
			return &ParseError{Section: SectionEdges, Err: errors.New("more edges than the nodes' edge_count fields declare")}
		}
		g.firstEdge[r.owner] = int32(len(g.to))
		r.left = g.edgeCount[r.owner]
	}
	r.left--
	if r.target < 0 || r.target%int64(r.nodeSize) != 0 || r.target/int64(r.nodeSize) >= int64(n) {
		return &ParseError{Section: SectionEdges, Err: fmt.Errorf("to_node %d is not a node offset", r.target)}
	}
	if r.typ == r.layout.weak || (r.typ == r.layout.shortcut && r.owner != rootNode) {
		return nil
	}
	g.to = append(g.to, int32(r.target/int64(r.nodeSize)))
	return nil
}

func (r *edgeReader) finish() error {
	g := r.g
	if r.left != 0 {
		return &ParseError{Section: SectionEdges, Err: errors.New("fewer edges than the nodes' edge_count fields declare")}
	}
	for k := r.owner + 1; k < len(g.selfSize); k++ {
		if g.edgeCount[k] != 0 {
			return &ParseError{Section: SectionEdges, Err: errors.New("fewer edges than the nodes' edge_count fields declare")}
		}
		g.firstEdge[k] = int32(len(g.to))
	}
	g.firstEdge[len(g.selfSize)] = int32(len(g.to))
	return nil
}

func (g *graph) postOrder() (order []int32, index []int32) {
	n := len(g.selfSize)
	index = make([]int32, n)
	for i := range index {
		index[i] = -1
	}
	if n == 0 {
		return nil, index
	}
	visited := make([]bool, n)
	order = make([]int32, 0, n)
	stack := []int32{rootNode}
	cursor := []int32{g.firstEdge[rootNode]}
	visited[rootNode] = true
	for len(stack) > 0 {
		top := len(stack) - 1
		node := stack[top]
		if cursor[top] < g.firstEdge[node+1] {
			next := g.to[cursor[top]]
			cursor[top]++
			if !visited[next] {
				visited[next] = true
				stack = append(stack, next)
				cursor = append(cursor, g.firstEdge[next])
			}
			continue
		}
		index[node] = int32(len(order))
		order = append(order, node)
		stack = stack[:top]
		cursor = cursor[:top]
	}
	return order, index
}

func (g *graph) dominators(order, index []int32) []int32 {
	m := len(order)
	predStart := make([]int32, m+1)
	for _, u := range order {
		for e := g.firstEdge[u]; e < g.firstEdge[u+1]; e++ {
			predStart[index[g.to[e]]+1]++
		}
	}
	for i := 1; i <= m; i++ {
		predStart[i] += predStart[i-1]
	}
	preds := make([]int32, predStart[m])
	fill := append([]int32(nil), predStart[:m]...)
	for _, u := range order {
		for e := g.firstEdge[u]; e < g.firstEdge[u+1]; e++ {
			v := index[g.to[e]]
			preds[fill[v]] = index[u]
			fill[v]++
		}
	}

	doms := make([]int32, m)
	for i := range doms {
		doms[i] = -1
	}
	root := int32(m - 1)
	doms[root] = root
	intersect := func(a, b int32) int32 {
		for a != b {
			for a < b {
				a = doms[a]
			}
			for b < a {
				b = doms[b]
			}
		}
		return a
	}
	for changed := true; changed; {
		changed = false
		for i := root - 1; i >= 0; i-- {
			idom := int32(-1)
			for _, p := range preds[predStart[i]:predStart[i+1]] {
				if doms[p] < 0 {
					continue
				}
				if idom < 0 {
					idom = p
				} else {
					idom = intersect(p, idom)
				}
			}
			if idom != doms[i] {
				doms[i] = idom
				changed = true
			}
		}
	}
	return doms
}

func (g *graph) classRetained(classOf []int32, classes int) []int64 {
	order, index := g.postOrder()
	m := len(order)
	out := make([]int64, classes)
	if m == 0 {
		return out
	}
	doms := g.dominators(order, index)
	retained := make([]int64, m)
	for i, node := range order {
		retained[i] = g.selfSize[node]
	}
	root := int32(m - 1)
	for i := range root {
		retained[doms[i]] += retained[i]
	}

	childStart := make([]int32, m+1)
	for i := range root {
		childStart[doms[i]+1]++
	}
	for i := 1; i <= m; i++ {
		childStart[i] += childStart[i-1]
	}
	children := make([]int32, childStart[m])
	fill := append([]int32(nil), childStart[:m]...)
	for i := range root {
		children[fill[doms[i]]] = i
		fill[doms[i]]++
	}

	active := make([]int32, classes)
	enter := func(i int32) {
		class := classOf[order[i]]
		if active[class] == 0 {
			out[class] += retained[i]
		}
		active[class]++
	}
	stack := []int32{root}
	cursor := []int32{childStart[root]}
	enter(root)
	for len(stack) > 0 {
		top := len(stack) - 1
		i := stack[top]
		if cursor[top] < childStart[i+1] {
			child := children[cursor[top]]
			cursor[top]++
			enter(child)
			stack = append(stack, child)
			cursor = append(cursor, childStart[child])
			continue
		}
		active[classOf[order[i]]]--
		stack = stack[:top]
		cursor = cursor[:top]
	}
	return out
}
