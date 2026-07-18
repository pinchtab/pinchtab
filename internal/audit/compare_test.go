package audit

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

func page(url string, opts ...func(*PageResult)) PageResult {
	p := PageResult{URL: url}
	for _, o := range opts {
		o(&p)
	}
	return p
}

func TestPairPagesByPath(t *testing.T) {
	live := []PageResult{
		page("http://x/live/"),
		page("http://x/live/a.html"),
		page("http://x/live/only-live.html"),
	}
	staging := []PageResult{
		page("http://y/stage/"),
		page("http://y/stage/a.html"),
		page("http://y/stage/only-staging.html"),
	}
	pairs := PairPages("http://x/live/", "http://y/stage/", live, staging)
	if len(pairs) != 4 {
		t.Fatalf("pairs = %d, want 4", len(pairs))
	}
	if pairs[0].Path != "" || pairs[0].Live == nil || pairs[0].Staging == nil {
		t.Errorf("base pair = %+v", pairs[0])
	}
	if pairs[1].Path != "a.html" || pairs[1].Live == nil || pairs[1].Staging == nil {
		t.Errorf("a.html pair = %+v", pairs[1])
	}
	if pairs[2].Path != "only-live.html" || pairs[2].Staging != nil {
		t.Errorf("live-only pair = %+v", pairs[2])
	}
	if pairs[3].Path != "only-staging.html" || pairs[3].Live != nil {
		t.Errorf("staging-only pair = %+v", pairs[3])
	}
}

func TestComparePagesAddedRemovedStatuses(t *testing.T) {
	live := AuditReport{Pages: []PageResult{page("http://x/l/gone.html")}}
	staging := AuditReport{Pages: []PageResult{page("http://y/s/new.html")}}
	outcome, err := ComparePages("http://x/l/", "http://y/s/", live, staging)
	if err != nil {
		t.Fatalf("ComparePages: %v", err)
	}
	if outcome.Report.Pages[0].Status != CompareStatusRemoved {
		t.Errorf("live-only status = %q", outcome.Report.Pages[0].Status)
	}
	if outcome.Report.Pages[1].Status != CompareStatusAdded {
		t.Errorf("staging-only status = %q", outcome.Report.Pages[1].Status)
	}
	if !outcome.Report.HasDiffs {
		t.Error("added/removed pages should count as diffs")
	}
}

func TestDiffPageDataIdentical(t *testing.T) {
	a := page("http://x/p", func(p *PageResult) {
		p.Browser.AccessibilityScore = 100
		p.Browser.TimingMetrics.Load = 20
	})
	b := page("http://y/p", func(p *PageResult) {
		p.Browser.AccessibilityScore = 100
		p.Browser.TimingMetrics.Load = 400 // below drift threshold: noise
	})
	if drift := diffPageData(a, b); len(drift) != 0 {
		t.Errorf("drift = %+v, want none", drift)
	}
}

func TestDiffPageDataFields(t *testing.T) {
	a := page("http://x/p", func(p *PageResult) {
		p.Browser.AccessibilityScore = 100
		p.Browser.ConsoleLogs = []ConsoleLogEntry{{Level: "error"}, {Level: "log"}}
		p.Browser.TimingMetrics.Load = 100
	})
	b := page("http://y/p", func(p *PageResult) {
		p.Browser.AccessibilityScore = 80
		p.Browser.BrokenAssets = []BrokenAsset{{URL: "x"}}
		p.Browser.TimingMetrics.Load = 2000
	})
	drift := diffPageData(a, b)
	fields := map[string]bool{}
	for _, d := range drift {
		fields[d.Field] = true
	}
	for _, want := range []string{"consoleErrors", "brokenAssets", "accessibilityScore", "loadMs"} {
		if !fields[want] {
			t.Errorf("missing drift field %q in %+v", want, drift)
		}
	}
}

func encodeShot(t *testing.T, c color.RGBA, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestComparePagesVisualDiff(t *testing.T) {
	white := encodeShot(t, color.RGBA{255, 255, 255, 255}, 16, 16)
	red := encodeShot(t, color.RGBA{200, 0, 0, 255}, 16, 16)

	live := AuditReport{Pages: []PageResult{
		page("http://x/l/same.html", func(p *PageResult) { p.Screenshot = white }),
		page("http://x/l/diff.html", func(p *PageResult) { p.Screenshot = white }),
	}}
	staging := AuditReport{Pages: []PageResult{
		page("http://y/s/same.html", func(p *PageResult) { p.Screenshot = white }),
		page("http://y/s/diff.html", func(p *PageResult) { p.Screenshot = red }),
	}}

	outcome, err := ComparePages("http://x/l/", "http://y/s/", live, staging)
	if err != nil {
		t.Fatalf("ComparePages: %v", err)
	}

	same := outcome.Report.Pages[0]
	if same.DiffPercentage == nil || *same.DiffPercentage != 0 {
		t.Errorf("identical pair DiffPercentage = %v, want 0", same.DiffPercentage)
	}
	if len(same.Drift) != 0 {
		t.Errorf("identical pair drift = %+v", same.Drift)
	}
	if _, ok := outcome.DiffImages["same.html"]; ok {
		t.Error("identical pair should not produce a diff image")
	}

	diff := outcome.Report.Pages[1]
	if diff.DiffPercentage == nil || *diff.DiffPercentage <= 0 {
		t.Errorf("changed pair DiffPercentage = %v, want > 0", diff.DiffPercentage)
	}
	if _, ok := outcome.DiffImages["diff.html"]; !ok {
		t.Error("changed pair should produce an annotated diff image")
	}
	if !outcome.Report.HasDiffs {
		t.Error("HasDiffs should be true")
	}
}

func TestComparePagesNoScreenshots(t *testing.T) {
	live := AuditReport{Pages: []PageResult{page("http://x/l/p.html")}}
	staging := AuditReport{Pages: []PageResult{page("http://y/s/p.html")}}
	outcome, err := ComparePages("http://x/l/", "http://y/s/", live, staging)
	if err != nil {
		t.Fatalf("ComparePages: %v", err)
	}
	pc := outcome.Report.Pages[0]
	if pc.DiffPercentage != nil {
		t.Errorf("DiffPercentage = %v, want nil without screenshots", pc.DiffPercentage)
	}
	if outcome.Report.HasDiffs {
		t.Error("no screenshots and no drift should not be a diff")
	}
}
