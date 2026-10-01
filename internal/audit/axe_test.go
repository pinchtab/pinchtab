package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/assets"
)

// The echoed version must track the vendored asset: the axe.min.js banner reads
// `axe vX.Y.Z`, and a refresh that bumps the file without bumping AxeVersion
// would report a stale version to every caller.
func TestAxeVersionMatchesVendoredBanner(t *testing.T) {
	banner := assets.AxeJS
	if len(banner) > 120 {
		banner = banner[:120]
	}
	want := "axe v" + assets.AxeVersion
	if !strings.Contains(banner, want) {
		t.Fatalf("axe.min.js banner does not contain %q; AxeVersion is out of step with the asset:\n%s", want, banner)
	}
}

func TestParseEngine(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"", EngineNative, true},
		{"native", EngineNative, true},
		{"axe", EngineAxe, true},
		{"AXE", EngineAxe, true},
		{"  axe  ", EngineAxe, true},
		{"bogus", "", false},
		{"lighthouse", "", false},
	} {
		got, err := ParseEngine(tc.in)
		if tc.ok {
			if err != nil || got != tc.want {
				t.Errorf("ParseEngine(%q) = %q,%v; want %q,nil", tc.in, got, err, tc.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("ParseEngine(%q) accepted an unknown engine", tc.in)
		}
		if !strings.Contains(err.Error(), tc.in) {
			t.Errorf("ParseEngine(%q) error does not name the input: %v", tc.in, err)
		}
	}
}

func TestBuildAxeRunConfig(t *testing.T) {
	// Default: the WCAG tag set without best-practice.
	def, err := BuildAxeRunConfig(nil, nil)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if !strings.Contains(def, `"type":"tag"`) || strings.Contains(def, "best-practice") {
		t.Errorf("default config should be a WCAG tag run without best-practice: %s", def)
	}
	for _, tag := range DefaultAxeTags {
		if !strings.Contains(def, tag) {
			t.Errorf("default config missing tag %q: %s", tag, def)
		}
	}

	// Tags override the default; best-practice appears only when requested.
	tagged, err := BuildAxeRunConfig([]string{"best-practice"}, nil)
	if err != nil {
		t.Fatalf("tagged: %v", err)
	}
	if !strings.Contains(tagged, `"type":"tag"`) || !strings.Contains(tagged, "best-practice") {
		t.Errorf("tags config should request best-practice: %s", tagged)
	}

	// A rules allowlist wins over tags and runs by rule id.
	ruled, err := BuildAxeRunConfig([]string{"wcag2a"}, []string{"image-alt", "label"})
	if err != nil {
		t.Fatalf("ruled: %v", err)
	}
	if !strings.Contains(ruled, `"type":"rule"`) || !strings.Contains(ruled, "image-alt") || strings.Contains(ruled, "wcag2a") {
		t.Errorf("rules must win over tags and run by rule id: %s", ruled)
	}
}

func TestBuildAxeReport_ShapeAndScore(t *testing.T) {
	raw := AxeRawResult{
		URL:        "http://example.test/page",
		TestEngine: AxeTestEngine{Name: "axe-core", Version: "4.13.0"},
		Violations: []AxeRawViolation{
			{
				ID: "image-alt", Impact: "critical", Tags: []string{"wcag2a", "wcag111"},
				Help: "Images must have alternate text", HelpURL: "https://dequeuniversity.com/rules/axe/4.13/image-alt",
				Nodes: []AxeRawNode{{Target: []any{"#no-alt"}, HTML: "<img id=\"no-alt\">", FailureSummary: "Fix any of the following:"}},
			},
			{
				ID: "color-contrast", Impact: "serious", Tags: []string{"wcag2aa", "wcag143"},
				Nodes: []AxeRawNode{{Target: []any{"p.low-contrast"}, HTML: "<p>", FailureSummary: "Element has insufficient color contrast"}},
			},
		},
		Incomplete:   []AxeRawViolation{{ID: "color-contrast", Impact: "serious", Nodes: []AxeRawNode{{Target: []any{"span"}}}}},
		Passes:       12,
		Inapplicable: 40,
	}

	rep := BuildAxeReport(raw, assets.AxeVersion, false)
	if rep.Engine != EngineAxe || rep.Version != assets.AxeVersion || rep.URL != raw.URL {
		t.Errorf("envelope = %+v, want engine=axe version=%s url=%s", rep, assets.AxeVersion, raw.URL)
	}
	if len(rep.Violations) != 2 {
		t.Fatalf("violations = %d, want 2", len(rep.Violations))
	}
	if rep.Passes != 12 || rep.Inapplicable != 40 || rep.Incomplete != 1 {
		t.Errorf("counts passes=%d inapplicable=%d incomplete=%d, want 12/40/1", rep.Passes, rep.Inapplicable, rep.Incomplete)
	}
	// critical folds into serious weight (10); serious is 10 → 100-10-10 = 80.
	if rep.Score != 80 {
		t.Errorf("score = %d, want 80 (two serious-weight nodes)", rep.Score)
	}
	if rep.Violations[0].Nodes[0].Target[0] != "#no-alt" || rep.Violations[0].HelpURL == "" {
		t.Errorf("violation not carried through: %+v", rep.Violations[0])
	}
	// includeIncomplete off → no details, but the count stays.
	if len(rep.IncompleteViolations) != 0 {
		t.Errorf("incomplete details present without includeIncomplete: %+v", rep.IncompleteViolations)
	}

	withInc := BuildAxeReport(raw, assets.AxeVersion, true)
	if len(withInc.IncompleteViolations) != 1 {
		t.Errorf("includeIncomplete should add the incomplete details, got %d", len(withInc.IncompleteViolations))
	}
}

func TestBuildAxeReport_ScoreFlooredAtZero(t *testing.T) {
	var nodes []AxeRawNode
	for range 20 {
		nodes = append(nodes, AxeRawNode{Target: []any{"img"}})
	}
	raw := AxeRawResult{Violations: []AxeRawViolation{{ID: "image-alt", Impact: "critical", Nodes: nodes}}}
	if rep := BuildAxeReport(raw, "x", false); rep.Score != 0 {
		t.Errorf("score = %d, want 0 (floored)", rep.Score)
	}
}

// A real /a11y/audit?engine=axe capture (from a Docker browser run against
// tests/e2e/fixtures/a11y-violations.html) decodes into AxeReport, so the
// response types match axe-core 4.13's actual output shape and the version is
// echoed. Each violation carries an impact, helpUrl and a ref-mapped node — the
// contract an agent relies on to fall back to /action.
func TestAxeReport_RealCaptureShape(t *testing.T) {
	path := filepath.Join("testdata", "axe-violations.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no captured axe result yet (%v); run the e2e capture to add it", err)
	}
	var rep AxeReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("real axe capture does not decode into AxeReport: %v", err)
	}
	if rep.Engine != EngineAxe || rep.Version != assets.AxeVersion {
		t.Errorf("capture engine=%q version=%q, want axe/%s", rep.Engine, rep.Version, assets.AxeVersion)
	}
	ids := map[string]bool{}
	anyRef := false
	for _, v := range rep.Violations {
		ids[v.ID] = true
		if v.Impact == "" {
			t.Errorf("violation %q has no impact", v.ID)
		}
		if v.HelpURL == "" {
			t.Errorf("violation %q has no helpUrl", v.ID)
		}
		for _, n := range v.Nodes {
			if n.Ref != "" {
				anyRef = true
			}
		}
	}
	for _, want := range []string{"image-alt", "label", "color-contrast"} {
		if !ids[want] {
			t.Errorf("captured result missing expected violation %q (have %v)", want, keys(ids))
		}
	}
	if !anyRef {
		t.Error("no violation node carried a ref; ref-mapping did not run on the capture")
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
