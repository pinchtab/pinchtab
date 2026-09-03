package actions

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/spf13/cobra"
)

func newNavigateCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("new-tab", false, "")
	cmd.Flags().Bool("block-images", false, "")
	cmd.Flags().Bool("block-ads", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().Float64("timeout", 0, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("print-tab-id", false, "")
	return cmd
}

func TestNavigateTimeoutIsSentAndClampedToTheAPICeiling(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  float64
		set   bool
	}{
		{name: "omitted"},
		{name: "explicit", value: "90.5", want: 90.5, set: true},
		{name: "over maximum", value: "121", want: httpx.MaxNavigationTimeout.Seconds(), set: true},
		{name: "infinite", value: "+Inf", want: httpx.MaxNavigationTimeout.Seconds(), set: true},
		{name: "not a number", value: "NaN", set: true},
		{name: "negative", value: "-1", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newNavigateCmd()
			if tc.set {
				if err := cmd.Flags().Set("timeout", tc.value); err != nil {
					t.Fatal(err)
				}
			}

			req := buildNavigateRequest("https://pinchtab.com", cmd)
			got, present := req.body["timeout"].(float64)
			if tc.want == 0 {
				if present {
					t.Fatalf("timeout = %v, want omitted", got)
				}
				return
			}
			if !present || got != tc.want {
				t.Fatalf("timeout = %v (present %v), want %v", got, present, tc.want)
			}
		})
	}
}

func TestNavigationHTTPClientPreservesCallerPolicy(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	base := &http.Client{
		Transport:     http.DefaultTransport,
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       time.Minute,
	}

	got := navigationHTTPClient(base, 90.5)
	if got == base {
		t.Fatal("navigation client mutated the shared client instead of cloning it")
	}
	if base.Timeout != time.Minute {
		t.Fatalf("shared client timeout changed to %v", base.Timeout)
	}
	if got.Transport != base.Transport || got.Jar != base.Jar || got.CheckRedirect == nil {
		t.Fatal("navigation client dropped caller transport, cookie jar, or redirect policy")
	}
	wantTimeout := 90*time.Second + 500*time.Millisecond + httpx.NavigationTransportGrace
	if got.Timeout != wantTimeout {
		t.Fatalf("timeout = %v, want %v", got.Timeout, wantTimeout)
	}
}

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestNavigate(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	if m.lastMethod != "POST" {
		t.Errorf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/navigate" {
		t.Errorf("expected /navigate, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["url"] != "https://pinchtab.com" {
		t.Errorf("expected url=https://pinchtab.com, got %v", body["url"])
	}
}

func TestNavigateReusesImplicitTabWhenItExists(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	cmd.Flags().Lookup("tab").DefValue = "ABC123"
	_ = cmd.Flags().Set("tab", "ABC123")
	cmd.Flags().Lookup("tab").Changed = false

	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)

	if len(m.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(m.requests))
	}
	if m.requests[0].Path != "/tabs/ABC123/navigate" {
		t.Fatalf("navigate path = %q, want /tabs/ABC123/navigate", m.requests[0].Path)
	}
}

func TestNavigateFallsBackToNewTabForStaleImplicitTab(t *testing.T) {
	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","status":"ok"}`)
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	cmd.Flags().Lookup("tab").DefValue = "STALE123"
	_ = cmd.Flags().Set("tab", "STALE123")
	cmd.Flags().Lookup("tab").Changed = false

	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)

	if len(m.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(m.requests))
	}
	if m.requests[0].Path != "/tabs/STALE123/navigate" {
		t.Fatalf("first request path = %q, want /tabs/STALE123/navigate", m.requests[0].Path)
	}
	if m.requests[1].Path != "/navigate" {
		t.Fatalf("navigate path = %q, want /navigate", m.requests[1].Path)
	}
}

func TestBuildNavigateRequestDoesNotFallbackForExplicitTab(t *testing.T) {
	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("tab", "EXPLICIT123")

	req := buildNavigateRequest("https://pinchtab.com", cmd)

	if req.path != "/tabs/EXPLICIT123/navigate" {
		t.Fatalf("path = %q, want /tabs/EXPLICIT123/navigate", req.path)
	}
	if req.fallbackOnNotFound {
		t.Fatal("explicit --tab should not fallback on 404")
	}
}

func TestNavigateWithAllFlags(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("new-tab", "true")
	_ = cmd.Flags().Set("block-images", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["newTab"] != true {
		t.Error("expected newTab=true")
	}
	if body["blockImages"] != true {
		t.Error("expected blockImages=true")
	}
}

func TestNavigateWithBlockAds(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("block-ads", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["blockAds"] != true {
		t.Error("expected blockAds=true")
	}
}

func TestNavigateDismissBanners(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["dismissBanners"] != true {
		t.Errorf("expected dismissBanners=true in body, got %v", body["dismissBanners"])
	}
}

func TestReloadDismissBannersAppendsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Reload(client, m.base(), "", cmd)
	if m.lastPath != "/reload" {
		t.Errorf("expected /reload path, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestBackDismissBannersAppendsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Back(client, m.base(), "", cmd)
	if m.lastPath != "/back" {
		t.Errorf("expected /back path, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestForwardDismissBannersAppendsQueryWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	_ = cmd.Flags().Set("tab", "TAB1")
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Forward(client, m.base(), "", cmd)
	if m.lastPath != "/tabs/TAB1/forward" {
		t.Errorf("expected /tabs/TAB1/forward, got %q", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "dismissBanners=true") {
		t.Errorf("expected dismissBanners=true in query, got %q", m.lastQuery)
	}
}

func TestReloadWithoutDismissBannersOmitsQuery(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHistoryCmd()
	Reload(client, m.base(), "", cmd)
	if m.lastQuery != "" {
		t.Errorf("expected empty query, got %q", m.lastQuery)
	}
}

func TestNavigatePrintTabID(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()
	client := m.server.Client()

	cmd := newNavigateCmd()
	_ = cmd.Flags().Set("print-tab-id", "true")

	out := captureStdout(t, func() {
		Navigate(client, m.base(), "", "https://pinchtab.com", cmd)
	})
	got := strings.TrimSpace(out)
	if got != "ABC123" {
		t.Errorf("stdout = %q, want exactly the tab ID so $(pinchtab nav URL) stays usable", got)
	}
}

func TestNavigateNoSessionHintNamesPrerequisiteAndCommand(t *testing.T) {
	anonymousCaller(t)

	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", newNavigateCmd())
		})
	})

	if !strings.Contains(stderr, cli.NoSessionHint) {
		t.Fatalf("hint = %q, want the shared cli.NoSessionHint", stderr)
	}
	// Following the hint top to bottom must not dead-end in "agent sessions are
	// not enabled on this server", so it has to carry the server-side
	// prerequisite as well as the command.
	if !strings.Contains(cli.NoSessionHint, "sessions.agent.enabled = true") {
		t.Errorf("hint does not name the server-side prerequisite: %q", cli.NoSessionHint)
	}
	if !strings.Contains(cli.NoSessionHint, cli.SessionCreateCommand) {
		t.Errorf("hint does not carry the create command: %q", cli.NoSessionHint)
	}

	// The hint decision reads nothing from the server: navigate stays one request.
	if len(m.requests) != 1 || m.requests[0].Path != "/navigate" {
		t.Fatalf("requests = %+v, want exactly the navigate call", m.requests)
	}
}

func TestNavigateIdentifiedCallerPrintsNoSessionHint(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "ses_something")

	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", newNavigateCmd())
		})
	})

	if strings.Contains(stderr, cli.SessionCreateCommand) {
		t.Fatalf("stderr = %q, want no session hint for an identified caller", stderr)
	}
}

func stdoutTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	old := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return isTerminal }
	t.Cleanup(func() { stdoutIsTerminal = old })
}

// The landed URL is the cheap signal that a redirect, login wall or error page
// intervened, and the server already returns it.
func TestNavigateReportsTheLandedURLAtATerminal(t *testing.T) {
	stdoutTerminal(t, true)
	m := newMockServer()
	m.response = `{"tabId":"ABC123","title":"Example Domain","url":"https://example.com/"}`
	defer m.close()

	out := captureStdout(t, func() {
		Navigate(m.server.Client(), m.base(), "", "https://httpbin.org/redirect-to?url=https://example.com/", newNavigateCmd())
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want the tab ID and the landed URL", out)
	}
	if lines[0] != "ABC123" {
		t.Errorf("first line = %q, want the tab ID", lines[0])
	}
	if lines[1] != "https://example.com/" {
		t.Errorf("second line = %q, want the landed URL, not the requested one", lines[1])
	}
}

// TAB=$(pinchtab nav URL) captures every line, so a second line would break it.
// Both the explicit flag and a non-terminal stdout must stay single-line.
func TestNavigatePrintsOnlyTheTabIDWhenCaptured(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal bool
		flag     bool
	}{
		{name: "stdout is not a character device", terminal: false},
		{name: "print-tab-id at a terminal", terminal: true, flag: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdoutTerminal(t, tc.terminal)

			m := newMockServer()
			m.response = `{"tabId":"ABC123","url":"https://example.com/"}`
			defer m.close()

			cmd := newNavigateCmd()
			if tc.flag {
				_ = cmd.Flags().Set("print-tab-id", "true")
			}
			out := captureStdout(t, func() {
				Navigate(m.server.Client(), m.base(), "", "https://example.com", cmd)
			})

			if got := strings.TrimSpace(out); got != "ABC123" {
				t.Errorf("stdout = %q, want exactly the tab ID so $(pinchtab nav URL) stays usable", got)
			}
		})
	}
}

// back, forward and reload share one server handler returning {"tabId","url"}, and
// none of them has a tab ID on stdout to protect — so unlike nav they report the
// landed URL through a pipe as well. The name says "through a pipe" because that
// asymmetry is the thing a later reader might "tidy" in either direction.
func TestHistoryNavigationPrintsTheLandedURLThroughAPipe(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*http.Client, string, string, *cobra.Command)
		path string
	}{
		{name: "back", run: Back, path: "/back"},
		{name: "forward", run: Forward, path: "/forward"},
		{name: "reload", run: Reload, path: "/reload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			m.response = `{"tabId":"ABC123","url":"https://example.com/landed"}`
			defer m.close()

			stdoutTerminal(t, false)

			out := captureStdout(t, func() {
				tc.run(m.server.Client(), m.base(), "", newHistoryCmd())
			})

			if m.lastPath != tc.path {
				t.Fatalf("path = %q, want %q", m.lastPath, tc.path)
			}
			if got := strings.TrimSpace(out); got != "https://example.com/landed" {
				t.Errorf("stdout = %q, want the landed URL even when stdout is not a terminal", got)
			}
		})
	}
}

// A response without a url must stay terse rather than print a blank line: OK
// for the history commands, the bare tab ID for nav.
func TestLandingReportDegradesWithoutAURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*http.Client, string, string, *cobra.Command)
	}{
		{name: "back", run: Back},
		{name: "forward", run: Forward},
		{name: "reload", run: Reload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			m.response = `{"tabId":"ABC123"}`
			defer m.close()

			out := captureStdout(t, func() {
				tc.run(m.server.Client(), m.base(), "", newHistoryCmd())
			})
			if got := strings.TrimSpace(out); got != "OK" {
				t.Errorf("stdout = %q, want the terse OK", got)
			}
			if strings.Contains(out, "\n\n") {
				t.Errorf("stdout = %q, want no blank line", out)
			}
		})
	}

	t.Run("navigate", func(t *testing.T) {
		stdoutTerminal(t, true)
		m := newMockServer()
		m.response = `{"tabId":"ABC123"}`
		defer m.close()

		out := captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://example.com", newNavigateCmd())
		})
		if got := strings.TrimSpace(out); got != "ABC123" {
			t.Errorf("stdout = %q, want just the tab ID", got)
		}
		if strings.Contains(out, "\n\n") {
			t.Errorf("stdout = %q, want no blank line", out)
		}
	})
}

// --json is the machine contract: the raw response body for all four commands, with
// no landed-URL line added.
func TestJSONOutputIsTheRawResponseForAllFour(t *testing.T) {
	const response = `{"tabId":"ABC123","url":"https://example.com/landed"}`
	// DoPost pretty-prints the decoded body, so that is what --json must equal.
	const want = "{\n  \"tabId\": \"ABC123\",\n  \"url\": \"https://example.com/landed\"\n}"

	t.Run("navigate", func(t *testing.T) {
		stdoutTerminal(t, true)
		m := newMockServer()
		m.response = response
		defer m.close()

		cmd := newNavigateCmd()
		cmd.Flags().Bool("json", false, "")
		_ = cmd.Flags().Set("json", "true")
		out := captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://example.com", cmd)
		})
		if got := strings.TrimSpace(out); got != want {
			t.Errorf("stdout = %q, want the response body alone %q", got, want)
		}
	})

	for _, tc := range []struct {
		name string
		run  func(*http.Client, string, string, *cobra.Command)
	}{
		{name: "back", run: Back},
		{name: "forward", run: Forward},
		{name: "reload", run: Reload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdoutTerminal(t, true)
			m := newMockServer()
			m.response = response
			defer m.close()

			cmd := newHistoryCmd()
			_ = cmd.Flags().Set("json", "true")
			out := captureStdout(t, func() {
				tc.run(m.server.Client(), m.base(), "", cmd)
			})
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("stdout = %q, want the response body alone %q", got, want)
			}
		})
	}
}

// nav gained --text; the shared tail must actually run it.
func TestNavigateTextFetchesPageText(t *testing.T) {
	stdoutTerminal(t, true)
	m := newMockServer()
	m.response = `{"tabId":"ABC123","url":"https://example.com/"}`
	m.responses["GET /tabs/ABC123/text"] = mockResponse{statusCode: 200, body: `{"text":"PAGE TEXT"}`}
	defer m.close()

	cmd := newNavigateCmd()
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	_ = cmd.Flags().Set("text", "true")

	out := captureStdout(t, func() {
		Navigate(m.server.Client(), m.base(), "", "https://example.com", cmd)
	})

	if !strings.Contains(out, "PAGE TEXT") {
		t.Errorf("stdout = %q, want the page text after navigation", out)
	}
}

// The reported defect: a stale saved tab id 404s, the CLI silently re-posts
// without it, and the server — following its published anonymous contract —
// opens a SECOND tab on the same URL. The navigate reported success and said
// nothing, so an agent had a page it was not looking at.
func TestNavigateFallbackAnnouncesTheNewTabWithoutASession(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")

	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","status":"ok"}`)
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", staleTabCmd(t, "STALE123"))
		})
	})

	notice := fallbackNoticeLine(t, stderr)
	for _, want := range []string{"STALE123", "NEW123", "new tab"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice = %q, want it to contain %q", notice, want)
		}
	}
	// The retry still has to have worked — a notice about a navigation that
	// failed would be worse than silence.
	if len(m.requests) != 2 || m.requests[1].Path != "/navigate" {
		t.Fatalf("requests = %+v, want the tab-scoped attempt then the unscoped retry", m.requests)
	}
}

// The session path is the one that already works: the unscoped retry reuses the
// session's current tab, so nothing was created and there is nothing to
// announce. Saying "opened a new tab" here would be false.
func TestNavigateFallbackStaysSilentForAnIdentifiedCaller(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "ses_something")

	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"SESSION_CURRENT","status":"ok"}`)
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", staleTabCmd(t, "STALE123"))
		})
	})

	if strings.Contains(stderr, "new tab") {
		t.Fatalf("stderr = %q, want no new-tab notice for a caller whose retry adopts the session's current tab", stderr)
	}
	// The retry itself is unchanged: still one unscoped POST, still no tab id in
	// the body, so the server keeps owning the tab-selection policy.
	if len(m.requests) != 2 || m.requests[1].Path != "/navigate" {
		t.Fatalf("requests = %+v, want the fallback to still fire", m.requests)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.requests[1].Body), &body)
	if _, ok := body["tabId"]; ok {
		t.Errorf("retry body = %v; the CLI must not start naming a tab, that is the server's policy", body)
	}
}

// The agent id reaches the server from EITHER the --agent-id flag or the environment, so a
// caller identified by the flag alone is identified to the server and its retry adopts that
// scope's current tab. Announcing a new tab there names an EXISTING tab as new — the inverse
// of the defect the notice exists to report, and just as misleading. The env is cleared, so
// only the flag can make this caller identified.
func TestNavigateFallbackStaysSilentForACallerIdentifiedByTheFlagAlone(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")

	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"ADOPTED9","status":"ok"}`)
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", agentIDFlagCmd(t, "STALE123", "bosch"))
		})
	})

	if strings.Contains(stderr, "new tab") {
		t.Fatalf("stderr = %q, want no new-tab notice: the retry carries X-Agent-Id, so the server adopted that scope's current tab rather than creating one", stderr)
	}
	// The no-session hint reads the same predicate, so it moves with it: a caller already
	// scoped by agent id is not unscoped, whichever provenance supplied the id.
	if strings.Contains(stderr, cli.SessionCreateCommand) {
		t.Errorf("stderr = %q, want no session advice for a caller the server already scopes by agent id", stderr)
	}
	if len(m.requests) != 2 || m.requests[1].Path != "/navigate" {
		t.Fatalf("requests = %+v, want the fallback to still fire", m.requests)
	}
}

// Absence assertion: an ordinary navigate that never hits the fallback gains no
// extra output. Without this the notice could fire on every call and the tests
// above would still pass.
func TestNavigateWithoutFallbackAnnouncesNothingExtra(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")

	m := newMockServer()
	m.response = `{"tabId":"ABC123","status":"ok"}`
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", newNavigateCmd())
		})
	})

	if strings.Contains(stderr, "no longer exists") {
		t.Fatalf("stderr = %q, want no fallback notice when no fallback fired", stderr)
	}
}

// The notice must not repeat the session advice cli.NoSessionHint already
// carries: an anonymous fallback prints both lines, and the same remedy twice on
// one command is the shape that trains readers to skip hints.
func TestFallbackNoticeDoesNotRepeatTheSessionRemedy(t *testing.T) {
	anonymousCaller(t)

	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","status":"ok"}`)
	defer m.close()

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", staleTabCmd(t, "STALE123"))
		})
	})

	if strings.Count(stderr, cli.SessionCreateCommand) != 1 {
		t.Errorf("stderr carries the session command %d times, want exactly once (from cli.NoSessionHint):\n%s",
			strings.Count(stderr, cli.SessionCreateCommand), stderr)
	}
	if !strings.Contains(fallbackNoticeLine(t, stderr), "new tab") {
		t.Errorf("the fallback notice itself is missing from:\n%s", stderr)
	}
}

// stdout stays tab-id-only through a fallback: `TAB=$(pinchtab nav URL)` would
// otherwise capture the notice.
func TestFallbackNoticeStaysOffStdout(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	stdoutTerminal(t, false)

	m := newMockServer()
	m.setResponse(http.MethodPost, "/tabs/STALE123/navigate", http.StatusNotFound, `{"error":"tab not found"}`)
	m.setResponse(http.MethodPost, "/navigate", http.StatusOK, `{"tabId":"NEW123","url":"https://pinchtab.com","status":"ok"}`)
	defer m.close()

	var out string
	_ = captureStderr(t, func() {
		out = captureStdout(t, func() {
			Navigate(m.server.Client(), m.base(), "", "https://pinchtab.com", staleTabCmd(t, "STALE123"))
		})
	})

	if strings.TrimSpace(out) != "NEW123" {
		t.Fatalf("stdout = %q, want only the tab id", out)
	}
}

// staleTabCmd builds the nav command shape the fallback needs: a tab id that came
// from saved state rather than an explicit --tab, which is the only case the
// retry is allowed to fire for.
func staleTabCmd(t *testing.T, tabID string) *cobra.Command {
	t.Helper()

	cmd := newNavigateCmd()
	cmd.Flags().Lookup("tab").DefValue = tabID
	if err := cmd.Flags().Set("tab", tabID); err != nil {
		t.Fatal(err)
	}
	cmd.Flags().Lookup("tab").Changed = false
	return cmd
}

// agentIDFlagCmd attaches the nav command to a root carrying --agent-id as a persistent
// flag, which is how the real tree registers it. The attachment is the point: the flag is
// never local to nav, so a standalone command cannot exercise the lookup at all.
func agentIDFlagCmd(t *testing.T, tabID, agentID string) *cobra.Command {
	t.Helper()

	cmd := staleTabCmd(t, tabID)
	root := &cobra.Command{Use: "pinchtab"}
	root.PersistentFlags().String("agent-id", "", "")
	root.AddCommand(cmd)
	if err := root.PersistentFlags().Set("agent-id", agentID); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// fallbackNoticeLine returns the single HINT line about the vanished tab, failing
// when there is none — so a test asserting on its content cannot pass against an
// empty string.
func fallbackNoticeLine(t *testing.T, stderr string) string {
	t.Helper()

	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "no longer exists") {
			return line
		}
	}
	t.Fatalf("no fallback notice in stderr:\n%s", stderr)
	return ""
}
