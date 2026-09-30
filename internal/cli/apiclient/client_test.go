package apiclient_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// epochServer models a server whose ref vocabulary is renumbered (a fresh token
// minted) whenever the tab navigates. /navigate and /action bump the epoch; every
// /snapshot returns the token of the CURRENT epoch. It records the token the last
// action echoed so a test can see whether a ref action sent the latest snapshot's
// token or a stale one the server would refuse 409.
type epochServer struct {
	mu         sync.Mutex
	tab        string
	epoch      int
	snapStatus int // >=400 makes /snapshot fail, to pin best-effort behaviour
	lastVocab  string
	hadVocab   bool
	httpServer *httptest.Server
}

func newEpochServer(tab string) *epochServer {
	s := &epochServer{tab: tab, epoch: 1}
	s.httpServer = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *epochServer) token() string {
	return s.tab + "-e" + itoa(s.epoch)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *epochServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/snapshot":
		if s.snapStatus >= 400 {
			w.WriteHeader(s.snapStatus)
			_, _ = w.Write([]byte("snapshot blew up"))
			return
		}
		w.Header().Set("X-PinchTab-Tab-Id", s.tab)
		w.Header().Set("X-PinchTab-Vocab", s.token())
		_, _ = w.Write([]byte("e0 button \"Go\"\n"))
	case r.URL.Path == "/navigate":
		s.epoch++ // navigation renumbers refs, minting a fresh token
		_, _ = w.Write([]byte(`{"tabId":"` + s.tab + `","url":"http://example.test"}`))
	default: // an action (/action or /tabs/<id>/action)
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		s.lastVocab, s.hadVocab = "", false
		if v, ok := decoded["vocab"].(string); ok {
			s.lastVocab, s.hadVocab = v, true
		}
		s.epoch++ // the action changed the page, renumbering refs as a navigation would
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

// navCmd builds a command carrying the flags Navigate reads, so a test can drive
// the real nav --snap / --snap-diff tail.
func navCmd(tab string, snap, snapDiff bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Float64("timeout", 0, "")
	for _, name := range []string{"new-tab", "block-images", "block-ads", "dismiss-banners", "json", "snap", "snap-diff", "text", "print-tab-id"} {
		cmd.Flags().Bool(name, false, "")
	}
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	if snap {
		_ = cmd.Flags().Set("snap", "true")
	}
	if snapDiff {
		_ = cmd.Flags().Set("snap-diff", "true")
	}
	return cmd
}

// actionCmd builds a command carrying the flags an element action reads.
func actionCmd(tab string, snap bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	for _, name := range []string{"json", "snap", "snap-diff", "text"} {
		cmd.Flags().Bool(name, false, "")
	}
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	if snap {
		_ = cmd.Flags().Set("snap", "true")
	}
	return cmd
}

// AC-1: after `nav <url> --snap`, a ref action on a printed ref echoes the token
// the post-nav snapshot returned — not the token of an earlier snapshot that the
// navigation superseded. An earlier implicit snapshot files X-e1; the navigate
// bumps the epoch to 2 and nav --snap fetches X-e2; the following implicit action
// must echo X-e2. On HEAD the --snap tail discarded the token, so the action echoed
// the stale X-e1 the server refuses 409.
func TestNavSnapCapturesTokenSoNextActionIsNotRefused(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newEpochServer("X")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapshotOnlyCmd(""), "") // files X-e1, current=X
	actions.Navigate(apiclient.Conn{Client: client, Base: base}, "http://example.test", navCmd("", true, false))
	actions.Action(apiclient.Conn{Client: client, Base: base}, "fill", "e0", actionCmd("", false))

	if !srv.hadVocab {
		t.Fatal("the action after nav --snap sent no token; a superseded ref would be resolved positionally")
	}
	if srv.lastVocab != "X-e2" {
		t.Fatalf("action echoed %q, want X-e2 (the token nav --snap returned); X-e1 is the stale pre-nav token the server 409s", srv.lastVocab)
	}
}

// AC-3: --snap-diff captures identically to --snap.
func TestNavSnapDiffCapturesTokenToo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newEpochServer("X")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapshotOnlyCmd(""), "")
	actions.Navigate(apiclient.Conn{Client: client, Base: base}, "http://example.test", navCmd("", false, true))
	actions.Action(apiclient.Conn{Client: client, Base: base}, "fill", "e0", actionCmd("", false))

	if srv.lastVocab != "X-e2" {
		t.Fatalf("action after nav --snap-diff echoed %q, want X-e2", srv.lastVocab)
	}
}

// AC-2: the post-action --snap tail captures the new token when the action itself
// changed the epoch, so the NEXT ref action is not refused either. The click bumps
// the epoch to 2 and its --snap tail fetches X-e2; the following action echoes X-e2.
func TestPostActionSnapCapturesTheNewToken(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newEpochServer("X")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapshotOnlyCmd(""), "") // files X-e1, current=X
	actions.Action(apiclient.Conn{Client: client, Base: base}, "click", "e0", actionCmd("", true))
	actions.Action(apiclient.Conn{Client: client, Base: base}, "fill", "e0", actionCmd("", false))

	if srv.lastVocab != "X-e2" {
		t.Fatalf("the second action echoed %q, want X-e2 (captured by the first click's --snap tail)", srv.lastVocab)
	}
}

// The review note's floor: when the trailing snapshot fails, the tail stays
// best-effort — it warns and returns rather than exiting the process, so an action
// that already succeeded is not turned into a non-zero exit. A capture helper that
// called exitOnAPIError here would terminate the test binary before this assertion.
func TestSnapTailDoesNotExitWhenSnapshotFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newEpochServer("X")
	srv.snapStatus = 500
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Action(apiclient.Conn{Client: client, Base: base}, "click", "e0", actionCmd("", true))

	if tok := apiclient.VocabTokenFor(base, "X"); tok != "" {
		t.Fatalf("a failed snapshot stored a token %q; nothing should be captured from a non-2xx", tok)
	}
}

// snapshotOnlyCmd carries just the flags actions.Snapshot reads.
func snapshotOnlyCmd(tab string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().String("max-tokens", "", "")
	cmd.Flags().String("depth", "", "")
	for _, name := range []string{"full", "interactive", "diff", "text", "json", "compact"} {
		cmd.Flags().Bool(name, false, "")
	}
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	return cmd
}
