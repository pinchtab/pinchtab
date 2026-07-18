package visualdiff

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata fixtures and golden files")

var (
	white = color.RGBA{0xff, 0xff, 0xff, 0xff}
	blue  = color.RGBA{0x20, 0x40, 0xc0, 0xff}
	red   = color.RGBA{0xd0, 0x20, 0x20, 0xff}
	black = color.RGBA{0x00, 0x00, 0x00, 0xff}
	gray  = color.RGBA{0x80, 0x80, 0x80, 0xff}
)

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// makeBaseline is the source of testdata/baseline.png: white 64x64 with a
// blue square at (8,8)-(20,20).
func makeBaseline() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	fill(img, img.Bounds(), white)
	fill(img, image.Rect(8, 8, 20, 20), blue)
	return img
}

// makeChanged is the source of testdata/changed.png: the baseline plus a red
// square at (20,20)-(30,30) — the one known change.
func makeChanged() *image.RGBA {
	img := makeBaseline()
	fill(img, image.Rect(20, 20, 30, 30), red)
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func loadPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v (run `go test -update` to generate fixtures)", path, err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

func TestMain(m *testing.M) {
	flag.Parse()
	if *update {
		regenerateTestdata()
	}
	os.Exit(m.Run())
}

func regenerateTestdata() {
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		panic(err)
	}
	t := &testing.T{}
	baseline := makeBaseline()
	changed := makeChanged()
	writePNG(t, filepath.Join("testdata", "baseline.png"), baseline)
	writePNG(t, filepath.Join("testdata", "changed.png"), changed)

	res, err := Compare(baseline, changed, Options{Tolerance: 0.05})
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := WriteAnnotatedPNG(&buf, baseline, res); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "annotated.golden.png"), buf.Bytes(), 0o644); err != nil {
		panic(err)
	}
}

func TestIdenticalImages(t *testing.T) {
	img := makeBaseline()
	res, err := Compare(img, img, Options{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.PixelsChanged != 0 {
		t.Errorf("PixelsChanged = %d, want 0", res.PixelsChanged)
	}
	if res.DiffPercentage != 0 {
		t.Errorf("DiffPercentage = %v, want 0", res.DiffPercentage)
	}
	if len(res.Regions) != 0 {
		t.Errorf("Regions = %v, want none", res.Regions)
	}
}

func TestKnownChangeGolden(t *testing.T) {
	baseline := loadPNG(t, filepath.Join("testdata", "baseline.png"))
	changed := loadPNG(t, filepath.Join("testdata", "changed.png"))

	res, err := Compare(baseline, changed, Options{Tolerance: 0.05})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.PixelsChanged != 100 {
		t.Errorf("PixelsChanged = %d, want 100", res.PixelsChanged)
	}
	wantPct := 100 * 100.0 / (64 * 64)
	if math.Abs(res.DiffPercentage-wantPct) > 0.01 {
		t.Errorf("DiffPercentage = %v, want %v", res.DiffPercentage, wantPct)
	}
	wantRegion := image.Rect(20, 20, 30, 30)
	if len(res.Regions) != 1 || res.Regions[0] != wantRegion {
		t.Errorf("Regions = %v, want [%v]", res.Regions, wantRegion)
	}
}

func TestDifferentDimensions(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fill(a, a.Bounds(), white)
	b := image.NewRGBA(image.Rect(0, 0, 4, 6))
	fill(b, b.Bounds(), white)

	res, err := Compare(a, b, Options{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.Width != 4 || res.Height != 6 {
		t.Errorf("compared area = %dx%d, want 4x6", res.Width, res.Height)
	}
	if res.PixelsChanged != 8 {
		t.Errorf("PixelsChanged = %d, want 8 (area present only in b)", res.PixelsChanged)
	}
	wantPct := 100 * 8.0 / 24
	if math.Abs(res.DiffPercentage-wantPct) > 0.01 {
		t.Errorf("DiffPercentage = %v, want %v", res.DiffPercentage, wantPct)
	}
	wantRegion := image.Rect(0, 4, 4, 6)
	if len(res.Regions) != 1 || res.Regions[0] != wantRegion {
		t.Errorf("Regions = %v, want [%v]", res.Regions, wantRegion)
	}
}

func TestNonZeroMinBounds(t *testing.T) {
	big := makeBaseline()
	sub, ok := big.SubImage(image.Rect(8, 8, 40, 40)).(*image.RGBA)
	if !ok {
		t.Fatal("SubImage is not *image.RGBA")
	}
	clone := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(clone, clone.Bounds(), sub, sub.Bounds().Min, draw.Src)

	res, err := Compare(sub, clone, Options{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if res.PixelsChanged != 0 {
		t.Errorf("PixelsChanged = %d, want 0 for translated identical content", res.PixelsChanged)
	}
}

func TestNilImage(t *testing.T) {
	if _, err := Compare(nil, makeBaseline(), Options{}); err == nil {
		t.Error("Compare(nil, img) should error")
	}
	if _, err := Compare(makeBaseline(), nil, Options{}); err == nil {
		t.Error("Compare(img, nil) should error")
	}
}

func TestInvalidTolerance(t *testing.T) {
	img := makeBaseline()
	for _, tol := range []float64{-0.1, 1.5} {
		if _, err := Compare(img, img, Options{Tolerance: tol}); err == nil {
			t.Errorf("Compare with tolerance %v should error", tol)
		}
	}
}

func TestIgnoreAntiAliasing(t *testing.T) {
	edge := func() *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 16, 16))
		fill(img, img.Bounds(), white)
		fill(img, image.Rect(5, 0, 6, 16), black)
		return img
	}
	a := edge()
	b := edge()
	fill(b, image.Rect(6, 4, 7, 5), gray)

	strict, err := Compare(a, b, Options{Tolerance: 0.05})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if strict.PixelsChanged != 1 {
		t.Errorf("strict PixelsChanged = %d, want 1", strict.PixelsChanged)
	}

	lenient, err := Compare(a, b, Options{Tolerance: 0.05, IgnoreAntiAliasing: true})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if lenient.PixelsChanged != 0 {
		t.Errorf("lenient PixelsChanged = %d, want 0 (edge pixel excused)", lenient.PixelsChanged)
	}
}

func TestAnnotatedGolden(t *testing.T) {
	baseline := loadPNG(t, filepath.Join("testdata", "baseline.png"))
	changed := loadPNG(t, filepath.Join("testdata", "changed.png"))

	res, err := Compare(baseline, changed, Options{Tolerance: 0.05})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteAnnotatedPNG(&buf, baseline, res); err != nil {
		t.Fatalf("WriteAnnotatedPNG: %v", err)
	}

	goldenPath := filepath.Join("testdata", "annotated.golden.png")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read %s: %v (run `go test -update` to generate)", goldenPath, err)
	}
	if !bytes.Equal(buf.Bytes(), golden) {
		t.Errorf("annotated PNG bytes differ from %s (%d vs %d bytes)", goldenPath, buf.Len(), len(golden))
	}
}
