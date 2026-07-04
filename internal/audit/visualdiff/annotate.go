package visualdiff

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
)

var (
	highlight  = color.RGBA{R: 0xff, A: 0xff}
	background = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// RenderAnnotated draws baseline on a canvas sized to the compared area,
// tints every changed pixel red, and outlines each changed region. The
// output depends only on the baseline pixels and the Result, so it is
// deterministic.
func RenderAnnotated(baseline image.Image, res Result) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, res.Width, res.Height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(canvas, baseline.Bounds().Sub(baseline.Bounds().Min), baseline, baseline.Bounds().Min, draw.Src)

	for y := 0; y < res.Height; y++ {
		for x := 0; x < res.Width; x++ {
			if !res.mask[y*res.Width+x] {
				continue
			}
			base := canvas.RGBAAt(x, y)
			canvas.SetRGBA(x, y, color.RGBA{
				R: uint8((uint16(base.R) + uint16(highlight.R)) / 2),
				G: uint8(uint16(base.G) / 2),
				B: uint8(uint16(base.B) / 2),
				A: 0xff,
			})
		}
	}

	for _, region := range res.Regions {
		outlineRect(canvas, region)
	}
	return canvas
}

// WriteAnnotatedPNG renders the annotated diff for baseline and encodes it
// as PNG.
func WriteAnnotatedPNG(w io.Writer, baseline image.Image, res Result) error {
	return png.Encode(w, RenderAnnotated(baseline, res))
}

func outlineRect(canvas *image.RGBA, r image.Rectangle) {
	r = r.Intersect(canvas.Bounds())
	for x := r.Min.X; x < r.Max.X; x++ {
		canvas.SetRGBA(x, r.Min.Y, highlight)
		canvas.SetRGBA(x, r.Max.Y-1, highlight)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		canvas.SetRGBA(r.Min.X, y, highlight)
		canvas.SetRGBA(r.Max.X-1, y, highlight)
	}
}
