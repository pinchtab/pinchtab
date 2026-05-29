package bridge

import (
	"testing"

	"github.com/chromedp/cdproto/page"
)

func TestClampScale(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 1},
		{-1, 1},
		{0.01, 0.05},
		{0.05, 0.05},
		{0.5, 0.5},
		{1, 1},
		{2, 2},
		{4, 4},
		{4.5, 4},
		{1000, 4},
	}
	for _, c := range cases {
		got := ClampScale(c.in)
		if got != c.want {
			t.Errorf("ClampScale(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestProjectBoundsToClip(t *testing.T) {
	nodes := []A11yNode{
		{
			Ref:         "e1",
			BoundingBox: &BoundingBox{X: 125, Y: 240, W: 30, H: 20},
		},
		{Ref: "e2"},
	}
	projectBoundsToClip(nodes, page.Viewport{X: 100, Y: 200, Width: 300, Height: 200})

	if nodes[0].BoundingBox.X != 25 || nodes[0].BoundingBox.Y != 40 {
		t.Fatalf("projected box = (%v,%v), want (25,40)", nodes[0].BoundingBox.X, nodes[0].BoundingBox.Y)
	}
	if nodes[0].BoundingBox.W != 30 || nodes[0].BoundingBox.H != 20 {
		t.Fatalf("projected size = %vx%v, want 30x20", nodes[0].BoundingBox.W, nodes[0].BoundingBox.H)
	}
	if nodes[1].BoundingBox != nil {
		t.Fatal("node without bounding box should remain unchanged")
	}
}
