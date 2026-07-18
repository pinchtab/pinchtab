package audit

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/audit/visualdiff"
)

// Page comparison statuses.
const (
	CompareStatusCompared = "compared"
	CompareStatusAdded    = "added"   // present only on the staging side
	CompareStatusRemoved  = "removed" // present only on the live side
)

// compareTolerance is the per-pixel tolerance for screenshot comparison,
// absorbing anti-aliasing noise between two renders of the same page.
const compareTolerance = 0.1

// timingDriftThresholdMs is the load-time delta below which timing
// differences are treated as run-to-run noise, not drift.
const timingDriftThresholdMs = 1000.0

// DataDrift is one field-level difference between paired pages.
type DataDrift struct {
	// Field identifies what drifted (consoleErrors, brokenAssets,
	// accessibilityScore, loadMs).
	Field string `json:"field"`
	// Live and Staging are the two observed values, stringified.
	Live string `json:"live"`
	// Staging is the staging-side value.
	Staging string `json:"staging"`
}

// PageComparison is the comparison outcome for one paired path.
type PageComparison struct {
	// Path is the page path relative to the two base URLs.
	Path string `json:"path"`
	// Status is compared, added, or removed.
	Status string `json:"status"`
	// LiveURL and StagingURL are the audited URLs, when present.
	LiveURL string `json:"liveUrl,omitempty"`
	// StagingURL is the staging-side URL.
	StagingURL string `json:"stagingUrl,omitempty"`
	// DiffPercentage is the visual difference over the compared area, in
	// [0,100]; null when no screenshot pair was available.
	DiffPercentage *float64 `json:"diffPercentage,omitempty"`
	// DiffImagePath is the annotated diff image written by the caller,
	// set only when the pair visually differs.
	DiffImagePath string `json:"diffImagePath,omitempty"`
	// Drift lists field-level data differences; empty means identical data.
	Drift []DataDrift `json:"drift"`
	// Error carries per-page audit errors from either side.
	Error string `json:"error,omitempty"`
}

// changed reports whether this page counts as a difference for CI gating.
func (pc PageComparison) changed() bool {
	if pc.Status != CompareStatusCompared || len(pc.Drift) > 0 {
		return true
	}
	return pc.DiffPercentage != nil && *pc.DiffPercentage > 0
}

// ComparisonReport is the site-level live-vs-staging comparison.
type ComparisonReport struct {
	// SchemaVersion is the report schema version; always SchemaVersion.
	SchemaVersion string `json:"schemaVersion"`
	// GeneratedAt is when the comparison was produced.
	GeneratedAt time.Time `json:"generatedAt"`
	// LiveBase and StagingBase are the compared base URLs.
	LiveBase string `json:"liveBase"`
	// StagingBase is the staging base URL.
	StagingBase string `json:"stagingBase"`
	// Pages are the per-path comparison entries.
	Pages []PageComparison `json:"pages"`
	// HasDiffs reports whether any page shows a visual or data difference.
	HasDiffs bool `json:"hasDiffs"`
}

// CompareOutcome carries the report plus the annotated diff images (PNG
// bytes keyed by page path) for the caller to write to disk.
type CompareOutcome struct {
	Report     ComparisonReport
	DiffImages map[string][]byte
}

// PagePair joins the live and staging entries for one relative path. A nil
// side means the page exists only on the other.
type PagePair struct {
	Path    string
	Live    *PageResult
	Staging *PageResult
}

// relPath strips base from url, normalizing the trailing slash, so
// "http://x/site/" + "http://x/site/a.html" → "a.html" and the base itself
// maps to "".
func relPath(base, url string) string {
	base = strings.TrimSuffix(base, "/")
	if url == base {
		return ""
	}
	if strings.HasPrefix(url, base+"/") {
		return strings.TrimPrefix(url, base+"/")
	}
	return url
}

// PairPages pairs live and staging report pages by path relative to their
// base URLs: live-order pairs first, then staging-only pages. One-side-only
// pages come back with the other side nil.
func PairPages(liveBase, stagingBase string, live, staging []PageResult) []PagePair {
	stagingByPath := make(map[string]*PageResult, len(staging))
	for i := range staging {
		stagingByPath[relPath(stagingBase, staging[i].URL)] = &staging[i]
	}

	var pairs []PagePair
	seen := map[string]bool{}
	for i := range live {
		path := relPath(liveBase, live[i].URL)
		seen[path] = true
		pairs = append(pairs, PagePair{Path: path, Live: &live[i], Staging: stagingByPath[path]})
	}
	for i := range staging {
		path := relPath(stagingBase, staging[i].URL)
		if seen[path] {
			continue
		}
		seen[path] = true
		pairs = append(pairs, PagePair{Path: path, Staging: &staging[i]})
	}
	return pairs
}

// ComparePages pairs the two audit reports' pages and computes per-page
// visual and data diffs. Diffs are data: the only errors are malformed
// screenshots.
func ComparePages(liveBase, stagingBase string, live, staging AuditReport) (CompareOutcome, error) {
	outcome := CompareOutcome{
		Report: ComparisonReport{
			SchemaVersion: SchemaVersion,
			GeneratedAt:   time.Now().UTC(),
			LiveBase:      liveBase,
			StagingBase:   stagingBase,
		},
		DiffImages: map[string][]byte{},
	}

	for _, pair := range PairPages(liveBase, stagingBase, live.Pages, staging.Pages) {
		pc := PageComparison{Path: pair.Path, Status: CompareStatusCompared, Drift: []DataDrift{}}
		switch {
		case pair.Staging == nil:
			pc.Status = CompareStatusRemoved
			pc.LiveURL = pair.Live.URL
		case pair.Live == nil:
			pc.Status = CompareStatusAdded
			pc.StagingURL = pair.Staging.URL
		default:
			pc.LiveURL = pair.Live.URL
			pc.StagingURL = pair.Staging.URL
			pc.Drift = diffPageData(*pair.Live, *pair.Staging)
			pc.Error = joinPageErrors(*pair.Live, *pair.Staging)
			if err := compareVisual(&pc, pair, outcome.DiffImages); err != nil {
				return CompareOutcome{}, fmt.Errorf("page %q: %w", pair.Path, err)
			}
		}
		if pc.changed() {
			outcome.Report.HasDiffs = true
		}
		outcome.Report.Pages = append(outcome.Report.Pages, pc)
	}
	return outcome, nil
}

// compareVisual runs the visual diff for a fully-paired page when both
// sides carry screenshots, storing the annotated image for changed pairs.
func compareVisual(pc *PageComparison, pair PagePair, images map[string][]byte) error {
	if pair.Live.Screenshot == "" || pair.Staging.Screenshot == "" {
		return nil
	}
	liveImg, err := decodeScreenshot(pair.Live.Screenshot)
	if err != nil {
		return fmt.Errorf("live screenshot: %w", err)
	}
	stagingImg, err := decodeScreenshot(pair.Staging.Screenshot)
	if err != nil {
		return fmt.Errorf("staging screenshot: %w", err)
	}

	res, err := visualdiff.Compare(liveImg, stagingImg, visualdiff.Options{
		Tolerance:          compareTolerance,
		IgnoreAntiAliasing: true,
	})
	if err != nil {
		return err
	}
	pc.DiffPercentage = &res.DiffPercentage
	if res.PixelsChanged == 0 {
		return nil
	}

	var buf bytes.Buffer
	if err := visualdiff.WriteAnnotatedPNG(&buf, liveImg, res); err != nil {
		return fmt.Errorf("annotate: %w", err)
	}
	images[pair.Path] = buf.Bytes()
	return nil
}

func decodeScreenshot(b64 string) (image.Image, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

// diffPageData compares the data-level audit fields of a page pair. Timing
// deltas under timingDriftThresholdMs are run-to-run noise, not drift.
func diffPageData(live, staging PageResult) []DataDrift {
	drift := []DataDrift{}
	record := func(field string, liveVal, stagingVal any) {
		drift = append(drift, DataDrift{
			Field:   field,
			Live:    fmt.Sprintf("%v", liveVal),
			Staging: fmt.Sprintf("%v", stagingVal),
		})
	}

	liveErrs, stagingErrs := consoleErrorCount(live), consoleErrorCount(staging)
	if liveErrs != stagingErrs {
		record("consoleErrors", liveErrs, stagingErrs)
	}
	if lb, sb := len(live.Browser.BrokenAssets), len(staging.Browser.BrokenAssets); lb != sb {
		record("brokenAssets", lb, sb)
	}
	if live.Browser.AccessibilityScore != staging.Browser.AccessibilityScore {
		record("accessibilityScore", live.Browser.AccessibilityScore, staging.Browser.AccessibilityScore)
	}
	if delta := math.Abs(live.Browser.TimingMetrics.Load - staging.Browser.TimingMetrics.Load); delta > timingDriftThresholdMs {
		record("loadMs", live.Browser.TimingMetrics.Load, staging.Browser.TimingMetrics.Load)
	}
	return drift
}

func consoleErrorCount(p PageResult) int {
	n := 0
	for _, l := range p.Browser.ConsoleLogs {
		if l.Level == "error" {
			n++
		}
	}
	return n
}

func joinPageErrors(live, staging PageResult) string {
	var parts []string
	if live.Error != "" {
		parts = append(parts, "live: "+live.Error)
	}
	if staging.Error != "" {
		parts = append(parts, "staging: "+staging.Error)
	}
	return strings.Join(parts, "; ")
}
