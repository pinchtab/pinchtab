package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/contentguard"
	"github.com/pinchtab/pinchtab/internal/idpi"
	"github.com/pinchtab/semantic"
)

func boundaryHandlers(enabled, wrap bool) *Handlers {
	cfg := &config.RuntimeConfig{IDPI: config.IDPIConfig{Enabled: enabled, WrapContent: wrap, ScanContent: true}}
	guard := idpi.NewGuard(cfg.IDPI, nil)
	return &Handlers{Config: cfg, IDPIGuard: guard, ContentGuard: &contentguard.Scanner{Guard: guard, WrapEnabled: wrap}}
}

func jsonKeys(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertBoundary(t *testing.T, keys map[string]any, want bool) {
	t.Helper()
	_, untrusted := keys["untrustedContent"]
	notice, _ := keys["idpiNotice"].(string)
	if want && (!untrusted || notice != idpiNoticeText) {
		t.Fatalf("boundary missing: untrustedContent=%v idpiNotice=%q", keys["untrustedContent"], notice)
	}
	if !want && (untrusted || notice != "") {
		t.Fatalf("boundary present with wrapping off: %v", keys)
	}
}

// The boundary is a config decision, never a scan verdict: on whenever wrapping
// is configured, off otherwise, and idpiWarning is the separate advisory.
func TestTheTrustBoundaryFollowsTheWrapContentGate(t *testing.T) {
	for _, tc := range []struct {
		name          string
		enabled, wrap bool
		want          bool
	}{
		{"enabled and wrapping", true, true, true},
		{"enabled without wrapping", true, false, false},
		{"wrapping without idpi", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := boundaryHandlers(tc.enabled, tc.wrap)
			resp := map[string]any{"status": "ok"}
			h.trustBoundary().attach(resp)
			assertBoundary(t, resp, tc.want)
			assertBoundary(t, jsonKeys(t, buildFindResponse(findRequest{}, semantic.FindResult{}, "", h.trustBoundary(), time.Now())), tc.want)
			assertBoundary(t, jsonKeys(t, inspectResponse{trustBoundary: h.inspectTrustBoundary(inspectKindHTML)}), tc.want)
			assertBoundary(t, jsonKeys(t, inspectResponse{trustBoundary: h.inspectTrustBoundary(inspectKindStyles)}), tc.want)
			for _, kind := range []inspectKind{inspectKindTitle, inspectKindURL} {
				assertBoundary(t, jsonKeys(t, inspectResponse{trustBoundary: h.inspectTrustBoundary(kind)}), false)
			}
		})
	}
}

// /text delivers the same boundary in-band, wrapped around the prose, which is
// why it gains no keys; the wrapper must actually appear when wrapping is on.
func TestTextWrapsTheBoundaryInBand(t *testing.T) {
	on := boundaryHandlers(true, true).ContentGuard.Scan("Revenue is up this quarter.", "https://example.com")
	if !strings.Contains(on.Text, "UNTRUSTED") || !strings.Contains(on.Text, "Revenue is up") {
		t.Fatalf("wrapping on did not wrap the text: %q", on.Text)
	}
	off := boundaryHandlers(true, false).ContentGuard.Scan("Revenue is up this quarter.", "https://example.com")
	if off.Text != "Revenue is up this quarter." {
		t.Fatalf("wrapping off changed the text: %q", off.Text)
	}
}

// Every producer that publishes idpiWarning delivers a boundary by one of three
// mechanisms, and the exclusions from the keyed form carry their reason: a
// producer added later must attach the boundary or be recorded here.
func TestEveryIDPIWarningProducerDeliversATrustBoundary(t *testing.T) {
	inBand := map[string]string{
		"text.go": "ContentGuard.Scan(",
		"pdf.go":  "SetHeaders(",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	producers := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "snapshot_idpi.go" {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		if !strings.Contains(text, `"idpiWarning"`) && !strings.Contains(text, "IDPIWarning") {
			continue
		}
		producers++
		if strings.Contains(text, "trustBoundary()") || strings.Contains(text, "inspectTrustBoundary(") {
			continue
		}
		if marker, recorded := inBand[file]; recorded {
			if !strings.Contains(text, marker) {
				t.Errorf("%s is recorded as delivering the boundary through %s but no longer does", file, marker)
			}
			continue
		}
		t.Errorf("%s publishes idpiWarning but attaches no trust boundary; call trustBoundary() or record its mechanism here", file)
	}
	if producers < 5 {
		t.Fatalf("only %d producers found; the census would prove little", producers)
	}
}
