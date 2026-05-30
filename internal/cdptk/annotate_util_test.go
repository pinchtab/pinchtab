package cdptk

import (
	"reflect"
	"testing"
)

func TestFilterAnnotationItems_Viewport(t *testing.T) {
	viewport := AnnotationRect{X: 0, Y: 0, W: 800, H: 600}
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 10, Y: 10, W: 100, H: 30}},   // inside
		{Ref: "e2", Box: AnnotationRect{X: 750, Y: 580, W: 100, H: 30}}, // partial overlap (bottom-right)
		{Ref: "e3", Box: AnnotationRect{X: 1000, Y: 10, W: 100, H: 30}}, // off-screen right
		{Ref: "e4", Box: AnnotationRect{X: 10, Y: 700, W: 100, H: 30}},  // off-screen below
	}
	got := FilterAnnotationItems(items, nil, viewport)
	if len(got) != 2 || got[0].Ref != "e1" || got[1].Ref != "e2" {
		t.Fatalf("FilterAnnotationItems viewport = %+v, want e1+e2", got)
	}
}

func TestFilterAnnotationItems_SelectorClip(t *testing.T) {
	target := AnnotationRect{X: 100, Y: 100, W: 200, H: 200}
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 110, Y: 110, W: 50, H: 20}}, // inside target
		{Ref: "e2", Box: AnnotationRect{X: 0, Y: 0, W: 50, H: 50}},     // outside (top-left)
		{Ref: "e3", Box: AnnotationRect{X: 290, Y: 290, W: 30, H: 30}}, // partial overlap with target bottom-right
	}
	// FilterAnnotationItems mutates the underlying slice; copy first.
	src := append([]AnnotationItem(nil), items...)
	got := FilterAnnotationItems(src, &target, AnnotationRect{})
	if len(got) != 2 || got[0].Ref != "e1" || got[1].Ref != "e3" {
		t.Fatalf("FilterAnnotationItems selector = %+v, want e1+e3", got)
	}
}

func TestProjectAnnotationBoxes_Viewport(t *testing.T) {
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 12.4, Y: 33.6, W: 100, H: 32}},
	}
	got := ProjectAnnotationBoxes(items, nil, ModeViewport, 0, 0)
	want := AnnotationRect{X: 12, Y: 34, W: 100, H: 32}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("viewport projection = %+v, want %+v", got[0].Box, want)
	}
}

func TestProjectAnnotationBoxes_SelectorClipSubtractsOrigin(t *testing.T) {
	target := AnnotationRect{X: 100, Y: 100, W: 200, H: 200}
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 110, Y: 130, W: 50, H: 20}},
	}
	got := ProjectAnnotationBoxes(items, &target, ModeSelectorClip, 0, 0)
	want := AnnotationRect{X: 10, Y: 30, W: 50, H: 20}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("selector projection = %+v, want %+v", got[0].Box, want)
	}
	// Source item should not be mutated.
	if items[0].Box.X != 110 {
		t.Fatalf("source mutated: %+v", items[0].Box)
	}
}

func TestProjectAnnotationBoxes_BeyondViewportAddsScroll(t *testing.T) {
	// Viewport-relative item rects must be shifted by the page scroll into the
	// document-coord image origin captured by beyondViewport mode.
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 10, Y: 20, W: 100, H: 32}},
	}
	got := ProjectAnnotationBoxes(items, nil, ModeBeyondViewport, 25, 500)
	want := AnnotationRect{X: 35, Y: 520, W: 100, H: 32}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("beyondViewport projection = %+v, want %+v", got[0].Box, want)
	}
}

func TestFilterAnnotationItems_BeyondViewportKeepsOffscreen(t *testing.T) {
	// In beyondViewport mode the region spans the whole document (shifted into
	// viewport space), so elements below the fold must be kept.
	region := AnnotationRect{X: 0, Y: 0, W: 800, H: 5000}
	items := []AnnotationItem{
		{Ref: "e1", Box: AnnotationRect{X: 10, Y: 10, W: 100, H: 30}},
		{Ref: "e2", Box: AnnotationRect{X: 10, Y: 3000, W: 100, H: 30}}, // far below the fold
	}
	got := FilterAnnotationItems(items, nil, region)
	if len(got) != 2 || got[0].Ref != "e1" || got[1].Ref != "e2" {
		t.Fatalf("beyondViewport filter = %+v, want e1+e2", got)
	}
}

func TestRefLessOrdersByNumber(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"e1", "e2", true},
		{"e2", "e10", true},  // numeric, not lexicographic
		{"e10", "e2", false}, // reverse direction
		{"e1", "e1", false},
	}
	for _, c := range cases {
		if got := RefLess(c.a, c.b); got != c.want {
			t.Errorf("RefLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRectsOverlapEdgeCases(t *testing.T) {
	a := AnnotationRect{X: 0, Y: 0, W: 10, H: 10}
	// Touching but not overlapping (right edge meets left edge).
	b := AnnotationRect{X: 10, Y: 0, W: 10, H: 10}
	if RectsOverlap(a, b) {
		t.Errorf("touching rects should not overlap")
	}
	// Zero-size rect never overlaps.
	if RectsOverlap(a, AnnotationRect{X: 5, Y: 5, W: 0, H: 0}) {
		t.Errorf("zero-size rect should not overlap")
	}
}
