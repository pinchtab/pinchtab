package apiclient

import (
	"context"
	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

// driftedTabBody is the REAL /url response for a tab that navigated off the
// allowlist, produced by driving the handler rather than hand-written here: a
// literal body would only prove this test's own fixture renders, which is the
// mistake that let the missing hint ship in the first place.
func driftedTabBody(t *testing.T) []byte {
	t.Helper()
	b := &driftedTabBridge{
		state: bridge.TabPolicyState{
			CurrentURL: "https://www.iana.org/help/example-domains",
			Blocked:    true,
			Reason:     `domain "www.iana.org" is not in the allowed list (security.allowedDomains)`,
			UpdatedAt:  time.Now(),
		},
	}
	h := handlers.New(b, &config.RuntimeConfig{
		ActionTimeout:  time.Second,
		AllowedDomains: []string{"example.com"},
		IDPI:           config.IDPIConfig{Enabled: true, StrictMode: true},
	}, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.HandleURL(rec, httptest.NewRequest("GET", "/url?tabId=tab1", nil))
	if rec.Code != 403 {
		t.Fatalf("expected the drifted tab to be blocked, got %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// The dead end this closes was a rendering outcome, not just a payload one: the
// user saw a 403 naming a config key and nothing else. Both lines must reach the
// terminal, the same way the navguard 409 already does.
func TestRenderAPIErrorBodyShowsDriftedTabHintAndBackRemedy(t *testing.T) {
	out := renderAPIErrorBody(403, driftedTabBody(t))

	for _, want := range []string{
		"Error 403: current tab blocked by IDPI",
		"💡 ",
		"Remedy: pinchtab back",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered error missing %q:\n%s", want, out)
		}
	}
	// The recovery line must not send the user at the allowlist: that widens the
	// isolation the operator set, and it does not unstick the tab either.
	if strings.Contains(out, "config set security.allowedDomains") {
		t.Errorf("a drifted tab must not be offered the allowlist widening:\n%s", out)
	}
}

// driftedTabBridge answers only what the /url read path touches before the domain
// policy refuses; everything else would be a bug in this test, not a real call.
type driftedTabBridge struct {
	bridge.BridgeAPI
	state bridge.TabPolicyState
}

func (b *driftedTabBridge) EnsureBrowser(*config.RuntimeConfig) error { return nil }

func (b *driftedTabBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (b *driftedTabBridge) GetTabPolicyState(string) (bridge.TabPolicyState, bool) {
	return b.state, true
}

func (b *driftedTabBridge) SetTabPolicyState(string, bridge.TabPolicyState) {}

// The read prelude asks every tab whether a JavaScript dialog is blocking it before
// it touches the page. The embedded BridgeAPI is nil, so the promoted method would
// panic rather than answer; this tab has no dialog.
func (b *driftedTabBridge) GetDialogManager() *bridgetabs.DialogManager { return nil }
