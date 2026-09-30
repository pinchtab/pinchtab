package heapsnap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf8"
)

const (
	SectionDocument = "document"
	SectionSnapshot = "snapshot"
	SectionMeta     = "snapshot.meta"
	SectionNodes    = "nodes"
	SectionEdges    = "edges"
	SectionStrings  = "strings"
)

var ErrMissingSection = errors.New("section missing")

type ParseError struct {
	Section string
	Err     error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("heap snapshot %s: %v", e.Section, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

func sectionError(section string, err error) error {
	var pe *ParseError
	if errors.As(err, &pe) {
		return err
	}
	return &ParseError{Section: section, Err: err}
}

type Meta struct {
	NodeFields []string          `json:"node_fields"`
	NodeTypes  []json.RawMessage `json:"node_types"`
	EdgeFields []string          `json:"edge_fields"`
	EdgeTypes  []json.RawMessage `json:"edge_types"`
}

type Header struct {
	Meta      Meta `json:"meta"`
	NodeCount int  `json:"node_count"`
	EdgeCount int  `json:"edge_count"`
}

type nodeLayout struct {
	fields      int
	typeAt      int
	nameAt      int
	sizeAt      int
	edgeCountAt int
	typeName    []string
}

func (h *Header) layout() (nodeLayout, error) {
	l := nodeLayout{fields: len(h.Meta.NodeFields), typeAt: -1, nameAt: -1, sizeAt: -1, edgeCountAt: -1}
	for i, f := range h.Meta.NodeFields {
		switch f {
		case "type":
			l.typeAt = i
		case "name":
			l.nameAt = i
		case "self_size":
			l.sizeAt = i
		case "edge_count":
			l.edgeCountAt = i
		}
	}
	if l.typeAt < 0 || l.nameAt < 0 || l.sizeAt < 0 {
		return l, &ParseError{Section: SectionMeta, Err: fmt.Errorf("node_fields %v lack type, name or self_size", h.Meta.NodeFields)}
	}
	if len(h.Meta.NodeTypes) <= l.typeAt {
		return l, &ParseError{Section: SectionMeta, Err: errors.New("node_types has no entry for the type field")}
	}
	if err := json.Unmarshal(h.Meta.NodeTypes[l.typeAt], &l.typeName); err != nil {
		return l, &ParseError{Section: SectionMeta, Err: fmt.Errorf("node_types type enum: %w", err)}
	}
	if len(h.Meta.EdgeFields) == 0 {
		return l, &ParseError{Section: SectionMeta, Err: errors.New("edge_fields is empty")}
	}
	return l, nil
}

func decodeHeader(raw []byte) (*Header, nodeLayout, error) {
	var h struct {
		Meta      *Meta `json:"meta"`
		NodeCount *int  `json:"node_count"`
		EdgeCount *int  `json:"edge_count"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, nodeLayout{}, &ParseError{Section: SectionSnapshot, Err: err}
	}
	if h.Meta == nil {
		return nil, nodeLayout{}, &ParseError{Section: SectionMeta, Err: ErrMissingSection}
	}
	if h.NodeCount == nil || h.EdgeCount == nil {
		return nil, nodeLayout{}, &ParseError{Section: SectionSnapshot, Err: errors.New("node_count or edge_count missing")}
	}
	header := &Header{Meta: *h.Meta, NodeCount: *h.NodeCount, EdgeCount: *h.EdgeCount}
	l, err := header.layout()
	if err != nil {
		return nil, l, err
	}
	return header, l, nil
}

func ReadHeader(r io.Reader) (*Header, error) {
	s := newScanner(r)
	if err := s.expect('{'); err != nil {
		return nil, &ParseError{Section: SectionDocument, Err: err}
	}
	for {
		c, err := s.peek()
		if err != nil {
			return nil, &ParseError{Section: SectionDocument, Err: err}
		}
		if c == '}' {
			return nil, &ParseError{Section: SectionSnapshot, Err: ErrMissingSection}
		}
		key, err := s.readString(true)
		if err != nil {
			return nil, &ParseError{Section: SectionDocument, Err: err}
		}
		if err := s.expect(':'); err != nil {
			return nil, &ParseError{Section: SectionDocument, Err: err}
		}
		if key == SectionSnapshot {
			raw, err := s.captureValue()
			if err != nil {
				return nil, &ParseError{Section: SectionSnapshot, Err: err}
			}
			h, _, err := decodeHeader(raw)
			return h, err
		}
		if err := s.skipValue(); err != nil {
			return nil, &ParseError{Section: key, Err: err}
		}
		if c, err := s.next(); err != nil || c != ',' {
			return nil, &ParseError{Section: SectionSnapshot, Err: ErrMissingSection}
		}
	}
}

type Constructor struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	SelfSize int64  `json:"selfSize"`
}

type DuplicateString struct {
	Value    string `json:"value"`
	Length   int    `json:"length"`
	Count    int    `json:"count"`
	SelfSize int64  `json:"selfSize"`
}

type Aggregate struct {
	NodeCount        int
	EdgeCount        int
	TotalSelfSize    int64
	Constructors     []Constructor
	DuplicateStrings []DuplicateString
	Retained         map[string]int64
}

type ParseOptions struct {
	Retained bool
}

type bucket struct {
	count int
	size  int64
}

type parser struct {
	s          *scanner
	header     *Header
	layout     nodeLayout
	nodeValues int
	edgeValues int
	totalSize  int64
	byName     map[int64]*bucket
	byType     map[string]*bucket
	byString   map[int64]*bucket
	strings    map[int64]string
	seen       map[string]bool
	graph      *graph
}

func ParseWith(r io.Reader, opts ParseOptions) (*Aggregate, error) {
	p := &parser{
		s:        newScanner(r),
		byName:   map[int64]*bucket{},
		byType:   map[string]*bucket{},
		byString: map[int64]*bucket{},
		strings:  map[int64]string{},
		seen:     map[string]bool{},
	}
	if opts.Retained {
		p.graph = &graph{}
	}
	if err := p.document(); err != nil {
		return nil, err
	}
	return p.aggregate()
}

func ParseFileWith(path string, opts ParseOptions) (*Aggregate, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open heap snapshot: %w", err)
	}
	defer func() { _ = f.Close() }()
	return ParseWith(f, opts)
}

func (p *parser) document() error {
	s := p.s
	if err := s.expect('{'); err != nil {
		return &ParseError{Section: SectionDocument, Err: err}
	}
	c, err := s.peek()
	if err != nil {
		return &ParseError{Section: SectionDocument, Err: err}
	}
	for c != '}' {
		key, err := s.readString(true)
		if err != nil {
			return &ParseError{Section: SectionDocument, Err: err}
		}
		if err := s.expect(':'); err != nil {
			return &ParseError{Section: SectionDocument, Err: err}
		}
		if err := p.section(key); err != nil {
			return err
		}
		p.seen[key] = true
		if c, err = s.next(); err != nil {
			return &ParseError{Section: SectionDocument, Err: err}
		}
		if c != ',' && c != '}' {
			return &ParseError{Section: SectionDocument, Err: fmt.Errorf("after %s: want ',' or '}', got %q", key, c)}
		}
	}
	for _, name := range []string{SectionSnapshot, SectionNodes, SectionEdges, SectionStrings} {
		if !p.seen[name] {
			return &ParseError{Section: name, Err: ErrMissingSection}
		}
	}
	return nil
}

func (p *parser) section(key string) error {
	switch key {
	case SectionSnapshot:
		raw, err := p.s.captureValue()
		if err != nil {
			return &ParseError{Section: SectionSnapshot, Err: err}
		}
		p.header, p.layout, err = decodeHeader(raw)
		return err
	case SectionNodes:
		return p.nodes()
	case SectionEdges:
		return p.edges()
	case SectionStrings:
		return p.stringTable()
	default:
		if err := p.s.skipValue(); err != nil {
			return &ParseError{Section: key, Err: err}
		}
		return nil
	}
}

func (p *parser) nodes() error {
	if p.header == nil {
		return &ParseError{Section: SectionNodes, Err: errors.New("appears before snapshot.meta")}
	}
	l := p.layout
	if p.graph != nil && l.edgeCountAt < 0 {
		return &ParseError{Section: SectionMeta, Err: fmt.Errorf("node_fields %v lack edge_count, which retained sizes need", p.header.Meta.NodeFields)}
	}
	var typ, name, size, edges int64
	field := 0
	count, err := p.s.intArray(func(v int64) error {
		switch field {
		case l.typeAt:
			typ = v
		case l.nameAt:
			name = v
		case l.sizeAt:
			size = v
		case l.edgeCountAt:
			edges = v
		}
		field++
		if field < l.fields {
			return nil
		}
		field = 0
		return p.node(typ, name, size, edges)
	})
	if err != nil {
		return sectionError(SectionNodes, err)
	}
	if count%l.fields != 0 {
		return &ParseError{Section: SectionNodes, Err: fmt.Errorf("%d values is not a multiple of %d node fields", count, l.fields)}
	}
	p.nodeValues = count
	if nodes := count / l.fields; nodes != p.header.NodeCount {
		return &ParseError{Section: SectionNodes, Err: fmt.Errorf("holds %d nodes, snapshot.node_count says %d", nodes, p.header.NodeCount)}
	}
	return nil
}

func (p *parser) node(typ, name, size, edges int64) error {
	if typ < 0 || int(typ) >= len(p.layout.typeName) {
		return &ParseError{Section: SectionNodes, Err: fmt.Errorf("node type %d outside node_types", typ)}
	}
	p.totalSize += size
	kind := p.layout.typeName[typ]
	if p.graph != nil {
		if err := p.graph.addNode(classKeyFor(kind, typ, name), size, edges); err != nil {
			return err
		}
	}
	switch kind {
	case "object", "native":
		add(p.byName, name, size)
	default:
		b := p.byType[kind]
		if b == nil {
			b = &bucket{}
			p.byType[kind] = b
		}
		b.count++
		b.size += size
	}
	if kind == "string" {
		add(p.byString, name, size)
	}
	return nil
}

func add(m map[int64]*bucket, key, size int64) {
	b := m[key]
	if b == nil {
		b = &bucket{}
		m[key] = b
	}
	b.count++
	b.size += size
}

func (p *parser) edges() error {
	if p.header == nil {
		return &ParseError{Section: SectionEdges, Err: errors.New("appears before snapshot.meta")}
	}
	visit := func(int64) error { return nil }
	var reader *edgeReader
	if p.graph != nil {
		if !p.seen[SectionNodes] {
			return &ParseError{Section: SectionEdges, Err: errors.New("appears before nodes, which retained sizes need")}
		}
		layout, err := p.header.edgeLayout()
		if err != nil {
			return err
		}
		reader = p.graph.edgeReader(layout, p.layout.fields)
		visit = reader.value
	}
	count, err := p.s.intArray(visit)
	if err != nil {
		return sectionError(SectionEdges, err)
	}
	fields := len(p.header.Meta.EdgeFields)
	if count%fields != 0 {
		return &ParseError{Section: SectionEdges, Err: fmt.Errorf("%d values is not a multiple of %d edge fields", count, fields)}
	}
	p.edgeValues = count
	if edges := count / fields; edges != p.header.EdgeCount {
		return &ParseError{Section: SectionEdges, Err: fmt.Errorf("holds %d edges, snapshot.edge_count says %d", edges, p.header.EdgeCount)}
	}
	if reader != nil {
		return reader.finish()
	}
	return nil
}

func (p *parser) stringTable() error {
	keepAll := !p.seen[SectionNodes]
	_, err := p.s.stringArray(func(i int) bool {
		if keepAll {
			return true
		}
		key := int64(i)
		if _, ok := p.byName[key]; ok {
			return true
		}
		b, ok := p.byString[key]
		return ok && b.count > 1
	}, func(i int, v string) {
		p.strings[int64(i)] = v
	})
	if err != nil {
		return sectionError(SectionStrings, err)
	}
	return nil
}

func (p *parser) lookup(index int64) (string, error) {
	v, ok := p.strings[index]
	if !ok {
		return "", &ParseError{Section: SectionStrings, Err: fmt.Errorf("name index %d has no entry", index)}
	}
	return v, nil
}

func className(kind string) string {
	switch kind {
	case "hidden":
		return "(system)"
	case "code":
		return "(compiled code)"
	default:
		return "(" + kind + ")"
	}
}

func (p *parser) aggregate() (*Aggregate, error) {
	classes := map[string]*bucket{}
	merge := func(name string, b *bucket) {
		c := classes[name]
		if c == nil {
			c = &bucket{}
			classes[name] = c
		}
		c.count += b.count
		c.size += b.size
	}
	for index, b := range p.byName {
		name, err := p.lookup(index)
		if err != nil {
			return nil, err
		}
		merge(name, b)
	}
	for kind, b := range p.byType {
		merge(className(kind), b)
	}

	agg := &Aggregate{
		NodeCount:     p.nodeValues / p.layout.fields,
		EdgeCount:     p.edgeValues / len(p.header.Meta.EdgeFields),
		TotalSelfSize: p.totalSize,
		Constructors:  make([]Constructor, 0, len(classes)),
	}
	for name, b := range classes {
		agg.Constructors = append(agg.Constructors, Constructor{Name: name, Count: b.count, SelfSize: b.size})
	}
	sort.Slice(agg.Constructors, func(i, j int) bool { return agg.Constructors[i].Name < agg.Constructors[j].Name })

	for index, b := range p.byString {
		if b.count < 2 {
			continue
		}
		value, err := p.lookup(index)
		if err != nil {
			return nil, err
		}
		agg.DuplicateStrings = append(agg.DuplicateStrings, DuplicateString{
			Value:    truncateValue(value),
			Length:   utf8.RuneCountInString(value),
			Count:    b.count,
			SelfSize: b.size,
		})
	}
	sort.Slice(agg.DuplicateStrings, func(i, j int) bool {
		return lessDuplicate(agg.DuplicateStrings[i], agg.DuplicateStrings[j])
	})
	if p.graph != nil {
		retained, err := p.retained()
		if err != nil {
			return nil, err
		}
		agg.Retained = retained
	}
	return agg, nil
}

func (p *parser) classNameFor(key int64) (string, error) {
	if key >= 0 {
		return p.lookup(key)
	}
	return className(p.layout.typeName[-key-1]), nil
}

func (p *parser) retained() (map[string]int64, error) {
	ids := map[int64]int32{}
	byName := map[string]int32{}
	var names []string
	classOf := make([]int32, len(p.graph.classKey))
	for node, key := range p.graph.classKey {
		id, ok := ids[key]
		if !ok {
			name, err := p.classNameFor(key)
			if err != nil {
				return nil, err
			}
			if id, ok = byName[name]; !ok {
				id = int32(len(names))
				names = append(names, name)
				byName[name] = id
			}
			ids[key] = id
		}
		classOf[node] = id
	}
	sizes := p.graph.classRetained(classOf, len(names))
	out := make(map[string]int64, len(names))
	for id, name := range names {
		out[name] = sizes[id]
	}
	return out, nil
}

const MaxValueRunes = 120

func truncateValue(v string) string {
	if utf8.RuneCountInString(v) <= MaxValueRunes {
		return v
	}
	runes := []rune(v)
	return string(runes[:MaxValueRunes]) + "…"
}

func lessDuplicate(a, b DuplicateString) bool {
	if a.SelfSize != b.SelfSize {
		return a.SelfSize > b.SelfSize
	}
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return a.Value < b.Value
}
