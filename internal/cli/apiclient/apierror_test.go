package apiclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestIsRouteNotFound(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"mux plain-text 404", 404, "404 page not found\n", true},
		{"application JSON 404", 404, `{"error":"tab not found"}`, false},
		{"JSON body without error field", 404, `{"status":"missing"}`, false},
		{"plain-text 500", 500, "boom", false},
		{"ok", 200, "404 page not found", false},
	}
	for _, tc := range cases {
		if got := isRouteNotFound(tc.status, []byte(tc.body)); got != tc.want {
			t.Errorf("%s: isRouteNotFound = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An older instance without the /audit route answers with the mux's
// plain-text 404; the rendered error must blame the instance, not the
// audited target site.
func TestRenderAPIErrorForFaked404Instance(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	r := request{method: "POST", url: srv.URL + "/audit", body: map[string]any{"urls": []string{"https://example.com"}}}
	resp, err := http.Post(r.url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	msg := renderAPIError(r, resp.StatusCode, body)
	host := strings.TrimPrefix(srv.URL, "http://")
	for _, want := range []string{host, "POST /audit", "older version", "Restart it with the current binary"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "example.com") {
		t.Errorf("message must not implicate the target site:\n%s", msg)
	}
	if strings.Contains(msg, "404 page not found") {
		t.Errorf("message should replace the bare mux text:\n%s", msg)
	}
}

func TestRenderAPIErrorKeepsApplicationErrors(t *testing.T) {
	r := request{method: "GET", url: "http://127.0.0.1:9867/console"}
	msg := renderAPIError(r, 404, []byte(`{"error":"tab not found: nonexistent_xyz"}`))
	if !strings.Contains(msg, "Error 404: tab not found: nonexistent_xyz") {
		t.Errorf("application 404 rendering changed:\n%s", msg)
	}
	if strings.Contains(msg, "older version") {
		t.Errorf("application 404 must not be treated as a missing route:\n%s", msg)
	}

	msg = renderAPIError(r, 403, []byte(`{"error":"blocked","details":{"hint":"allow the domain"}}`))
	if !strings.Contains(msg, "Error 403: blocked") || !strings.Contains(msg, "allow the domain") {
		t.Errorf("hint rendering changed:\n%s", msg)
	}
}

// A refusal whose details carry the way forward; the renderer is what puts it in
// front of the user. The sample used to be navigation_changed, which the server no
// longer emits — a click that navigates succeeds — so it is an allowlist refusal,
// which is a code the server does still produce with the same details shape.
func TestRenderAPIErrorBodyShowsHintAndRemedy(t *testing.T) {
	body := `{"code":"idpi_domain_blocked",` +
		`"error":"navigation blocked by IDPI: domain \"evil.test\" is not in the allowed list",` +
		`"details":{"hint":"the requested URL is outside security.allowedDomains, so the request was refused.",` +
		`"remedy":"pinchtab config set security.allowedDomains \"...\" && pinchtab server restart",` +
		`"url":"https://evil.test/"}}`

	out := renderAPIErrorBody(403, []byte(body))

	for _, want := range []string{
		"Error 403: navigation blocked by IDPI",
		"💡 the requested URL is outside security.allowedDomains",
		"Remedy: pinchtab config set security.allowedDomains",
		"server restart",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered error missing %q:\n%s", want, out)
		}
	}
}

// A tab_paused_handoff refusal must name the CLI resume command, not the HTTP route the
// hint prose carries: the CLI user runs `pinchtab resume`, not a POST. The server ships
// the command as details.remedy; renderGuidance prints it in the Remedy slot.
func TestHandoffRefusalRendersTheCLIResumeCommand(t *testing.T) {
	body := `{"code":"tab_paused_handoff",` +
		`"error":"tab tab1 is paused for human handoff (manual_handoff)",` +
		`"details":{"hint":"...then call POST /tabs/{id}/resume to continue","remedy":"pinchtab resume tab1"}}`

	out := renderAPIErrorBody(http.StatusConflict, []byte(body))

	if !strings.Contains(out, "Remedy: pinchtab resume tab1") {
		t.Errorf("rendered refusal does not name the CLI resume command:\n%s", out)
	}
}

// cachedTabFile installs a cached-tab context pointing at a real file, so a render
// that still performed the old os.Remove would be visible as a missing file rather
// than as an assertion about a string.
func cachedTabFile(t *testing.T, tabID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "current-tab-127.0.0.1-9867")
	if err := os.WriteFile(path, []byte(tabID+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := cachedTab
	UseCachedTab(CachedTab{TabID: tabID, Base: "http://127.0.0.1:9867", StateFile: path})
	t.Cleanup(func() { UseCachedTab(previous) })
	return path
}

// Formatting an error must not mutate anything. The remedy used to arrive as an
// installed callback that deleted the state file and read the config, so every
// render — including the ones a poll loop throws away — cleared persistent state.
func TestRenderingAStaleTab404TwiceLeavesTheCacheIntact(t *testing.T) {
	path := cachedTabFile(t, "CACHED-TAB")
	body := []byte(`{"code":"error","error":"tab CACHED-TAB not found"}`)

	first := renderAPIErrorBody(http.StatusNotFound, body)
	second := renderAPIErrorBody(http.StatusNotFound, body)

	if first != second {
		t.Errorf("rendering the same error twice differs, so something is carrying state:\n%s\n---\n%s", first, second)
	}
	for _, want := range []string{"cached current tab", "127.0.0.1:9867", "retry the command", "pinchtab nav"} {
		if !strings.Contains(first, want) {
			t.Errorf("remedy missing %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "now cleared") {
		t.Errorf("a render claims a completed clearing it did not perform:\n%s", first)
	}
	if data, err := os.ReadFile(path); err != nil || strings.TrimSpace(string(data)) != "CACHED-TAB" {
		t.Fatalf("rendering an error mutated the cached tab file (%v, %q)", err, string(data))
	}
}

// A 404 about a tab this CLI never cached is not this cache's business.
func TestUnrelatedTabNotFoundGetsNoRemedy(t *testing.T) {
	path := cachedTabFile(t, "CACHED-TAB")
	out := renderAPIErrorBody(http.StatusNotFound, []byte(`{"error":"tab SOMEONE-ELSE not found"}`))
	if strings.Contains(out, "cached current tab") {
		t.Errorf("remedy offered for a tab the cache never held:\n%s", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("unrelated 404 cleared the cache: %v", err)
	}
}

func wireNotFoundBody(t *testing.T, write func(http.ResponseWriter)) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	write(w)
	if w.Code != http.StatusNotFound {
		t.Fatalf("fixture answered %d, want 404", w.Code)
	}
	return w.Body.Bytes()
}

func TestOnlyAMissingTab404MarksTheCachedTabStale(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(http.ResponseWriter)
		stale bool
	}{
		{"bridge missing tab", func(w http.ResponseWriter) {
			httpx.Error(w, http.StatusNotFound, fmt.Errorf("tab %s not found", "CACHED-TAB"))
		}, true},
		{"orchestrator missing tab", func(w http.ResponseWriter) {
			httpx.Error(w, http.StatusNotFound, fmt.Errorf("tab %q not found", "CACHED-TAB"))
		}, true},
		{"tab lost to a browser crash", func(w http.ResponseWriter) {
			httpx.ErrorCode(w, http.StatusNotFound, "browser_crashed", "tab CACHED-TAB not found: the browser crashed", false, nil)
		}, true},
		{"ref miss naming the live tab", func(w http.ResponseWriter) {
			httpx.ErrorCode(w, http.StatusNotFound, "ref_not_found",
				"ref e99 not found - tab CACHED-TAB has no snapshot - take a /snapshot first", false,
				map[string]any{"tabId": "CACHED-TAB"})
		}, false},
		{"submit target miss naming the live tab", func(w http.ResponseWriter) {
			httpx.ErrorCode(w, http.StatusNotFound, "submit_target_not_found", "ref e99 not found in tab CACHED-TAB", false, nil)
		}, false},
		{"code-less 404 naming the tab", func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"tab CACHED-TAB not found"}`))
		}, false},
		{"uncoded 404 naming the tab only in details", func(w http.ResponseWriter) {
			httpx.ErrorCode(w, http.StatusNotFound, "error", "no such download", false, map[string]any{"tabId": "CACHED-TAB"})
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := cachedTabFile(t, "CACHED-TAB")
			body := wireNotFoundBody(t, tc.write)

			rendered := renderAPIErrorBody(http.StatusNotFound, body)
			cleared := clearCachedTabOnFailure(http.StatusNotFound, body)
			_, statErr := os.Stat(path)

			if got := strings.Contains(rendered, "cached current tab"); got != tc.stale {
				t.Errorf("stale-tab hint printed = %v, want %v:\n%s", got, tc.stale, rendered)
			}
			if got := strings.Contains(rendered, "pinchtab nav"); got != tc.stale {
				t.Errorf("nav remedy printed = %v, want %v:\n%s", got, tc.stale, rendered)
			}
			if got := cleared != ""; got != tc.stale {
				t.Errorf("clearing reported = %v, want %v: %q", got, tc.stale, cleared)
			}
			if got := os.IsNotExist(statErr); got != tc.stale {
				t.Errorf("cache file removed = %v, want %v (%v)", got, tc.stale, statErr)
			}
		})
	}
}

// Conn.Raw returns instead of terminating — one caller polls it
// ten times a second — so rendering its error must leave the cache alone.
func TestReturningRequestPathsLeaveTheCacheIntact(t *testing.T) {
	path := cachedTabFile(t, "CACHED-TAB")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"error","error":"tab CACHED-TAB not found"}`))
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		call func() ([]byte, error)
	}{
		{"GET", func() ([]byte, error) {
			return (Conn{Client: srv.Client(), Base: srv.URL}).Raw(http.MethodGet, "/tabs/CACHED-TAB/text")
		}},
		{"POST", func() ([]byte, error) {
			return (Conn{Client: srv.Client(), Base: srv.URL}).Raw(http.MethodPost, "/action", WithBody(map[string]any{"kind": "click"}))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.call()
			if err == nil {
				t.Fatal("expected the 404 to be returned as an error")
			}
			if !strings.Contains(err.Error(), "cached current tab") {
				t.Errorf("returned error lost the remedy:\n%v", err)
			}
			if strings.Contains(err.Error(), "now cleared") {
				t.Errorf("a returning path claims the cache was cleared:\n%v", err)
			}
			if _, statErr := os.Stat(path); statErr != nil {
				t.Fatalf("%s cleared the cached tab: %v", tc.name, statErr)
			}
		})
	}
}

// The remedy used to be an installed func the command layer assigned at init(), so
// this package's behaviour depended on which binary linked it and its own tests
// were side-effect free only because the variable happened to be nil here. Data
// cannot carry behaviour; a func variable can, so none may come back.
func TestNoMutablePackageLevelFunctionVariables(t *testing.T) {
	fileSet, files := srccensus.Load(t, ".", 5).Syntax()
	for path, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if declaresFunc(value.Type) || (i < len(value.Values) && declaresFunc(value.Values[i])) {
						t.Errorf("%s:%d declares the package-level function variable %s; the command layer must hand this package DATA, not behaviour it installs",
							path, fileSet.Position(name.Pos()).Line, name.Name)
					}
				}
			}
		}
	}
}

func declaresFunc(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.FuncType, *ast.FuncLit:
		return true
	}
	return false
}

const terminalChildEnv = "PINCHTAB_TEST_TERMINAL_API_ERROR"

// The clearing moved to the terminal paths, and those end in os.Exit, so the only
// honest way to test them is to be a different process. Criterion two of the card
// is kept verbatim: the purity cleanup must not quietly drop the feature — the
// remedy tells the user to retry, and the retry works because the dead id is gone.
func TestTerminalPathsClearTheCachedTabAndSayTheyDid(t *testing.T) {
	if mode := os.Getenv(terminalChildEnv); mode != "" {
		runTerminalChild(mode)
		return
	}

	for _, mode := range []string{"ExitWithAPIError", "GetRaw"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "current-tab-127.0.0.1-9867")
			if err := os.WriteFile(path, []byte("CACHED-TAB\n"), 0600); err != nil {
				t.Fatal(err)
			}

			child := exec.Command(os.Args[0], "-test.run=TestTerminalPathsClearTheCachedTabAndSayTheyDid", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), terminalChildEnv+"="+mode, "PINCHTAB_TEST_TAB_STATE_FILE="+path)
			raw, err := child.CombinedOutput()
			out := string(raw)

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("child exited with %v, want the terminal path's os.Exit(1); output:\n%s", err, out)
			}
			if code := exitErr.ExitCode(); code != 1 {
				t.Fatalf("child exit code = %d, want 1; output:\n%s", code, out)
			}
			if !strings.Contains(out, "cached current tab") || !strings.Contains(out, "retry the command") {
				t.Errorf("the terminal path lost the remedy:\n%s", out)
			}
			if !strings.Contains(out, "now cleared") {
				t.Errorf("the terminal path cleared the cache without saying so:\n%s", out)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("the cached tab survived a command that failed on it (%v), so the remedy's retry advice is false", statErr)
			}
		})
	}
}

// runTerminalChild drives a real terminal path with a real cached-tab context, and
// exits through it.
func runTerminalChild(mode string) {
	body := []byte(`{"code":"error","error":"tab CACHED-TAB not found"}`)
	UseCachedTab(CachedTab{
		TabID:     "CACHED-TAB",
		Base:      "http://127.0.0.1:9867",
		StateFile: os.Getenv("PINCHTAB_TEST_TAB_STATE_FILE"),
	})

	switch mode {
	case "ExitWithAPIError":
		Conn{}.ExitWithAPIError(http.StatusNotFound, body)
	case "GetRaw":
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(body)
		}))
		defer srv.Close()
		(Conn{Client: srv.Client(), Base: srv.URL}).GetRaw("/tabs/CACHED-TAB/text", nil)
	}
	fmt.Fprintf(os.Stderr, "child mode %q returned instead of exiting\n", mode)
	os.Exit(9)
}

func TestConnExitWithAPIErrorNamesTheRejectedTokenPairing(t *testing.T) {
	if os.Getenv(terminalChildEnv) == "RejectedToken" {
		w := httptest.NewRecorder()
		httpx.Unauthorized(w, httpx.CodeBadToken, "deadbeef")
		Conn{Base: "http://203.0.113.9:9867", TokenSource: "the PINCHTAB_TOKEN environment variable"}.ExitWithAPIError(http.StatusUnauthorized, w.Body.Bytes())
		os.Exit(9)
	}

	child := exec.Command(os.Args[0], "-test.run=TestConnExitWithAPIErrorNamesTheRejectedTokenPairing", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
	child.Env = append(os.Environ(), terminalChildEnv+"=RejectedToken")
	raw, err := child.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("child exited with %v, want the terminal path's os.Exit(1); output:\n%s", err, raw)
	}
	want := "came from the PINCHTAB_TOKEN environment variable and was sent to http://203.0.113.9:9867"
	if !strings.Contains(string(raw), want) {
		t.Errorf("terminal bad-token output lost the Conn's pairing, want %q in:\n%s", want, raw)
	}
}

// The past-tense sentence is a claim about a completed deletion, so it may only be
// emitted when the file is actually gone. A removal that fails must stay silent
// rather than tell the user to retry a command that will fail the same way.
func TestAFailedClearingSaysNothing(t *testing.T) {
	dir := t.TempDir()
	unremovable := filepath.Join(dir, "state-dir")
	if err := os.MkdirAll(filepath.Join(unremovable, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := cachedTab
	UseCachedTab(CachedTab{TabID: "CACHED-TAB", Base: "http://127.0.0.1:9867", StateFile: unremovable})
	t.Cleanup(func() { UseCachedTab(previous) })

	got := clearCachedTabOnFailure(http.StatusNotFound, []byte(`{"code":"error","error":"tab CACHED-TAB not found"}`))
	if got != "" {
		t.Errorf("clearCachedTabOnFailure reported %q although the removal failed", got)
	}
	if _, err := os.Stat(unremovable); err != nil {
		t.Fatalf("precondition: the path was removable after all, so this test proves nothing: %v", err)
	}
}

func wireUnauthorizedBody(t *testing.T, code, presented string) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	httpx.Unauthorized(w, code, presented)
	return w.Body.Bytes()
}

func TestThe401CasesRenderDistinctMessagesWithTheirCodes(t *testing.T) {
	missing := renderAPIErrorBody(401, wireUnauthorizedBody(t, httpx.CodeMissingToken, ""))
	bad := renderAPIErrorBody(401, wireUnauthorizedBody(t, httpx.CodeBadToken, "deadbeef"))

	if !strings.Contains(missing, "(missing_token)") || !strings.Contains(missing, "PINCHTAB_TOKEN") {
		t.Errorf("missing-token render = %q, want the code surfaced and PINCHTAB_TOKEN named", missing)
	}
	if !strings.Contains(bad, "(bad_token)") || !strings.Contains(bad, "--server") {
		t.Errorf("bad-token render = %q, want the code surfaced and the remote cue", bad)
	}
	if missing == bad {
		t.Errorf("both 401 cases render identically: %q — the collapse this card removes", missing)
	}
}

func TestACodedErrorWithNoDetailsSurfacesItsCode(t *testing.T) {
	got := renderAPIErrorBody(400, []byte(`{"error":"invalid button \"rihgt\"","code":"invalid_mouse_button"}`))
	if !strings.Contains(got, "(invalid_mouse_button)") {
		t.Errorf("render = %q; a coded error with no details drops the one machine-readable field the server sent", got)
	}
}

func TestARejectedTokenNamesItsSourceAndTheBaseThatRejectedIt(t *testing.T) {
	rejecting := func(code, presented string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			httpx.Unauthorized(w, code, presented)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	local := rejecting(httpx.CodeBadToken, "deadbeef")
	remote := rejecting(httpx.CodeBadToken, "deadbeef")
	localConn := Conn{Client: local.Client(), Base: local.URL, Token: "deadbeef", TokenSource: "server.token in the config file"}
	remoteConn := Conn{Client: remote.Client(), Base: remote.URL, Token: "deadbeef", TokenSource: "the PINCHTAB_TOKEN environment variable"}

	_, err := remoteConn.Raw(http.MethodGet, "/health")
	want := "the rejected token came from the PINCHTAB_TOKEN environment variable and was sent to " + remote.URL
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("remote bad-token error = %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), local.URL) || strings.Contains(err.Error(), localConn.TokenSource) {
		t.Errorf("remote bad-token error = %q names the other connection's pairing", err)
	}

	_, err = localConn.Raw(http.MethodGet, "/health")
	want = "the rejected token came from server.token in the config file and was sent to " + local.URL
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("local bad-token error = %v, want %q", err, want)
	}

	missing := rejecting(httpx.CodeMissingToken, "")
	_, err = Conn{Client: missing.Client(), Base: missing.URL, TokenSource: "the PINCHTAB_TOKEN environment variable"}.Raw(http.MethodGet, "/health")
	if err == nil || strings.Contains(err.Error(), "came from") {
		t.Errorf("missing-token error = %v names a pairing for a credential that was never sent", err)
	}
}

// The shape 424 call sites produce, and the one no fixture drove: httpx.Error stamps the
// generic sentinel on every uncoded refusal, so a code suffix here would append a bare
// "(error)" to the most common error in the product — noise on the largest error family,
// added by a change written for the coded one. This fixture is also what stops the next
// widening of the condition from re-introducing it.
func TestTheGenericSentinelCodeRendersExactlyAsAnUncodedError(t *testing.T) {
	const message = `element text extract: no element matches selector "#nope": selector matched no element`

	withSentinel := renderAPIErrorBody(404, []byte(`{"code":"error","error":`+quoteJSON(message)+`}`))
	withoutCode := renderAPIErrorBody(404, []byte(`{"error":`+quoteJSON(message)+`}`))

	if withSentinel != withoutCode {
		t.Errorf("a generic-sentinel error renders differently from an uncoded one:\n with %q\n without %q", withSentinel, withoutCode)
	}
	if strings.Contains(withSentinel, "(error)") {
		t.Errorf("the render appends the sentinel, which adds nothing beyond the 404 already printed: %q", withSentinel)
	}
	if !strings.Contains(withSentinel, message) {
		t.Errorf("suppressing the sentinel lost the message: %q", withSentinel)
	}
}

// The sentinel is spelled here rather than imported, so the agreement is pinned against the
// BYTES the server actually sends rather than against a shared symbol. Drives the real
// httpx.Error, so a rename or a change of the stamped code reds here instead of quietly
// putting "(some_new_sentinel)" back on 424 responses.
func TestTheGenericSentinelStillMatchesWhatTheServerSends(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.Error(w, 404, errors.New("no element matches selector"))

	var wire struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wire); err != nil {
		t.Fatalf("httpx.Error did not produce the error envelope: %v (%s)", err, w.Body.String())
	}
	if wire.Code == "" {
		t.Fatal("httpx.Error stamped no code, so this parity check would compare against nothing")
	}
	if wire.Code != genericErrorCode {
		t.Errorf("httpx.Error now stamps %q but the renderer suppresses %q, so every uncoded refusal would render with a meaningless suffix again — update genericErrorCode", wire.Code, genericErrorCode)
	}
	// The other half: a code that is NOT the sentinel must still reach the reader.
	if !codeAddsInformation("tab_not_found", "unauthorized") {
		t.Error("a real code was classified as adding nothing; the suppression is too wide")
	}
}

func quoteJSON(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
