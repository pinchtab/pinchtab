// Package visualdiff compares two screenshots pixel by pixel and renders
// annotated diff images. Pure Go: stdlib image packages only, no CGO.
package visualdiff

import (
	"errors"
	"image"
	"math"
)

// DefaultTolerance is a reasonable per-pixel tolerance for screenshot
// comparison when the caller has no better value.
const DefaultTolerance = 0.1

// aaEdgeLumaDelta is the neighbor luma contrast above which a pixel is
// considered to sit on a color edge for anti-aliasing detection.
const aaEdgeLumaDelta = 0.25

// Options controls how Compare matches pixels.
type Options struct {
	// Tolerance is the normalized color distance in [0,1] at or below which
	// two pixels count as equal. The zero value compares exactly.
	Tolerance float64
	// IgnoreAntiAliasing excuses differing pixels that sit on a color edge
	// in both images, the signature of anti-aliasing artifacts.
	IgnoreAntiAliasing bool
}

// Result is the outcome of comparing two images.
//
// When the input dimensions differ, the comparison area is the union of both
// sizes (anchored at the top-left): pixels present in only one image count as
// changed, and Width/Height report the union.
type Result struct {
	// Width and Height are the dimensions of the compared area.
	Width  int
	Height int
	// PixelsChanged is the number of pixels that differ.
	PixelsChanged int
	// DiffPercentage is PixelsChanged over the compared area, in [0,100].
	DiffPercentage float64
	// Regions are the bounding rectangles of 8-connected clusters of
	// changed pixels, in scan order.
	Regions []image.Rectangle

	mask []bool
}

// Compare diffs images a and b pixel by pixel. Differing dimensions are
// handled without error as documented on Result; it never panics on
// mismatched sizes.
func Compare(a, b image.Image, opts Options) (Result, error) {
	if a == nil || b == nil {
		return Result{}, errors.New("visualdiff: both images must be non-nil")
	}
	if opts.Tolerance < 0 || opts.Tolerance > 1 {
		return Result{}, errors.New("visualdiff: tolerance must be in [0,1]")
	}

	aw, ah := a.Bounds().Dx(), a.Bounds().Dy()
	bw, bh := b.Bounds().Dx(), b.Bounds().Dy()
	w, h := max(aw, bw), max(ah, bh)

	res := Result{Width: w, Height: h, mask: make([]bool, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inA := x < aw && y < ah
			inB := x < bw && y < bh
			if inA && inB {
				if colorDistance(pixelAt(a, x, y), pixelAt(b, x, y)) <= opts.Tolerance {
					continue
				}
				if opts.IgnoreAntiAliasing && onEdge(a, x, y) && onEdge(b, x, y) {
					continue
				}
			} else if !inA && !inB {
				continue
			}
			res.mask[y*w+x] = true
			res.PixelsChanged++
		}
	}

	if area := w * h; area > 0 {
		res.DiffPercentage = 100 * float64(res.PixelsChanged) / float64(area)
	}
	res.Regions = findRegions(res.mask, w, h)
	return res, nil
}

type pixel struct {
	r, g, b, a float64
}

func pixelAt(img image.Image, x, y int) pixel {
	r, g, b, a := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
	return pixel{
		r: float64(r) / 0xffff,
		g: float64(g) / 0xffff,
		b: float64(b) / 0xffff,
		a: float64(a) / 0xffff,
	}
}

func colorDistance(p, q pixel) float64 {
	dr, dg, db, da := p.r-q.r, p.g-q.g, p.b-q.b, p.a-q.a
	return math.Sqrt((dr*dr + dg*dg + db*db + da*da) / 4)
}

func luma(p pixel) float64 {
	return 0.299*p.r + 0.587*p.g + 0.114*p.b
}

// onEdge reports whether the pixel at image-local (x, y) has an 8-neighbor
// with strongly contrasting luma, i.e. sits on a color edge.
func onEdge(img image.Image, x, y int) bool {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if x >= w || y >= h {
		return false
	}
	l := luma(pixelAt(img, x, y))
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx, ny := x+dx, y+dy
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			if math.Abs(luma(pixelAt(img, nx, ny))-l) > aaEdgeLumaDelta {
				return true
			}
		}
	}
	return false
}

// findRegions returns the bounding rectangle of every 8-connected cluster of
// set pixels in mask, in scan order.
func findRegions(mask []bool, w, h int) []image.Rectangle {
	var regions []image.Rectangle
	visited := make([]bool, len(mask))
	var stack []int

	for i := range mask {
		if !mask[i] || visited[i] {
			continue
		}
		bounds := image.Rect(i%w, i/w, i%w+1, i/w+1)
		visited[i] = true
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			px, py := p%w, p/w
			bounds = bounds.Union(image.Rect(px, py, px+1, py+1))
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := px+dx, py+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					n := ny*w + nx
					if mask[n] && !visited[n] {
						visited[n] = true
						stack = append(stack, n)
					}
				}
			}
		}
		regions = append(regions, bounds)
	}
	return regions
}
