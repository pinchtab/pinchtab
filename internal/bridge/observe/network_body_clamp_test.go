package observe

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pinchtab/pinchtab/internal/sanitize"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// multiByteBody's cut offsets fall inside a 3-byte and a 4-byte character, so a
// raw byte cut orphans continuation bytes and the JSON encoder emits U+FFFD.
const multiByteBody = `{"note":"Docs — guide 🎯 done","tail":"padding padding padding"}`

func retainWithBody(t *testing.T, body string, base64Encoded bool, maxBytes int, perTab int64) NetworkEntry {
	t.Helper()

	orig := fetchResponseBody
	fetchResponseBody = func(context.Context, string) (string, bool, error) {
		return body, base64Encoded, nil
	}
	t.Cleanup(func() { fetchResponseBody = orig })

	nm := NewNetworkMonitor(16)
	nm.ConfigureBodyRetention(true, maxBytes)
	nm.retainBodyMaxPerTab = perTab

	buf := NewNetworkBuffer(16)
	buf.Add(NetworkEntry{RequestID: "req-1", URL: "https://example.test/", BodyPending: true})
	nm.maybeRetainBody(context.Background(), buf, "req-1")

	entry, ok := buf.Get("req-1")
	if !ok {
		t.Fatal("entry disappeared from the buffer")
	}
	return entry
}

func assertBodyIsCleanUTF8(t *testing.T, source, got string) {
	t.Helper()
	if !utf8.ValidString(got) {
		t.Fatalf("retained body is not valid UTF-8: %q", got)
	}
	if strings.Count(got, "�") > strings.Count(source, "�") {
		t.Fatalf("retained body gained replacement characters: %q", got)
	}
	assertRetainedIsPrefix(t, source, got)
}

// The clamp exists to keep characters the response never contained out of the
// payload, and valid-UTF-8-plus-no-new-U+FFFD does not say that: a retained body
// of ".." passes both. A retained body is machine-read and bodyTruncated already
// carries the signal, so the only honest shape is a byte-exact prefix.
func assertRetainedIsPrefix(t *testing.T, source, got string) {
	t.Helper()
	if len(got) > len(source) {
		t.Fatalf("retained body is longer than the source: %d > %d bytes (%q)", len(got), len(source), got)
	}
	if got != source[:len(got)] {
		t.Fatalf("retained body is not a byte-exact prefix of the response:\n got %q\nwant %q", got, source[:len(got)])
	}
}

// The two cut sites are the opt-in retainBodyMaxBytes limit and the always-on
// per-tab budget; the second is reachable without opting in at all.
func TestRetainTextBodyCutsOnRuneBoundariesAtBothSites(t *testing.T) {
	for offset := 1; offset < len(multiByteBody); offset++ {
		byMaxBytes := retainWithBody(t, multiByteBody, false, offset, 1<<20)
		assertBodyIsCleanUTF8(t, multiByteBody, byMaxBytes.ResponseBody)
		if !byMaxBytes.BodyRetained {
			t.Fatalf("maxBytes=%d: text body must still be retained", offset)
		}

		byBudget := retainWithBody(t, multiByteBody, false, 0, int64(offset))
		assertBodyIsCleanUTF8(t, multiByteBody, byBudget.ResponseBody)
		if !byBudget.BodyRetained {
			t.Fatalf("budget=%d: text body must still be retained", offset)
		}
	}
}

func TestRetainTextBodyMarksTruncatedNotSkipped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		perTab int64
	}{
		{"retainBodyMaxBytes limit", 16, 1 << 20},
		{"per-tab remaining budget", 0, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := retainWithBody(t, multiByteBody, false, tc.limit, tc.perTab)

			if !entry.BodyTruncated {
				t.Fatalf("a cut-but-usable text body must report bodyTruncated: %+v", entry)
			}
			if entry.BodySkipped || entry.BodySkipReason != "" {
				t.Fatalf("a truncated text body must not also report bodySkipped: skipped=%v reason=%q", entry.BodySkipped, entry.BodySkipReason)
			}
			if entry.ResponseBody == "" {
				t.Fatal("a truncated text body must still carry the part that fit")
			}
		})
	}
}

func TestRetainBase64BodyOverBudgetIsSkippedNotCorrupted(t *testing.T) {
	raw := strings.Repeat("binary-payload-", 40)
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))

	for _, tc := range []struct {
		name       string
		limit      int
		perTab     int64
		wantReason string
	}{
		{"retainBodyMaxBytes limit", 32, 1 << 20, "base64 body exceeds retention limit"},
		{"per-tab remaining budget", 0, 32, "base64 body exceeds retention budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := retainWithBody(t, encoded, true, tc.limit, tc.perTab)

			if entry.ResponseBody != "" {
				if _, err := base64.StdEncoding.DecodeString(entry.ResponseBody); err != nil {
					t.Fatalf("returned an undecodable base64 fragment: %v (%q)", err, entry.ResponseBody)
				}
			}
			if !entry.BodySkipped {
				t.Fatalf("an over-budget base64 body must report bodySkipped: %+v", entry)
			}
			if entry.BodySkipReason != tc.wantReason {
				t.Fatalf("skip reason = %q, want %q", entry.BodySkipReason, tc.wantReason)
			}
			if entry.BodyTruncated {
				t.Fatal("a dropped body must not be reported as truncated: the client would parse a body that is not there")
			}
		})
	}
}

func TestRetainBodiesUnderBudgetAreUnchanged(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("small binary payload"))

	for _, tc := range []struct {
		name          string
		body          string
		base64Encoded bool
	}{
		{"text", multiByteBody, false},
		{"base64", encoded, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := retainWithBody(t, tc.body, tc.base64Encoded, len(tc.body)+1, 1<<20)

			if entry.ResponseBody != tc.body {
				t.Fatalf("body under the budget was altered:\n got %q\nwant %q", entry.ResponseBody, tc.body)
			}
			if entry.BodyTruncated || entry.BodySkipped {
				t.Fatalf("body under the budget must be neither truncated nor skipped: %+v", entry)
			}
			if entry.Base64Encoded != tc.base64Encoded {
				t.Fatalf("base64Encoded = %v, want %v", entry.Base64Encoded, tc.base64Encoded)
			}
		})
	}
}

// firstRuneWideBody's first character is 4 bytes, so budgets 1..3 leave no whole
// rune to keep. multiByteBody cannot reach that case — it opens with "{" — which
// is why the fabricated-suffix bug survived a loop over every offset of it.
const firstRuneWideBody = `🎯 target reached`

func TestRetainTextBodyIsDroppedWhenNoWholeRuneFits(t *testing.T) {
	for _, site := range []struct {
		name       string
		limitFor   func(int) (int, int64)
		wantReason string
	}{
		{
			name:       "retainBodyMaxBytes limit",
			limitFor:   func(limit int) (int, int64) { return limit, 1 << 20 },
			wantReason: "retention limit is smaller than the body's first character",
		},
		{
			name:       "per-tab remaining budget",
			limitFor:   func(limit int) (int, int64) { return 0, int64(limit) },
			wantReason: "retention budget is smaller than the body's first character",
		},
	} {
		t.Run(site.name, func(t *testing.T) {
			for limit := 1; limit < 4; limit++ {
				maxBytes, perTab := site.limitFor(limit)
				entry := retainWithBody(t, firstRuneWideBody, false, maxBytes, perTab)

				if entry.ResponseBody != "" {
					t.Errorf("limit=%d: kept %q of a body whose first character does not fit", limit, entry.ResponseBody)
				}
				if entry.BodyRetained {
					t.Errorf("limit=%d: reported bodyRetained with nothing retained: %+v", limit, entry)
				}
				if entry.BodyTruncated {
					t.Errorf("limit=%d: reported bodyTruncated for a body that was dropped, not cut", limit)
				}
				if !entry.BodySkipped {
					t.Errorf("limit=%d: a dropped body must report bodySkipped: %+v", limit, entry)
				}
				if entry.BodySkipReason != site.wantReason {
					t.Errorf("limit=%d: skip reason = %q, want %q", limit, entry.BodySkipReason, site.wantReason)
				}
			}

			// One byte more and the whole first rune fits, so the body comes back as a
			// prefix — the drop above is the budget being too small, not a dead branch.
			maxBytes, perTab := site.limitFor(4)
			entry := retainWithBody(t, firstRuneWideBody, false, maxBytes, perTab)
			if entry.ResponseBody != "🎯" {
				t.Fatalf("limit=4: body = %q, want the first character alone", entry.ResponseBody)
			}
			if !entry.BodyRetained || !entry.BodyTruncated || entry.BodySkipped {
				t.Fatalf("limit=4: want retained+truncated and not skipped: %+v", entry)
			}
			assertRetainedIsPrefix(t, firstRuneWideBody, entry.ResponseBody)
		})
	}
}

// PostData is the request body — the same field class as the response body, and it
// went through the same display helper. It has no truncated flag of its own, so a
// fabricated ellipsis there is indistinguishable from content the client sent.
func TestPostDataIsCutToAByteExactPrefix(t *testing.T) {
	long := strings.Repeat("a", maxNetworkPostDataBytes-2) + "🎯" + "tail"

	got := normalizeNetworkEntry(NetworkEntry{PostData: long}).PostData

	if len(got) > maxNetworkPostDataBytes {
		t.Fatalf("PostData = %d bytes, want at most %d", len(got), maxNetworkPostDataBytes)
	}
	if got != long[:len(got)] {
		t.Fatalf("PostData is not a byte-exact prefix of what was sent: tail = %q", got[max(0, len(got)-8):])
	}
	if strings.Contains(got, sanitize.TruncationSuffix) && !strings.Contains(long, sanitize.TruncationSuffix) {
		t.Fatalf("PostData gained the display truncation suffix: tail = %q", got[max(0, len(got)-8):])
	}
}

// The display fields are the case the ellipsis is right for: a human reads a URL
// or an error string and the marker is the point. This pins the split both ways —
// the two body fields must not reach the ellipsis variant, and the metadata fields
// must not lose it.
func TestBodyClampsAndDisplayFieldsUseTheirOwnHelper(t *testing.T) {
	pkg := srccensus.Load(t, ".", 10)
	clamp := pkg.Source(pkg.MustFunc(t, "clampRetainedBody"))
	src := pkg.FileSource(pkg.MustFunc(t, "normalizeNetworkEntry"))
	if !strings.Contains(clamp, "sanitize.TruncateUTF8BytesExact(") {
		t.Error("clampRetainedBody no longer cuts with the suffix-free helper")
	}
	if strings.Contains(clamp, "sanitize.TruncateUTF8BytesWithEllipsis(") {
		t.Error("clampRetainedBody reaches the ellipsis variant again: the retained body would carry characters the response never sent")
	}
	if !strings.Contains(src, "clampRetainedBody(entry.PostData,") {
		t.Error("PostData no longer goes through clampRetainedBody: a second clamp beside it drifts, and PostData loses the drop-with-reason cases the response body already handles")
	}
	// The scope this site passes is the wording an operator reads in the drop reason, and the
	// branch that emits it cannot be reached through normalizeNetworkEntry — the cap is a
	// constant far larger than any rune — so no behavioural test can see which scope
	// production hands over. Read it from the source instead: passing a retention scope here
	// would tell an operator the RESPONSE budget refused their request body.
	if !strings.Contains(src, "maxNetworkPostDataBytes, postDataLimitScope)") {
		t.Error("the PostData clamp no longer passes postDataLimitScope, so its drop reason would name a budget that did not refuse the body")
	}
	if strings.Contains(src, "entry.PostData = sanitize.") {
		t.Error("PostData cuts with a sanitize helper directly again, bypassing the one payload clamp")
	}

	displayFields := []string{"entry.URL", "entry.Method", "entry.ResourceType", "entry.StatusText", "entry.MimeType", "entry.Error"}
	for _, field := range displayFields {
		if !strings.Contains(src, field+" = sanitize.TruncateUTF8BytesWithEllipsis(") {
			t.Errorf("%s no longer uses the ellipsis variant; a human reads it and the marker is the signal", field)
		}
	}
	headerClamps := []string{"key = sanitize.TruncateUTF8BytesWithEllipsis(", "value = sanitize.TruncateUTF8BytesWithEllipsis("}
	for _, headerClamp := range headerClamps {
		if !strings.Contains(src, headerClamp) {
			t.Errorf("header clamp %q no longer uses the ellipsis variant", headerClamp)
		}
	}

	// The checks above ask whether each site this test KNOWS about picked the right
	// helper, which cannot see a site it does not know about — and an unlisted field
	// taking the ellipsis by default is how the defect this file exists for arrived.
	accounted := len(displayFields) + len(headerClamps) + 1 // the one payload clamp, which both the retained body and PostData go through
	if sites := len(pkg.CallsAllowingNone("sanitize.TruncateUTF8BytesWithEllipsis")) + len(pkg.CallsAllowingNone("sanitize.TruncateUTF8BytesExact")); sites != accounted {
		t.Errorf("%s cuts a field at %d sites but this test classifies %d — a new field is picking a truncation policy nobody reviewed. Add it above: the ellipsis variant if a human reads it, the suffix-free one if it is machine-read and a marker would be fabricated content",
			pkg.Dir(), sites, accounted)
	}
}
