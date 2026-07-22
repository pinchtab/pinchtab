package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestInitBrowserFromExistingCDPRefreshesStaleBrowserWebSocket(t *testing.T) {
	var versionCalls, staleDials, freshDials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			id := "stale"
			if versionCalls.Add(1) > 1 {
				id = "fresh"
			}
			_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/%s"}`, r.Host, id)
		case "/devtools/browser/stale":
			staleDials.Add(1)
			http.NotFound(w, r)
		case "/devtools/browser/fresh":
			freshDials.Add(1)
			serveMinimalCDP(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &config.RuntimeConfig{
		CDPAttachURL:       server.URL,
		AttachAllowSchemes: []string{"http"},
	}
	allocCtx, allocCancel, browserCtx, browserCancel, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err != nil {
		t.Fatalf("attach did not refresh the stale browser WebSocket: %v", err)
	}
	if allocCtx == nil || browserCtx == nil {
		t.Fatal("attach returned nil contexts")
	}
	browserCancel()
	allocCancel()

	if got := versionCalls.Load(); got != 2 {
		t.Fatalf("/json/version calls = %d, want one initial resolution plus one refresh", got)
	}
	if got := staleDials.Load(); got != 1 {
		t.Fatalf("stale WebSocket dials = %d, want 1", got)
	}
	if got := freshDials.Load(); got != 1 {
		t.Fatalf("fresh WebSocket dials = %d, want 1", got)
	}
}

func TestInitBrowserFromExistingCDPRefreshesSuppliedStaleBrowserWebSocket(t *testing.T) {
	var versionCalls, staleDials, freshDials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			versionCalls.Add(1)
			_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/fresh"}`, r.Host)
		case "/devtools/browser/stale":
			staleDials.Add(1)
			http.NotFound(w, r)
		case "/devtools/browser/fresh":
			freshDials.Add(1)
			serveMinimalCDP(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &config.RuntimeConfig{
		CDPAttachURL:       "ws://" + strings.TrimPrefix(server.URL, "http://") + "/devtools/browser/stale",
		AttachAllowSchemes: []string{"ws"},
	}
	_, allocCancel, _, browserCancel, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err != nil {
		t.Fatalf("attach did not refresh the supplied stale browser WebSocket: %v", err)
	}
	browserCancel()
	allocCancel()

	if got := versionCalls.Load(); got != 1 {
		t.Fatalf("/json/version calls = %d, want 1 refresh", got)
	}
	if got := staleDials.Load(); got != 1 {
		t.Fatalf("stale WebSocket dials = %d, want 1", got)
	}
	if got := freshDials.Load(); got != 1 {
		t.Fatalf("fresh WebSocket dials = %d, want 1", got)
	}
}

func TestInitBrowserFromExistingCDPRetriesStaleEndpointOnlyOnce(t *testing.T) {
	var versionCalls, staleDials, freshDials atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			id := "stale"
			if versionCalls.Add(1) > 1 {
				id = "fresh"
			}
			_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/%s"}`, r.Host, id)
		case "/devtools/browser/stale":
			staleDials.Add(1)
			http.NotFound(w, r)
		case "/devtools/browser/fresh":
			freshDials.Add(1)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := &config.RuntimeConfig{CDPAttachURL: server.URL, AttachAllowSchemes: []string{"http"}}
	_, _, _, _, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err == nil {
		t.Fatal("expected the refreshed endpoint's 404 to be returned")
	}
	if got := versionCalls.Load(); got != 2 {
		t.Fatalf("/json/version calls = %d, want initial resolution plus one refresh", got)
	}
	if got := staleDials.Load(); got != 1 {
		t.Fatalf("stale WebSocket dials = %d, want 1", got)
	}
	if got := freshDials.Load(); got != 1 {
		t.Fatalf("fresh WebSocket dials = %d, want exactly 1", got)
	}
}

func TestRefreshStaleCDPURLDoesNotRefreshOtherFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "authentication rejection", err: fmt.Errorf("dial: %w", ws.StatusError(http.StatusUnauthorized))},
		{name: "server failure", err: fmt.Errorf("dial: %w", ws.StatusError(http.StatusInternalServerError))},
		{name: "connection failure", err: errors.New("connection refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := refreshStaleCDPURL("http://127.0.0.1:1", "ws://127.0.0.1:1/devtools/browser/stale", tc.err, &config.RuntimeConfig{})
			if err != nil {
				t.Fatalf("unexpected refresh error: %v", err)
			}
			if got != "" {
				t.Fatalf("refresh URL = %q, want none", got)
			}
		})
	}
}

func TestRefreshStaleCDPURLRequiresChangedBrowserPath(t *testing.T) {
	var versionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		versionCalls.Add(1)
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://%s/devtools/browser/stale"}`, r.Host)
	}))
	defer server.Close()

	staleURL := "ws://" + strings.TrimPrefix(server.URL, "http://") + "/devtools/browser/stale"
	got, err := refreshStaleCDPURL(
		server.URL,
		staleURL,
		fmt.Errorf("dial: %w", ws.StatusError(http.StatusNotFound)),
		&config.RuntimeConfig{AttachAllowSchemes: []string{"http"}},
	)
	if err != nil {
		t.Fatalf("unexpected refresh error: %v", err)
	}
	if got != "" {
		t.Fatalf("refresh URL = %q, want none for the unchanged browser path", got)
	}
	if got := versionCalls.Load(); got != 1 {
		t.Fatalf("/json/version calls = %d, want 1 proof check", got)
	}
}

func serveMinimalCDP(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		t.Errorf("upgrade fresh WebSocket: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	for {
		payload, op, err := wsutil.ReadClientData(conn)
		if err != nil {
			return
		}
		if op != ws.OpText {
			continue
		}
		var command struct {
			ID        int64  `json:"id"`
			Method    string `json:"method"`
			SessionID string `json:"sessionId,omitempty"`
		}
		if err := json.Unmarshal(payload, &command); err != nil {
			t.Errorf("decode CDP command: %v", err)
			return
		}

		result := `{}`
		switch command.Method {
		case "Target.createTarget":
			result = `{"targetId":"page-1"}`
		case "Target.attachToTarget":
			result = `{"sessionId":"session-1"}`
		case "Runtime.evaluate":
			result = `{"result":{"type":"object","className":"Window"}}`
		}
		response := fmt.Sprintf(`{"id":%d,"result":%s`, command.ID, result)
		if command.SessionID != "" {
			response += fmt.Sprintf(`,"sessionId":%q`, command.SessionID)
		}
		response += `}`
		if err := wsutil.WriteServerMessage(conn, ws.OpText, []byte(response)); err != nil {
			return
		}
	}
}
