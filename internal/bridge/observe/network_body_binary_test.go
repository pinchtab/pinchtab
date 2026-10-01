package observe

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// The retention path drove a fetcher that hardcoded base64Encoded to false, so a binary
// body was retained as raw bytes and JSON-encoded into U+FFFD soup, and the drop-and-mark
// branch that exists to prevent exactly that could never fire. Driven through the REAL
// fetcher against real image responses — the seam-based clamp test stayed green through
// all of it, because the seam supplied the flag production never set.
func TestBinaryResponseBodiesCarryTheCDPFlagOnEveryPath(t *testing.T) {
	small := pngBytes(t, 8)
	large := pngBytes(t, 96)
	// Multibyte on purpose: the limit can land mid-rune, which is what the rune-safe cut exists for.
	textBody := strings.Repeat("héllo wörld ", 100)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/small.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(small)
		case "/large.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(large)
		case "/body.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(textBody))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprintf(w, `<img src="/small.png"><img src="/large.png">
				<script>fetch("/body.txt");</script>`)
		}
	}))
	defer srv.Close()

	// Between the two images: the small one stays under it and is retained, the large one
	// is over and must be dropped rather than cut, since a base64 fragment is undecodable
	// in whole rather than only at the tail.
	limit := (base64Len(len(small)) + base64Len(len(large))) / 2
	if base64Len(len(small)) >= limit || base64Len(len(large)) <= limit {
		t.Fatalf("fixture does not straddle the limit: small=%d large=%d limit=%d",
			base64Len(len(small)), base64Len(len(large)), limit)
	}
	if len(textBody) <= limit {
		t.Fatalf("text fixture (%d) must also exceed the limit %d, or truncation is untested", len(textBody), limit)
	}

	ctx, cancel := browserContext(t)
	defer cancel()

	nm := NewNetworkMonitor(0)
	nm.ConfigureBodyRetention(true, limit)
	if err := nm.StartCapture(ctx, "tab1"); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/index.html")); err != nil {
		t.Fatal(err)
	}

	buf := nm.GetBuffer("tab1")
	entries := waitForSettledEntries(t, buf, "/small.png", "/large.png", "/body.txt")

	smallEntry := entries["/small.png"]
	if !smallEntry.BodyRetained {
		t.Fatalf("the under-budget image was not retained: %+v", skipSummary(smallEntry))
	}
	if !smallEntry.Base64Encoded {
		t.Error("the retained image reports base64Encoded=false, so a caller decodes U+FFFD instead of a PNG")
	}
	if decoded, err := base64.StdEncoding.DecodeString(smallEntry.ResponseBody); err != nil {
		t.Errorf("the retained image body is not decodable base64: %v", err)
	} else if !bytes.Equal(decoded, small) {
		t.Errorf("the retained image decodes to %d bytes, want the %d served", len(decoded), len(small))
	}
	if strings.ContainsRune(smallEntry.ResponseBody, '�') {
		t.Error("the retained image body carries U+FFFD; bytes were destroyed on the way in")
	}

	// The export/on-demand path and the retention path must agree about the same request:
	// they are one fetcher now, and this is what proves it at runtime rather than by grep.
	liveBody, liveBase64, err := GetResponseBody(ctx, smallEntry.RequestID)
	if err != nil {
		t.Fatalf("live fetch of the retained request failed: %v", err)
	}
	if liveBase64 != smallEntry.Base64Encoded {
		t.Errorf("live fetch reports base64Encoded=%v but retention recorded %v for the same request",
			liveBase64, smallEntry.Base64Encoded)
	}
	if !liveBase64 {
		t.Error("the on-demand path reports base64Encoded=false for an image/png response")
	}
	if liveBody != smallEntry.ResponseBody {
		t.Error("live fetch and retention returned different bodies for the same request")
	}

	largeEntry := entries["/large.png"]
	if largeEntry.BodyRetained || largeEntry.ResponseBody != "" {
		t.Errorf("the over-budget image was retained (%d bytes) instead of dropped; a base64 fragment is undecodable",
			len(largeEntry.ResponseBody))
	}
	if !largeEntry.BodySkipped {
		t.Error("the over-budget image is neither retained nor marked skipped, so a caller cannot tell it is absent")
	}
	if !strings.Contains(largeEntry.BodySkipReason, "base64 body exceeds") {
		t.Errorf("skip reason %q does not say a base64 body exceeded a budget", largeEntry.BodySkipReason)
	}

	// A text body is unaffected: still decoded, still flagged false, still cut rather than
	// dropped, and still cut on a rune boundary.
	textEntry := entries["/body.txt"]
	if !textEntry.BodyRetained {
		t.Fatalf("the text body was not retained: %+v", skipSummary(textEntry))
	}
	if textEntry.Base64Encoded {
		t.Error("the text body reports base64Encoded=true")
	}
	if !textEntry.BodyTruncated {
		t.Error("the over-limit text body was not marked truncated")
	}
	if !strings.HasPrefix(textBody, textEntry.ResponseBody) {
		t.Errorf("the retained text %q is not a prefix of the response", textEntry.ResponseBody)
	}
	if len(textEntry.ResponseBody) > limit {
		t.Errorf("the retained text is %d bytes, over the %d limit", len(textEntry.ResponseBody), limit)
	}
	if !utf8.ValidString(textEntry.ResponseBody) {
		t.Error("the retained text is not valid UTF-8; the cut split a rune")
	}
}

func browserContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	chromePath := testbrowser.Path(t)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	return ctx, func() {
		cancelTimeout()
		cancelCtx()
		cancelAlloc()
	}
}

// waitForSettledEntries waits until every named path has an entry whose body work is
// finished — retained, skipped or errored — so the assertions never read a pending row.
func waitForSettledEntries(t *testing.T, buf *NetworkBuffer, paths ...string) map[string]NetworkEntry {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for {
		found := map[string]NetworkEntry{}
		for _, entry := range buf.List(NetworkFilter{}) {
			for _, path := range paths {
				if strings.HasSuffix(entry.URL, path) && !entry.BodyPending && entry.Finished {
					found[path] = entry
				}
			}
		}
		if len(found) == len(paths) {
			return found
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d responses settled: %v", len(found), len(paths), found)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func skipSummary(entry NetworkEntry) string {
	return fmt.Sprintf("skipped=%v reason=%q error=%q base64=%v bytes=%d",
		entry.BodySkipped, entry.BodySkipReason, entry.BodyError, entry.Base64Encoded, len(entry.ResponseBody))
}

// base64Len is the encoded size CDP reports a body at, which is what the retention
// budget is measured against for a binary response.
func base64Len(raw int) int {
	return base64.StdEncoding.EncodedLen(raw)
}

// pngBytes builds a real PNG rather than arbitrary bytes, so Chrome classifies the
// response as binary and CDP sets base64Encoded — the whole subject of this test.
func pngBytes(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: uint8(x ^ y), A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
