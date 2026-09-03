package cdptk

import (
	"errors"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestWindowsViewportCaptureUsesSurface(t *testing.T) {
	if !captureFromSurface("windows", false, nil) {
		t.Fatal("Windows viewport capture used the read-the-view path")
	}
}

// The property is that a REFUSAL of the read-the-view fast path is handled, whatever the
// clip — not that an unclipped capture works. A browser fixture cannot pin this: the
// refusal depends on whether the renderer's view is available, which no test can withhold
// on demand, and when it is available both flags return a frame. So the capture is
// injected and the flag it was called with is the assertion.
func TestCaptureRetriesFromSurfaceOnlyWhenTheFastPathWasRefused(t *testing.T) {
	refusal := errors.New("Unable to capture screenshot (-32000)")

	for _, tc := range []struct {
		name        string
		fromSurface bool
		errs        []error
		wantCalls   []bool
		wantErr     error
	}{
		{
			name:      "the fast path is refused, so the surface path answers",
			errs:      []error{refusal, nil},
			wantCalls: []bool{false, true},
		},
		{
			name:      "a healthy fast path costs no second call",
			errs:      []error{nil},
			wantCalls: []bool{false},
		},
		{
			name:      "any other error is a real failure and surfaces",
			errs:      []error{errors.New("target closed")},
			wantCalls: []bool{false},
			wantErr:   errors.New("target closed"),
		},
		{
			name:        "a refused surface capture has no faster path to fall back from",
			fromSurface: true,
			errs:        []error{refusal},
			wantCalls:   []bool{true},
			wantErr:     refusal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []bool
			buf, err := CaptureWithSurfaceFallback(tc.fromSurface, func(fromSurface bool) ([]byte, error) {
				calls = append(calls, fromSurface)
				return []byte("frame"), tc.errs[len(calls)-1]
			})

			if len(calls) != len(tc.wantCalls) {
				t.Fatalf("capture called with %v, want %v", calls, tc.wantCalls)
			}
			for i, want := range tc.wantCalls {
				if calls[i] != want {
					t.Errorf("call %d used fromSurface=%v, want %v", i+1, calls[i], want)
				}
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want the capture to succeed", err)
				}
				if string(buf) != "frame" {
					t.Errorf("buf = %q, want the captured frame", buf)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// The refusal is matched on the message because CDP sends no code the client can read, so
// the string is load-bearing rather than cosmetic. This pins it against the error the
// browser actually produces, which is what the capture sites saw on a real host.
func TestTheRefusalIsRecognisedInTheErrorTheBrowserSends(t *testing.T) {
	if !strings.Contains("screenshot: Unable to capture screenshot (-32000)", captureRefusal) {
		t.Errorf("the refusal constant %q does not appear in the error CDP sends, so the fallback never fires", captureRefusal)
	}
}

// The wiring, which nothing else can pin. The refusal depends on whether the renderer's
// view is available, and no test can withhold it — so a capture site that stops calling the
// fallback keeps every browser-backed test green and only fails on the host that was
// already failing. A source census is the only guard available, and without it this fix is
// unwired the first time someone rewrites a capture site.
func TestEveryCaptureSiteRoutesThroughTheSurfaceFallback(t *testing.T) {
	// No site is exempt today. An exemption records a capture that legitimately
	// answers to a different cadence — a polling frame loop rather than one
	// request — and must carry its reason rather than be silently skipped.
	exempt := map[string]string{}

	sites := 0
	for _, file := range srccensus.Tree(t, "../..", moduleFileFloor) {
		if !capturesAScreenshot(t, file.Name, file.Text) {
			continue
		}
		sites++
		if reason, ok := exempt[file.Name]; ok {
			if reason == "" {
				t.Errorf("%s is exempt with no reason recorded", file.Name)
			}
			continue
		}
		if !strings.Contains(file.Text, "CaptureWithSurfaceFallback") {
			t.Errorf("%s captures a screenshot without routing through CaptureWithSurfaceFallback, so a renderer that refuses the read-the-view path fails the whole capture there", file.Name)
		}
	}

	if sites <= len(exempt) {
		t.Fatalf("found %d capture sites against %d exemptions; the census matched almost nothing and would pass vacuously", sites, len(exempt))
	}
	for name := range exempt {
		found := false
		for _, file := range srccensus.Tree(t, "../..", moduleFileFloor) {
			if file.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is exempt but the walk never found it, so the exemption is stale", name)
		}
	}
}

const moduleFileFloor = 200

const cdprotoPage = "github.com/chromedp/cdproto/page"

// captureSpellings are the two ways this module reaches Page.captureScreenshot:
// cdproto's builder and chromedp's helper. Both land on the same CDP command and
// both need the fallback, but a census that knew only the builder could not see a
// helper call at all — which is how a raw capture lived on in an adapter that
// already held a bridge whose engine owns the rule.
var captureSpellings = map[string]string{
	cdprotoPage:                    ".CaptureScreenshot()",
	"github.com/chromedp/chromedp": ".CaptureScreenshot(",
}

// capturesAScreenshot resolves the file's OWN local name for cdproto/page before matching
// the call, rather than matching the literal "page.CaptureScreenshot()".
//
// A plain import alias defeats a literal scan: `cdp "…/cdproto/page"` then
// `cdp.CaptureScreenshot()` is the same call, compiles, and is invisible to it — and no
// vacuity floor can see that, because the three conventional sites still match and the
// count still passes. Reading the import makes every spelling of the call one case.
func capturesAScreenshot(t *testing.T, name, text string) bool {
	t.Helper()
	if !strings.Contains(text, ".CaptureScreenshot(") {
		return false
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), name, text, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		call, spelled := captureSpellings[path]
		if !spelled {
			continue
		}
		local := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			local = spec.Name.Name
		}
		if strings.Contains(text, local+call) {
			return true
		}
	}
	return false
}

// A clip must survive the retry: the fallback re-runs the same params, and re-running
// without the clip would answer the whole viewport while reporting success — the silent
// widening the clip-forces-surface rule exists to prevent.
func TestTheRetryDoesNotChangeWhatWasAskedForBesidesTheFlag(t *testing.T) {
	var seen []bool
	if _, err := CaptureWithSurfaceFallback(false, func(fromSurface bool) ([]byte, error) {
		seen = append(seen, fromSurface)
		if len(seen) == 1 {
			return nil, errors.New("Unable to capture screenshot (-32000)")
		}
		return []byte("frame"), nil
	}); err != nil {
		t.Fatalf("err = %v", err)
	}
	// The callback receives only the flag, so nothing else CAN differ between the two
	// attempts — that is the point of the seam, and this asserts the shape stays that way.
	if len(seen) != 2 || seen[0] || !seen[1] {
		t.Errorf("attempts = %v, want the fast path then the surface path", seen)
	}
}
