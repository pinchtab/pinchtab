package mcp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// looksLikeStructuredSelector decides whether an MCP client's free-form `query`
// is passed through as a selector or wrapped as semantic `find:` text. Anything
// passed through is parsed as CSS by default, so a false positive turns natural
// language into an unmatchable selector. These cases pin the current boundary.
func TestLooksLikeStructuredSelector(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"plain label", "Sign In", false},
		{"plain sentence", "accept all cookies", false},
		{"decimal number", "50.50", false},
		{"id selector", "#submit", true},
		{"class selector", ".btn-primary", true},
		{"attribute selector", "[data-test=submit]", true},
		{"xpath", "//button[@id='ok']", true},
		{"parenthesised xpath", "(//a)[1]", true},
		{"descendant combinator", "div > p", true},
		{"tag dot class", "button.primary", true},
		{"pseudo class colon has no surrounding space", "a:hover", true},
		{"attribute equals has no surrounding space", "input[type=text]", true},
		{"attribute selector with sibling combinator", "input[name=q] + label", true},

		{"prose colon is space separated", "Sign up: it's free", false},
		{"prose equals is space separated", "name = value", false},
		{"colon with a space only before it", "ready :go", false},
		{"equals with a space only after it", "name= value", false},
		{"prose plus stays CSS because combinators are space separated in CSS too", "Add + remove", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeStructuredSelector(tt.in); got != tt.want {
				t.Errorf("looksLikeStructuredSelector(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestActionSelectorArgUsesTheGrammarsPrefixVocabulary(t *testing.T) {
	// "find: login button" and "Role: Save" are the discriminating cases: the
	// space after the colon keeps looksLikeStructuredSelector from claiming
	// them, so only the grammar's prefix vocabulary passes them through.
	for _, in := range []string{"css:#a", "CSS:#a", "Find:submit", "NTH:2:div", "  text:hello", "find: login button", "Role: Save"} {
		want := strings.TrimSpace(in)
		if got, _, _ := actionSelectorArg(queryRequest(in)); got != want {
			t.Errorf("actionSelectorArg(%q) = %q, want %q passed through as a selector", in, got, want)
		}
	}
	for _, in := range []string{"submit", "unknownprefix value"} {
		if got, _, _ := actionSelectorArg(queryRequest(in)); got != "find:"+in {
			t.Errorf("actionSelectorArg(%q) = %q, want it wrapped as find:", in, got)
		}
	}
}

func queryRequest(query string) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": query}
	return req
}

func TestActionSelectorArgRoutesSpaceSeparatedProseToFind(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"prose colon is space separated", "Sign up: it's free", "find:Sign up: it's free"},
		{"prose equals is space separated", "name = value", "find:name = value"},
		{"pseudo class colon has no surrounding space", "a:hover", "a:hover"},
		{"attribute equals has no surrounding space", "input[type=text]", "input[type=text]"},
		{"attribute selector with sibling combinator", "input[name=q] + label", "input[name=q] + label"},
		{"descendant combinator", "div > p", "div > p"},
		{"prose plus stays CSS because combinators are space separated in CSS too", "Add + remove", "Add + remove"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"query": tt.query}
			if got, _, _ := actionSelectorArg(req); got != tt.want {
				t.Errorf("actionSelectorArg(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

// The funnel's whole rule used to be the status code, so any endpoint reporting failure
// inside a 200 reached the agent as success — the cookie-set path shipped exactly that.
// These rows drive the counting-shape rule through resultFromBytes itself, so a NEW
// endpoint emitting the shape is covered without anyone enumerating it.
func TestA200ReportingNoSuccessesIsAnErrorResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantError bool
		wantSays  string
	}{
		{name: "zero successes with the body's own reason", body: `{"set":0,"failed":1,"total":1,"failures":[{"name":"sid","error":"invalid domain"}]}`, wantError: true, wantSays: "invalid domain"},
		{name: "zero successes on the batch spelling", body: `{"successful":0,"failed":2,"total":2,"results":[]}`, wantError: true, wantSays: "no successes"},
		{name: "failed count with no success count fails closed", body: `{"failed":2,"total":2}`, wantError: true, wantSays: "no success count"},

		// Partial success is a SUCCESS carrying detail: the landed items' effects already
		// happened, and the body's counts and failures are what the agent needs to retry
		// only what missed. An error frame would report work that succeeded as lost.
		{name: "partial success stays a success carrying detail", body: `{"set":1,"failed":1,"total":2,"failures":[{"error":"x"}]}`, wantError: false},
		{name: "full success with a zero failed count", body: `{"set":2,"failed":0,"total":2}`, wantError: false},

		// The exclusions are structural, not a key-name list — each of these reports
		// failures AS its payload and must stay a successful call.
		{name: "observability snapshot's failures payload", body: `{"layer":"instance","failures":{"recent":[{"reason":"crash"}]}}`, wantError: false},
		{name: "health tabs failures payload", body: `{"status":"degraded","failures":{"recent":[]}}`, wantError: false},
		{name: "console errors payload", body: `{"errors":[{"text":"TypeError"}],"logs":[]}`, wantError: false},
		{name: "network response with a per-request failed bool", body: `{"requests":[{"url":"https://a/x.js","failed":true,"status":0}]}`, wantError: false},

		// Guards on the shape itself.
		{name: "a top-level boolean failed is not a count", body: `{"failed":true}`, wantError: false},
		{name: "a nested failed count is not this call's count", body: `{"detail":{"failed":3}}`, wantError: false},
		{name: "plain text keeps the status rule", body: `all good`, wantError: false},
		{name: "a JSON array keeps the status rule", body: `[1,2,3]`, wantError: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := resultFromBytes([]byte(tc.body), 200)
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError != tc.wantError {
				t.Fatalf("IsError = %v, want %v for body %s", res.IsError, tc.wantError, tc.body)
			}
			if tc.wantSays != "" {
				if text := resultText(t, res); !strings.Contains(text, tc.wantSays) {
					t.Errorf("error = %q, want it to carry %q — the body's own reason, not a generic failure", text, tc.wantSays)
				}
			}
		})
	}
}

// The rule holds at the tool surface, not only at the helper: a click whose endpoint
// answers 200 with zero successes must reach the agent as an error.
func TestAToolCallSeesAZeroSuccess200AsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"successful":0,"failed":1,"total":1,"results":[{"kind":"click","error":"node gone"}]}`))
	}))
	defer srv.Close()

	res := callTool(t, "pinchtab_click", map[string]any{"ref": "e5"}, srv)
	if !res.IsError {
		t.Fatalf("a zero-success 200 reached the agent as success: %s", resultText(t, res))
	}
	if text := resultText(t, res); !strings.Contains(text, "node gone") {
		t.Errorf("error = %q, want the body's own reason to ride along", text)
	}
}

// The population the rule serves is DERIVED, not hand-listed: every non-test file in the
// packages the funnel proxies that spells a "failed" JSON key carries a recorded decision.
// A new endpoint counting its work lands in the funnel rule automatically, whatever package
// it lives in; a new FILE spelling this same key lands here and forces a human decision.
//
// What neither catches, stated so it is not mistaken for covered: a genuinely different
// failure vocabulary — "errorCount", "rejected" — is invisible to the funnel (which reads
// the key "failed") and to this census (which greps for it), and a producer outside the two
// prefixes below is outside this census entirely. Measured. Widening either is a decision,
// not an oversight.
func TestEveryFailedKeyProducerHasARecordedFunnelDecision(t *testing.T) {
	decisions := map[string]string{
		"internal/handlers/cookies.go":              "top-level numeric failed beside set/total: the counting shape, converted by the funnel rule",
		"internal/handlers/actions.go":              "batch/macro counting shape (successful/failed/total); not reachable from MCP today, covered by the rule the day a batch tool lands",
		"internal/bridge/observe/network_buffer.go": "per-request failed is a nested BOOL describing the page's own subresources, excluded by the top-level-numeric requirement — an agent must not see a page's 404 as a failed call",
	}

	countingShape := map[string]bool{
		"internal/handlers/cookies.go": true,
		"internal/handlers/actions.go": true,
	}

	files := srccensus.Tree(t, filepath.Join("..", ".."), 200)
	found := map[string]bool{}
	for _, file := range files {
		if !strings.HasPrefix(file.Name, "internal/handlers/") && !strings.HasPrefix(file.Name, "internal/bridge/observe/") {
			continue
		}
		if !strings.Contains(file.Text, `"failed"`) {
			continue
		}
		found[file.Name] = true
		if countingShape[file.Name] && !spellsAnySuccessKey(file.Text) {
			t.Errorf("%s emits a counting shape but none of the funnel's success keys %v; the funnel would report its total failure as success", file.Name, successCountKeys)
		}
		if decisions[file.Name] == "" {
			t.Errorf("%s spells a \"failed\" JSON key and has no recorded funnel decision; decide whether it is the counting shape (the funnel converts it) or a payload (say why it is excluded) and record it here", file.Name)
		}
	}
	for name, reason := range decisions {
		if reason == "" {
			t.Errorf("%s is recorded with no reason; every entry is a decision", name)
		}
		if !found[name] {
			t.Errorf("%s is recorded but no longer spells a \"failed\" key; drop or re-point its entry", name)
		}
	}
	if len(found) == 0 {
		t.Fatal("found no \"failed\" key producer at all; if the spelling moved, re-point this census rather than deleting it")
	}
}

func spellsAnySuccessKey(text string) bool {
	for _, key := range successCountKeys {
		if strings.Contains(text, `"`+key+`"`) {
			return true
		}
	}
	return false
}
