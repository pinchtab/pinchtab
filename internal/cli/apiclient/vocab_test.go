package apiclient_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// vocabServer mints a distinct vocabulary token per (tab, epoch) and records the
// token and tab tag each action echoes, so a test can assert which snapshot's
// token a later action sent and which tab it claimed it for.
type vocabServer struct {
	mu           sync.Mutex
	current      string
	epoch        map[string]int
	lastVocab    string
	lastVocabTab string
	hadVocab     bool
	lastPath     string
	httpServer   *httptest.Server
}

func newVocabServer(current string) *vocabServer {
	s := &vocabServer{current: current, epoch: map[string]int{}}
	s.httpServer = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *vocabServer) tokenFor(tab string) string {
	if s.epoch[tab] == 0 {
		s.epoch[tab] = 1
	}
	return fmt.Sprintf("%s-e%d", tab, s.epoch[tab])
}

func (s *vocabServer) bumpEpoch(tab string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.epoch[tab] == 0 {
		s.epoch[tab] = 1
	}
	s.epoch[tab]++
}

func (s *vocabServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet && r.URL.Path == "/snapshot" {
		resolved := r.URL.Query().Get("tabId")
		if resolved == "" {
			resolved = s.current
		}
		w.Header().Set("X-PinchTab-Tab-Id", resolved)
		w.Header().Set("X-PinchTab-Vocab", s.tokenFor(resolved))
		_, _ = w.Write([]byte("{}\n"))
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/find") {
		resolved := s.current
		if tab := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/tabs/"), "/find"); tab != "" {
			resolved = tab
		}
		w.Header().Set("X-PinchTab-Tab-Id", resolved)
		w.Header().Set("X-PinchTab-Vocab", s.tokenFor(resolved))
		_, _ = w.Write([]byte(`{"best_ref":"e11","matches":[]}`))
		return
	}

	body, _ := io.ReadAll(r.Body)
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	s.lastPath = r.URL.Path
	s.lastVocab, s.hadVocab = "", false
	if v, ok := decoded["vocab"].(string); ok {
		s.lastVocab, s.hadVocab = v, true
	}
	s.lastVocabTab, _ = decoded["vocabTab"].(string)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func snapCmd(tab string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	return cmd
}

func clickCmd(tab string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("json", false, "")
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	return cmd
}

// AC-1 / row (c): an explicit action echoes the token of the snapshot that
// resolved the SAME tab, tagged with that tab, even when that snapshot was
// implicit. On the pre-fix commit the click read the slot keyed by the "--tab X"
// spelling (T1) and missed the implicit snapshot's T2.
func TestExplicitClickSendsTheLatestTokenForItsResolvedTab(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newVocabServer("X")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapCmd("X"), "")
	srv.bumpEpoch("X")
	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapCmd(""), "")
	actions.Action(apiclient.Conn{Client: client, Base: base}, "click", "e5", clickCmd("X"))

	if srv.lastVocab != "X-e2" {
		t.Fatalf("click sent vocab %q (hadVocab=%v), want the current token X-e2", srv.lastVocab, srv.hadVocab)
	}
	if srv.lastVocabTab != "X" {
		t.Fatalf("click tagged the token for tab %q, want X", srv.lastVocabTab)
	}
	if srv.lastPath != "/tabs/X/action" {
		t.Fatalf("click went to %q, want /tabs/X/action", srv.lastPath)
	}
}

// The implicit flow keeps its protection: after an implicit snapshot resolves the
// current tab, an implicit click echoes that tab's token, tagged with it, so the
// server can refuse a superseded ref. The pre-fix (omit) commit sent no token
// here, which is the silent-wrong-click regression this restores.
func TestImplicitClickSendsTheCurrentTabsTaggedToken(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	srv := newVocabServer("X")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapCmd(""), "")
	actions.Action(apiclient.Conn{Client: client, Base: base}, "click", "e5", clickCmd(""))

	if !srv.hadVocab {
		t.Fatal("implicit click sent no token; the common snap-then-click flow lost its supersession protection")
	}
	if srv.lastVocab != "X-e1" || srv.lastVocabTab != "X" {
		t.Fatalf("implicit click sent vocab %q tagged %q, want X-e1 tagged X", srv.lastVocab, srv.lastVocabTab)
	}
}

func findCmd(tab string, flag string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("threshold", "", "")
	cmd.Flags().Bool("explain", false, "")
	cmd.Flags().Bool("ref-only", false, "")
	cmd.Flags().Bool("json", false, "")
	if tab != "" {
		_ = cmd.Flags().Set("tab", tab)
	}
	if flag != "" {
		_ = cmd.Flags().Set(flag, "true")
	}
	return cmd
}

func TestFindCapturesTheVocabularyItMintedSoTheNextClickEchoesIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		tab  string
		flag string
	}{
		{"ref-only", "", "ref-only"},
		{"json", "", "json"},
		{"terse", "", ""},
		{"explicit tab", "X", "ref-only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			srv := newVocabServer("X")
			defer srv.httpServer.Close()
			base, client := srv.httpServer.URL, srv.httpServer.Client()

			actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapCmd(tc.tab), "")
			srv.bumpEpoch("X")
			actions.Find(apiclient.Conn{Client: client, Base: base}, "Go page 2", findCmd(tc.tab, tc.flag))
			actions.Action(apiclient.Conn{Client: client, Base: base}, "click", "e11", clickCmd(tc.tab))

			if srv.lastVocab != "X-e2" || srv.lastVocabTab != "X" {
				t.Fatalf("click after find sent vocab %q tagged %q, want the token find published (X-e2) tagged X", srv.lastVocab, srv.lastVocabTab)
			}
		})
	}
}

// AC-3: the per-tab files are replaced by one bounded file per server, written
// 0600, holding at most the N most recent tabs plus the current pointer.
func TestVocabStoreIsOneBoundedFilePerServer(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	srv := newVocabServer("")
	defer srv.httpServer.Close()
	base, client := srv.httpServer.URL, srv.httpServer.Client()

	for i := range 50 {
		actions.Snapshot(apiclient.Conn{Client: client, Base: base}, snapCmd(fmt.Sprintf("tab-%02d", i)), "")
	}

	files, err := filepath.Glob(filepath.Join(stateHome, "pinchtab", "vocab-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected exactly one vocab store file per server, found %d: %v", len(files), files)
	}

	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("vocab store written %o, want 0600 (it holds a capability token on a possibly shared path)", perm)
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var store struct {
		Current string `json:"current"`
		Entries []struct {
			TabID string `json:"tabId"`
			Token string `json:"token"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("vocab store is not the bounded JSON map: %v\n%s", err, data)
	}
	if len(store.Entries) == 0 || len(store.Entries) > 16 {
		t.Fatalf("store holds %d entries after 50 snapshots, want 1..16", len(store.Entries))
	}
}
