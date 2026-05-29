package observe

import "testing"

func TestIsVisibleWithDocumentCoordinates(t *testing.T) {
	vp := ViewportInfo{Width: 800, Height: 600, ScrollX: 0, ScrollY: 1000}

	if !isVisible(BoundingBox{X: 20, Y: 1050, W: 100, H: 40}, true, vp) {
		t.Fatal("box inside scrolled viewport should be visible")
	}
	if isVisible(BoundingBox{X: 20, Y: 50, W: 100, H: 40}, true, vp) {
		t.Fatal("box above scrolled viewport should not be visible")
	}
}
