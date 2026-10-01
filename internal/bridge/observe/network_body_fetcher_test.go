package observe

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// The behavioural proof of this rule is browser-backed, and the lightweight CI run
// installs no browser — so the wiring it depends on gets a source-level guard that runs
// everywhere. Two fetchers existed; the one the retention path used called cdproto's
// typed GetResponseBodyParams.Do, which base64-decodes inside the dependency and returns
// []byte, so it had no flag left to report and hardcoded false.
func TestOneResponseBodyFetcherThatReportsTheCDPFlag(t *testing.T) {
	const fetcher = "func GetResponseBody(ctx context.Context, requestID string) (string, bool, error)"

	observe := srccensus.Load(t, ".", 10)
	owner := observe.MustFunc(t, "GetResponseBody")
	body := observe.FileSource(owner)

	if !strings.Contains(body, fetcher) {
		t.Fatalf("%s no longer declares %q; this census would guard nothing", owner.File, fetcher)
	}
	if !strings.Contains(body, "base64Encoded = resp.Base64Encoded") {
		t.Error("the fetcher no longer reads base64Encoded off the CDP result, so the flag is not the browser's answer")
	}
	if strings.Contains(body, "base64Encoded = false") {
		t.Error("network.go assigns base64Encoded a constant; a binary body then retains as raw bytes and JSON-encodes to U+FFFD")
	}
	if strings.Contains(body, "network.GetResponseBody(") {
		t.Error("network.go fetches a body through cdproto's typed GetResponseBody, which decodes inside the dependency and destroys the flag")
	}
	if !strings.Contains(body, "var fetchResponseBody = GetResponseBody") {
		t.Error("the retention seam no longer defaults to GetResponseBody, so retention can drive a different fetcher than export")
	}

	// The on-demand and export call sites must reach the same fetcher. A second
	// implementation reachable from handlers is how the two paths came to disagree about
	// the same request in the first place.
	var handlerCalls int
	for name, handlerBody := range srccensus.Load(t, filepath.Join("..", "..", "handlers"), 80).Sources() {
		if strings.Contains(handlerBody, "GetResponseBodyDirect") {
			t.Errorf("handlers/%s reaches GetResponseBodyDirect instead of the single fetcher", name)
		}
		for _, line := range strings.Split(handlerBody, "\n") {
			if !strings.Contains(line, "GetResponseBody(") {
				continue
			}
			handlerCalls++
			if !strings.Contains(line, "bridge.GetResponseBody(") {
				t.Errorf("handlers/%s fetches a body outside the single fetcher: %s", name, strings.TrimSpace(line))
			}
		}
		assertFlagNotDiscarded(t, "handlers/"+name, handlerBody)
	}
	if handlerCalls == 0 {
		t.Error("no body-fetch call site found in handlers; the call-site half of this census guards nothing")
	}

	// The same rule inside this package: one owner is not enough if a wrapper re-creates
	// the old behaviour by throwing the flag away.
	assertFlagNotDiscarded(t, owner.File, body)
}

// assertFlagNotDiscarded bans a caller that takes the body and drops the flag. Routing
// every site through one fetcher does not fix anything on its own — a thin wrapper that
// reads the body and returns a constant false is byte-for-byte the defect this card is
// about, wearing a different name, and a ban keyed on the fetcher's NAME cannot see it.
func assertFlagNotDiscarded(t *testing.T, label, code string) {
	t.Helper()
	for _, line := range strings.Split(code, "\n") {
		if !strings.Contains(line, "GetResponseBody(") {
			continue
		}
		if strings.Contains(line, ", _, err") || strings.Contains(line, ", _, _") {
			t.Errorf("%s discards the base64Encoded flag: %s", label, strings.TrimSpace(line))
		}
	}
}
