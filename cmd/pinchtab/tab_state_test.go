package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// tabStateHarness points the tab-state store at a temp state dir and at a stub
// server, resolving the endpoint the way the CLI does rather than through env
// vars — the divergence between those two resolutions is the defect under test.
type tabStateHarness struct {
	base        string
	token       string
	serverToken string
	statusFor   func(tabID string) int
	handle      func(w http.ResponseWriter, r *http.Request) bool
	requests    []*http.Request
}

func newTabStateHarness(t *testing.T) *tabStateHarness {
	t.Helper()
	h := &tabStateHarness{token: "cfg-token", serverToken: "cfg-token", statusFor: func(string) int { return http.StatusOK }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.requests = append(h.requests, r)
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/tabs/"), "/title")
		// The server validates against serverToken, independent of the token the
		// probe reads (h.token). A test forcing a real 401 sets only the probe's
		// token wrong; coupling both to one field would make the probe authenticate
		// against its own mistake and answer 200 instead.
		if got := r.Header.Get("Authorization"); got != "Bearer "+h.serverToken {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		if h.handle != nil && h.handle(w, r) {
			return
		}
		status := h.statusFor(id)
		w.WriteHeader(status)
		if status == http.StatusNotFound {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "tab " + id + " not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"title": "ok"})
	}))
	t.Cleanup(srv.Close)
	h.base = srv.URL

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	t.Setenv("PINCHTAB_SERVER", "")
	t.Setenv("PINCHTAB_TOKEN", "")
	// The cached-tab announcement is process-global data in apiclient, so each
	// harness starts from nothing rather than from whatever a previous test left.
	previousAnnounced := apiclient.AnnouncedCachedTab()
	apiclient.UseCachedTab(apiclient.CachedTab{})
	t.Cleanup(func() { apiclient.UseCachedTab(previousAnnounced) })

	oldAgent := cliAgentID
	cliAgentID = ""
	oldResolve := resolveTabStateEndpoint
	resolveTabStateEndpoint = func() (string, string) { return h.base, h.token }
	t.Cleanup(func() {
		cliAgentID = oldAgent
		resolveTabStateEndpoint = oldResolve
	})
	return h
}

func tabFlagCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "read"}
	cmd.Flags().String("tab", "", "Tab ID")
	return cmd
}

// The card's failing case: nav writes the cache, the server restarts, and the very
// next read must not send the dead id. The write is fresh, which is what the old
// freshness window trusted — so this is the case a TTL-gated probe cannot catch.
func TestCachedTabClearedAfterRestartWithNoInterveningNav(t *testing.T) {
	h := newTabStateHarness(t)
	WriteTabStateFile("TAB-BEFORE-RESTART")

	// The restart: the id the cache holds is unknown to the server now.
	h.statusFor = func(string) int { return http.StatusNotFound }

	cmd := tabFlagCommand()
	defaultTabFlagFromState(cmd)

	if got, _ := cmd.Flags().GetString("tab"); got != "" {
		t.Fatalf("--tab = %q after a restart, want empty so the command targets the server's current tab", got)
	}
	if _, err := os.Stat(tabStateFile()); !os.IsNotExist(err) {
		t.Fatalf("stale state file survived: %v", err)
	}
	if len(h.requests) == 0 {
		t.Fatal("no probe was issued; a freshly written cache must still be checked")
	}
}

// The probe has to reach the same server with the same credential as the command.
// With a config-file port and token and no env vars, the old probe queried 9867
// unauthenticated: nothing listening, "assume valid", stale forever.
func TestProbeUsesTheCommandsBaseURLAndToken(t *testing.T) {
	h := newTabStateHarness(t)
	WriteTabStateFile("LIVE-TAB")
	h.statusFor = func(id string) int {
		if id == "LIVE-TAB" {
			return http.StatusOK
		}
		return http.StatusNotFound
	}

	cmd := tabFlagCommand()
	defaultTabFlagFromState(cmd)

	if got, _ := cmd.Flags().GetString("tab"); got != "LIVE-TAB" {
		t.Fatalf("--tab = %q, want the live cached tab", got)
	}
	if len(h.requests) != 1 {
		t.Fatalf("probe issued %d requests, want 1", len(h.requests))
	}
	req := h.requests[0]
	if req.Host != strings.TrimPrefix(h.base, "http://") {
		t.Errorf("probe went to %q, want the command's server %q", req.Host, h.base)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+h.token {
		t.Errorf("probe Authorization = %q, want the config-file token", got)
	}
	if got := req.URL.Path; got != "/tabs/LIVE-TAB/title" {
		t.Errorf("probe path = %q", got)
	}
}

// Two servers, two caches. A single global file made an ordinary setup (a daemon
// plus a project server) send one server's tab id to the other.
func TestCurrentTabIsScopedPerServer(t *testing.T) {
	h := newTabStateHarness(t)

	WriteTabStateFile("TAB-ON-A")
	pathA := tabStateFile()

	h.base = "http://127.0.0.1:19999"
	WriteTabStateFile("TAB-ON-B")
	pathB := tabStateFile()

	if pathA == pathB {
		t.Fatalf("both servers share one state file: %s", pathA)
	}
	if got := defaultTabState.read(); got != "TAB-ON-B" {
		t.Fatalf("second server reads %q, want its own TAB-ON-B", got)
	}

	// The first server's cached tab must be untouched by the second server's nav.
	data, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("first server's state file: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "TAB-ON-A" {
		t.Fatalf("first server's cached tab = %q, want TAB-ON-A", got)
	}
}

// An inconclusive probe must not buy the stale entry any trust: the next command
// probes again, and when that probe can finally answer, the entry is cleared. The
// old code touched the file on every assume-valid return, so back-to-back commands
// renewed the window forever.
func TestInconclusiveProbeDoesNotProtectAStaleEntry(t *testing.T) {
	h := newTabStateHarness(t)
	WriteTabStateFile("STALE-TAB")

	// Round one: the probe cannot tell — wrong credential, so 401, not 404.
	h.token = "a-different-token"
	first := tabFlagCommand()
	defaultTabFlagFromState(first)
	if got, _ := first.Flags().GetString("tab"); got != "STALE-TAB" {
		t.Fatalf("--tab = %q, want the cached tab used when the probe is inconclusive", got)
	}
	if _, err := os.Stat(tabStateFile()); err != nil {
		t.Fatalf("an inconclusive probe must not delete the cache: %v", err)
	}

	// Round two, immediately after: the credential works and the server answers 404.
	h.token = "cfg-token"
	h.statusFor = func(string) int { return http.StatusNotFound }
	second := tabFlagCommand()
	defaultTabFlagFromState(second)

	if got, _ := second.Flags().GetString("tab"); got != "" {
		t.Fatalf("--tab = %q on the second command; the first probe's inconclusive answer suppressed the second", got)
	}
	if _, err := os.Stat(tabStateFile()); !os.IsNotExist(err) {
		t.Fatal("stale entry survived a definitive 404 on the very next command")
	}
}

// A 404 naming an id the user never typed is unactionable without saying where the
// id came from, and only this layer knows. It hands the renderer that fact as data
// at the moment it substitutes the cached id — so the remedy needs no callback
// installed into apiclient, and no config read or file deletion while an error is
// being formatted.
func TestSubstitutingTheCachedTabAnnouncesItToTheErrorRenderer(t *testing.T) {
	h := newTabStateHarness(t)
	WriteTabStateFile("CACHED-TAB")

	cmd := tabFlagCommand()
	defaultTabFlagFromState(cmd)
	if got, _ := cmd.Flags().GetString("tab"); got != "CACHED-TAB" {
		t.Fatalf("--tab = %q, want the cached tab so the announcement has something to describe", got)
	}

	announced := apiclient.AnnouncedCachedTab()
	if announced.TabID != "CACHED-TAB" {
		t.Errorf("announced tab id = %q, want CACHED-TAB; without it the renderer cannot tell a cached id from one the user typed", announced.TabID)
	}
	if announced.Base != h.base {
		t.Errorf("announced base = %q, want the server this cache belongs to (%q)", announced.Base, h.base)
	}
	if announced.StateFile != tabStateFile() {
		t.Errorf("announced state file = %q, want %q — the terminal path removes this file", announced.StateFile, tabStateFile())
	}
	if _, err := os.Stat(tabStateFile()); err != nil {
		t.Errorf("announcing the cached tab must not touch it: %v", err)
	}
}

// A positional tab argument resolves from the same cache, so it needs the same
// announcement — the commands taking `pinchtab <cmd> [tab]` reach the identical 404.
func TestResolvingAPositionalTabFromTheCacheAnnouncesIt(t *testing.T) {
	newTabStateHarness(t)
	WriteTabStateFile("CACHED-TAB")

	if got := resolveTabArg(nil); got != "CACHED-TAB" {
		t.Fatalf("resolveTabArg = %q, want the cached tab", got)
	}
	if got := apiclient.AnnouncedCachedTab().TabID; got != "CACHED-TAB" {
		t.Errorf("announced tab id = %q after a positional resolve, want CACHED-TAB", got)
	}
}

// An id the user typed is not this cache's business: the remedy says "not something
// you asked for", so announcing an explicit id would make the message a lie.
func TestAnExplicitTabIsNotAnnouncedAsCached(t *testing.T) {
	newTabStateHarness(t)
	WriteTabStateFile("CACHED-TAB")

	if got := resolveTabArg([]string{"TYPED-TAB"}); got != "TYPED-TAB" {
		t.Fatalf("resolveTabArg = %q, want the explicit argument", got)
	}
	if got := apiclient.AnnouncedCachedTab().TabID; got != "" {
		t.Errorf("announced %q for a tab the user typed; the remedy would claim it came from the cache", got)
	}

	cmd := tabFlagCommand()
	if err := cmd.Flags().Set("tab", "TYPED-TAB"); err != nil {
		t.Fatal(err)
	}
	defaultTabFlagFromState(cmd)
	if got := apiclient.AnnouncedCachedTab().TabID; got != "" {
		t.Errorf("announced %q when --tab was set explicitly", got)
	}
}

func TestAnInvalidServerBaseLeavesTheTabStateInconclusiveWithoutExiting(t *testing.T) {
	oldServerURL := serverURL
	serverURL = ""
	t.Cleanup(func() { serverURL = oldServerURL })
	t.Setenv("PINCHTAB_SERVER", "127.0.0.1:9867")
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	oldExit := osExit
	osExit = func(code int) { t.Fatalf("the tab-state helpers exited with %d on an invalid base", code) }
	t.Cleanup(func() { osExit = oldExit })

	if got := defaultTabState.probe("tab1"); got != tabProbeInconclusive {
		t.Fatalf("probe = %v, want inconclusive", got)
	}
	if defaultTabState.useLocal() {
		t.Fatal("an invalid base must mean no cached-tab state")
	}
	if got := defaultTabState.resolveArg(nil); got != "" {
		t.Fatalf("resolveArg = %q, want no cached tab", got)
	}
	defaultTabState.announceCachedTab("tab1")
}

func TestCloseOfCurrentTabHandsTheStateFileToTheSuccessor(t *testing.T) {
	h := newTabStateHarness(t)
	h.handle = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/close":
			_, _ = w.Write([]byte(`{"closed":true,"tabId":"TAB-C"}`))
		case "/tabs":
			_, _ = w.Write([]byte(`{"tabs":[{"id":"TAB-B"},{"id":"TAB-A"}]}`))
		default:
			return false
		}
		return true
	}
	conn := apiclient.Conn{Client: http.DefaultClient, Base: h.base, Token: h.token}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	WriteTabStateFile("TAB-C")
	closeTabAndAdvance(conn, "TAB-A", cmd)
	if got := defaultTabState.read(); got != "TAB-C" {
		t.Fatalf("closing another tab moved the current tab to %q", got)
	}

	closeTabAndAdvance(conn, "TAB-C", cmd)
	if got := defaultTabState.read(); got != "TAB-B" {
		t.Fatalf("after closing the current tab the state file holds %q, want the successor TAB-B", got)
	}
}
