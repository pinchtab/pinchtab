package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// vocabMockServer returns a server that stamps a vocabulary token on every
// /snapshot response and echoes each /action body, so a test can read back what
// the interaction sent.
func vocabMockServer(token string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/snapshot" {
			w.Header().Set(vocabHeader, token)
		}
		resp := map[string]any{"path": r.URL.Path}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			if json.Unmarshal(body, &parsed) == nil {
				resp["body"] = parsed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func callSharedTool(t *testing.T, c *Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	h, ok := handlerMap(c)[name]
	if !ok {
		t.Fatalf("no handler for %q", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler %q error: %v", name, err)
	}
	return result
}

func actionVocabSent(t *testing.T, r *mcp.CallToolResult) (string, bool) {
	t.Helper()
	body, _ := resultJSON(t, r)["body"].(map[string]any)
	v, ok := body["vocab"]
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

// The point of the whole card at the MCP surface: a snapshot's vocabulary token is
// remembered and echoed on the next interaction, so the server can refuse a ref that
// a later snapshot has renumbered instead of clicking the wrong node. One Client is
// reused across both calls because the stash lives on it, exactly as the long-lived
// MCP server holds it across tool calls.
func TestSnapshotVocabularyTokenIsEchoedOnTheNextAction(t *testing.T) {
	srv := vocabMockServer("vocab-abc")
	defer srv.Close()
	c := NewClient(srv.URL, "")

	callSharedTool(t, c, "pinchtab_snapshot", map[string]any{"tabId": "t1"})
	clickResult := callSharedTool(t, c, "pinchtab_click", map[string]any{"ref": "e5", "tabId": "t1"})

	got, ok := actionVocabSent(t, clickResult)
	if !ok {
		t.Fatal("the click carried no vocab token, so a superseded ref would still be resolved positionally")
	}
	if got != "vocab-abc" {
		t.Errorf("click echoed vocab %q, want the token the snapshot returned", got)
	}
}

// The absence half: without a prior snapshot there is nothing to echo, so the token
// stays optional and the action is unchanged — a guard that always attached something
// would break the permissive contract raw callers rely on.
func TestAnActionWithoutAPriorSnapshotSendsNoToken(t *testing.T) {
	srv := vocabMockServer("vocab-abc")
	defer srv.Close()
	c := NewClient(srv.URL, "")

	clickResult := callSharedTool(t, c, "pinchtab_click", map[string]any{"ref": "e5", "tabId": "t1"})
	if got, ok := actionVocabSent(t, clickResult); ok {
		t.Errorf("an action with no prior snapshot echoed a token %q", got)
	}
}

// The token is remembered per tab, so a snapshot of one tab does not attach its
// vocabulary to an action on another — that would refuse the second tab's valid refs.
func TestTheEchoedTokenIsScopedToItsTab(t *testing.T) {
	srv := vocabMockServer("vocab-abc")
	defer srv.Close()
	c := NewClient(srv.URL, "")

	callSharedTool(t, c, "pinchtab_snapshot", map[string]any{"tabId": "t1"})
	clickResult := callSharedTool(t, c, "pinchtab_click", map[string]any{"ref": "e5", "tabId": "t2"})
	if got, ok := actionVocabSent(t, clickResult); ok {
		t.Errorf("an action on t2 echoed t1's vocabulary token %q", got)
	}
}

// rotatingVocabServer hands out a fresh token on each /snapshot and records the
// query every snapshot request carried, so a test can see both which token the
// client kept and which instance it asked.
func rotatingVocabServer(tokens ...string) (*httptest.Server, *[]url.Values) {
	var seen []url.Values
	var mu sync.Mutex
	next := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/snapshot" {
			mu.Lock()
			seen = append(seen, r.URL.Query())
			if next < len(tokens) {
				w.Header().Set(vocabHeader, tokens[next])
				next++
			}
			mu.Unlock()
		}
		resp := map[string]any{"path": r.URL.Path}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			if json.Unmarshal(body, &parsed) == nil {
				resp["body"] = parsed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resp)
	})), &seen
}

// snap=true renumbers the tab, so the snapshot it returns must be the one the next
// action echoes. When the navigation's own snapshot was fetched without capturing
// its token, the client kept the token an earlier pinchtab_snapshot left behind and
// the server answered the very refs it had just handed out with 409 vocab_superseded.
func TestNavigateWithSnapCapturesTheTokenItReturns(t *testing.T) {
	srv, _ := rotatingVocabServer("vocab-1", "vocab-2")
	defer srv.Close()
	c := NewClient(srv.URL, "")

	callSharedTool(t, c, "pinchtab_snapshot", map[string]any{"tabId": "t1"})
	callSharedTool(t, c, "pinchtab_navigate", map[string]any{"url": "https://example.com", "tabId": "t1", "snap": true})
	clickResult := callSharedTool(t, c, "pinchtab_click", map[string]any{"ref": "e5", "tabId": "t1"})

	got, ok := actionVocabSent(t, clickResult)
	if !ok {
		t.Fatal("the click carried no vocab token")
	}
	if got != "vocab-2" {
		t.Errorf("click echoed vocab %q, want the token the navigation's own snapshot returned; the refs the agent read came from that snapshot", got)
	}
}

// The follow-up snapshot decides which instance answers, and only the query is read
// when routing. Fetching it unrouted described a different browser's page under the
// action's result — and stashed that page's vocabulary under this tab.
func TestClickWithSnapRoutesItsFollowUpSnapshotToTheSameBrowser(t *testing.T) {
	srv, seen := rotatingVocabServer("vocab-1")
	defer srv.Close()
	c := NewClient(srv.URL, "")

	callSharedTool(t, c, "pinchtab_click", map[string]any{"ref": "e5", "tabId": "t1", "browser": "cloak", "snap": true})

	if len(*seen) != 1 {
		t.Fatalf("got %d snapshot requests, want 1", len(*seen))
	}
	if browser := (*seen)[0].Get("browser"); browser != "cloak" {
		t.Errorf("the follow-up snapshot asked browser=%q, want cloak; routing reads the query and nothing else", browser)
	}
}
