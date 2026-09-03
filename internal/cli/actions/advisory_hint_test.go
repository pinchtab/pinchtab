package actions

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/output"
)

// staleTabCmd drives the fallback that reports a dead tab — an occurrence hint,
// which must print on every occurrence — alongside the no-session advisory.
func navigateOverAStaleTab(t *testing.T, m *mockServer) func() {
	t.Helper()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","status":"ok"}`)
	return func() {
		cmd := staleTabCmd(t, "STALE123")
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", cmd)
		})
	}
}

func anonymousCaller(t *testing.T) {
	t.Helper()
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	// Advisories are remembered under the CLI state directory, so a test that does
	// not move it would silence itself on its second run and write to the machine
	// it runs on.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	output.ResetAdvisories()
	t.Cleanup(output.ResetAdvisories)
}

// The no-session hint describes a steady state the caller may have chosen, so a
// second navigation has nothing new to say. The occurrence hint beside it is the
// control: it reports what happened on this navigation and must repeat.
func TestTheNoSessionAdvisoryPrintsOncePerRunWhileOccurrenceHintsRepeat(t *testing.T) {
	anonymousCaller(t)
	m := newMockServer()
	defer m.close()
	nav := navigateOverAStaleTab(t, m)

	first := captureStderr(t, nav)
	second := captureStderr(t, nav)

	if !strings.Contains(first, cli.NoSessionHint) {
		t.Fatalf("first navigation printed no session advisory: %q", first)
	}
	if strings.Contains(second, cli.SessionCreateCommand) {
		t.Errorf("the advisory repeated on the second navigation of the same run: %q", second)
	}
	for i, out := range []string{first, second} {
		if !strings.Contains(out, "no longer exists") {
			t.Errorf("navigation %d dropped the dead-tab report; an occurrence hint must fire every time: %q", i+1, out)
		}
	}
}

// The switch is discoverable from the output it silences, and it is published
// exactly once — a per-navigation reminder would be the defect it fixes.
func TestTheFirstAdvisoryPublishesTheSilencingSwitchOnce(t *testing.T) {
	anonymousCaller(t)
	m := newMockServer()
	defer m.close()
	nav := navigateOverAStaleTab(t, m)

	first := captureStderr(t, nav)
	second := captureStderr(t, nav)

	if !strings.Contains(first, output.SilenceAdvisoryHint) {
		t.Fatalf("the first advisory did not say how to silence it: %q", first)
	}
	if !strings.HasSuffix(strings.TrimSpace(output.SilenceAdvisoryHint), output.HintsEnv+"="+output.HintsOff) {
		t.Errorf("the switch must end the line so it can be lifted verbatim: %q", output.SilenceAdvisoryHint)
	}
	if strings.Contains(second, output.SilenceAdvisoryHint) {
		t.Errorf("the silencing switch was published twice in one run: %q", second)
	}
}

// Silencing takes the advisory and only the advisory: a caller who has decided
// not to use sessions still needs to hear that the tab it asked for is gone.
func TestSilencingAdvisoriesKeepsOccurrenceHints(t *testing.T) {
	anonymousCaller(t)
	t.Setenv(output.HintsEnv, output.HintsOff)
	m := newMockServer()
	defer m.close()
	nav := navigateOverAStaleTab(t, m)

	out := captureStderr(t, nav)

	if strings.Contains(out, cli.SessionCreateCommand) {
		t.Errorf("the advisory printed with hints silenced: %q", out)
	}
	if strings.Contains(out, output.SilenceAdvisoryHint) {
		t.Errorf("the silencing switch was advertised to a caller who had already used it: %q", out)
	}
	if !strings.Contains(out, "no longer exists") {
		t.Errorf("silencing advisories also silenced the dead-tab report: %q", out)
	}
}
