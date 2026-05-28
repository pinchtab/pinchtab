package handlers

import (
	"reflect"
	"testing"
)

func TestFilterAnnotationItems_Viewport(t *testing.T) {
	viewport := annotationRect{X: 0, Y: 0, W: 800, H: 600}
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 10, Y: 10, W: 100, H: 30}},   // inside
		{Ref: "e2", Box: annotationRect{X: 750, Y: 580, W: 100, H: 30}}, // partial overlap (bottom-right)
		{Ref: "e3", Box: annotationRect{X: 1000, Y: 10, W: 100, H: 30}}, // off-screen right
		{Ref: "e4", Box: annotationRect{X: 10, Y: 700, W: 100, H: 30}},  // off-screen below
	}
	got := filterAnnotationItems(items, nil, viewport)
	if len(got) != 2 || got[0].Ref != "e1" || got[1].Ref != "e2" {
		t.Fatalf("filterAnnotationItems viewport = %+v, want e1+e2", got)
	}
}

func TestFilterAnnotationItems_SelectorClip(t *testing.T) {
	target := annotationRect{X: 100, Y: 100, W: 200, H: 200}
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 110, Y: 110, W: 50, H: 20}}, // inside target
		{Ref: "e2", Box: annotationRect{X: 0, Y: 0, W: 50, H: 50}},     // outside (top-left)
		{Ref: "e3", Box: annotationRect{X: 290, Y: 290, W: 30, H: 30}}, // partial overlap with target bottom-right
	}
	// filterAnnotationItems mutates the underlying slice; copy first.
	src := append([]annotationItem(nil), items...)
	got := filterAnnotationItems(src, &target, annotationRect{})
	if len(got) != 2 || got[0].Ref != "e1" || got[1].Ref != "e3" {
		t.Fatalf("filterAnnotationItems selector = %+v, want e1+e3", got)
	}
}

func TestProjectAnnotationBoxes_Viewport(t *testing.T) {
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 12.4, Y: 33.6, W: 100, H: 32}},
	}
	got := projectAnnotationBoxes(items, nil, modeViewport, 0, 0)
	want := annotationRect{X: 12, Y: 34, W: 100, H: 32}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("viewport projection = %+v, want %+v", got[0].Box, want)
	}
}

func TestProjectAnnotationBoxes_SelectorClipSubtractsOrigin(t *testing.T) {
	target := annotationRect{X: 100, Y: 100, W: 200, H: 200}
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 110, Y: 130, W: 50, H: 20}},
	}
	got := projectAnnotationBoxes(items, &target, modeSelectorClip, 0, 0)
	want := annotationRect{X: 10, Y: 30, W: 50, H: 20}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("selector projection = %+v, want %+v", got[0].Box, want)
	}
	// Source item should not be mutated.
	if items[0].Box.X != 110 {
		t.Fatalf("source mutated: %+v", items[0].Box)
	}
}

func TestProjectAnnotationBoxes_BeyondViewportAddsScroll(t *testing.T) {
	// Item rect is viewport-relative; scroll offsets convert it into the
	// document-coord image origin captured by beyondViewport mode.
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 50, Y: -200, W: 100, H: 40}},
	}
	got := projectAnnotationBoxes(items, nil, modeBeyondViewport, 25, 500)
	want := annotationRect{X: 75, Y: 300, W: 100, H: 40}
	if !reflect.DeepEqual(got[0].Box, want) {
		t.Fatalf("beyondViewport projection = %+v, want %+v", got[0].Box, want)
	}
	if items[0].Box.Y != -200 {
		t.Fatalf("source mutated: %+v", items[0].Box)
	}
}

func TestFilterAnnotationItems_BeyondViewportKeepsOffscreen(t *testing.T) {
	// Document is 800×3000; viewport is 800×600 scrolled to y=500.
	// Items live at document y=10 (above viewport) and y=2500 (below viewport).
	// Both should survive the filter because both fall inside the document.
	docRegion := annotationRect{X: 0, Y: -500, W: 800, H: 3000} // viewport-rel doc bounds
	items := []annotationItem{
		{Ref: "e1", Box: annotationRect{X: 10, Y: -490, W: 100, H: 30}}, // doc y=10
		{Ref: "e2", Box: annotationRect{X: 10, Y: 2000, W: 100, H: 30}}, // doc y=2500
		{Ref: "e3", Box: annotationRect{X: 10, Y: 2510, W: 100, H: 30}}, // doc y=3010 — outside doc
	}
	src := append([]annotationItem(nil), items...)
	got := filterAnnotationItems(src, nil, docRegion)
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
		if got := refLess(c.a, c.b); got != c.want {
			t.Errorf("refLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRectsOverlapEdgeCases(t *testing.T) {
	a := annotationRect{X: 0, Y: 0, W: 10, H: 10}
	// Touching but not overlapping (right edge meets left edge).
	b := annotationRect{X: 10, Y: 0, W: 10, H: 10}
	if rectsOverlap(a, b) {
		t.Errorf("touching rects should not overlap")
	}
	// Zero-size rect never overlaps.
	if rectsOverlap(a, annotationRect{X: 5, Y: 5, W: 0, H: 0}) {
		t.Errorf("zero-size rect should not overlap")
	}
}
