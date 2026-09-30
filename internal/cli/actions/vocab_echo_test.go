package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

// vocabEchoServer stamps a vocabulary token on every /snapshot response and records
// the last /action body so a test can read back what the interaction sent.
func vocabEchoServer(t *testing.T, token string, lastActionBody *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/snapshot" {
			w.Header().Set("X-PinchTab-Tab-Id", r.URL.Query().Get("tabId"))
			w.Header().Set("X-PinchTab-Vocab", token)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		*lastActionBody = string(body)
		_, _ = w.Write([]byte(`{"status":"ok","success":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func actionBodyVocab(t *testing.T, raw string) (string, bool) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("action body was not JSON: %v (%q)", err, raw)
	}
	v, ok := body["vocab"]
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

// A snapshot's vocabulary token is persisted and echoed on the next CLI action for the
// same tab, across the two separate process-like invocations, so the server can refuse
// a ref a later snapshot has renumbered rather than clicking the wrong node. The token
// rides in the response header because the CLI's default snapshot is compact text, which
// has no JSON body to carry it.
func TestCLISnapshotVocabTokenIsEchoedOnTheNextAction(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var lastActionBody string
	srv := vocabEchoServer(t, "cli-tok-1", &lastActionBody)
	client := srv.Client()

	snap := newSnapshotCmd()
	_ = snap.Flags().Set("tab", "t1")
	Snapshot(apiclient.Conn{Client: client, Base: srv.URL}, snap, "")

	act := newActionCmd()
	_ = act.Flags().Set("tab", "t1")
	Action(apiclient.Conn{Client: client, Base: srv.URL}, "click", "e5", act)

	got, ok := actionBodyVocab(t, lastActionBody)
	if !ok {
		t.Fatal("the CLI action carried no vocab token, so a superseded ref would still be mis-resolved")
	}
	if got != "cli-tok-1" {
		t.Errorf("action echoed vocab %q, want the token the snapshot returned", got)
	}
}

// Absence: without a prior snapshot there is nothing to echo, so the action stays
// unchanged — the token is optional and no existing flow gains a spurious refusal.
func TestCLIActionWithoutAPriorSnapshotSendsNoToken(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var lastActionBody string
	srv := vocabEchoServer(t, "cli-tok-1", &lastActionBody)

	act := newActionCmd()
	_ = act.Flags().Set("tab", "t1")
	Action(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "click", "e5", act)

	if got, ok := actionBodyVocab(t, lastActionBody); ok {
		t.Errorf("an action with no prior snapshot echoed a token %q", got)
	}
}

// The token is stored per tab, so a snapshot of one tab does not attach its vocabulary to
// an action on another — that would refuse the second tab's valid refs.
func TestCLIEchoedTokenIsScopedToItsTab(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var lastActionBody string
	srv := vocabEchoServer(t, "cli-tok-1", &lastActionBody)
	client := srv.Client()

	snap := newSnapshotCmd()
	_ = snap.Flags().Set("tab", "t1")
	Snapshot(apiclient.Conn{Client: client, Base: srv.URL}, snap, "")

	act := newActionCmd()
	_ = act.Flags().Set("tab", "t2")
	Action(apiclient.Conn{Client: client, Base: srv.URL}, "click", "e5", act)

	if got, ok := actionBodyVocab(t, lastActionBody); ok {
		t.Errorf("an action on t2 echoed t1's vocabulary token %q", got)
	}
}
