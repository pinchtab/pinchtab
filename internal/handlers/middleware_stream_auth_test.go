package handlers

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func shortenStreamRevalidate(t *testing.T) {
	t.Helper()
	prev := streamRevalidateInterval
	streamRevalidateInterval = 25 * time.Millisecond
	t.Cleanup(func() { streamRevalidateInterval = prev })
}

// blockingSSE commits a text/event-stream response, writes one event, then holds
// the connection open until its request context is cancelled — standing in for
// every SSE handler that loops on r.Context().Done(). It never inspects the
// request's Accept header, which is the point of the handler-driven detection.
func blockingSSE(started chan<- struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprint(w, "data: open\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		<-r.Context().Done()
	}
}

func awaitStreamEnd(t *testing.T, resp *http.Response, started <-chan struct{}, revoke func()) bool {
	t.Helper()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("stream never delivered its first event: %v", err)
	}
	<-started

	revoke()

	ended := make(chan struct{})
	go func() {
		var err error
		for err == nil {
			_, err = reader.ReadString('\n')
		}
		close(ended)
	}()

	select {
	case <-ended:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

// TestStreamEndsWhenCookieSessionRevoked sends NO Accept header: detection must
// come from the handler committing text/event-stream, not from what the client
// asked for. A holder of a logged-out cookie opens the stream exactly this way.
func TestStreamEndsWhenCookieSessionRevoked(t *testing.T) {
	shortenStreamRevalidate(t)

	cfg := &config.RuntimeConfig{Token: "server-secret"}
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started := make(chan struct{})
	handler := AuthMiddlewareWithSessions(config.NewLive(cfg), sessions, nil, blockingSSE(started))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	req.Header.Set("Origin", srv.URL)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream did not open: status %d", resp.StatusCode)
	}

	if !awaitStreamEnd(t, resp, started, func() { sessions.Revoke(sessionID) }) {
		t.Fatal("stream kept delivering after the cookie session was revoked (logout)")
	}
}

func TestStreamHandlersBehindSessionAuthCanClearTheirWriteDeadline(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "server-secret"}
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var deadlineErr error
	handler := AuthMiddlewareWithSessions(config.NewLive(cfg), sessions, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadlineErr = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	req.Header.Set("Origin", srv.URL)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	_ = resp.Body.Close()

	if deadlineErr != nil {
		t.Fatalf("SetWriteDeadline through the session-auth writer = %v; the dashboard and logs SSE handlers answer 500 on this, so the stream-auth wrapper must expose Unwrap", deadlineErr)
	}
}

func TestStreamEndsWhenAgentSessionRevoked(t *testing.T) {
	shortenStreamRevalidate(t)

	cfg := &config.RuntimeConfig{Token: "server-secret"}
	store := session.NewStore(session.Config{Enabled: true, MaxLifetime: time.Hour, IdleTimeout: time.Hour})
	sessID, token, err := store.Create("agent-1", "test", "")
	if err != nil {
		t.Fatalf("create agent session: %v", err)
	}

	started := make(chan struct{})
	handler := AuthMiddlewareWithSessions(config.NewLive(cfg), nil, store, blockingSSE(started))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/network/stream", nil)
	req.Header.Set("Authorization", "Session "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream did not open: status %d", resp.StatusCode)
	}

	if !awaitStreamEnd(t, resp, started, func() { store.Revoke(sessID) }) {
		t.Fatal("stream kept delivering after the agent session was revoked")
	}
}

// TestNonStreamRequestSpawnsNoStreamGoroutine is the must-stay-green row: a short
// request that never commits a stream must start no poller goroutine.
func TestNonStreamRequestSpawnsNoStreamGoroutine(t *testing.T) {
	shortenStreamRevalidate(t)

	cfg := &config.RuntimeConfig{Token: "server-secret"}
	sessions := browsersession.NewManager(browsersession.Config{})
	sessionID, err := sessions.Create(cfg.Token)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := AuthMiddlewareWithSessions(config.NewLive(cfg), sessions, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
		req.Header.Set("Origin", "http://example.test")
		req.Host = "example.test"
		req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
		return req
	}

	handler.ServeHTTP(httptest.NewRecorder(), newReq())
	runtime.Gosched()
	baseline := runtime.NumGoroutine()
	for i := 0; i < 200; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), newReq())
	}

	deadline := time.Now().Add(time.Second)
	for {
		runtime.Gosched()
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("non-stream requests leaked goroutines: baseline %d, now %d", baseline, runtime.NumGoroutine())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// streamSiteFloor is every production site that opens a long-lived response — the
// four SSE endpoints, the screencast websocket, and the websocket proxy tunnel.
// Each is served through the front door's AuthMiddlewareWithSessions (directly or
// via the proxy it fronts), so the stream-aware credential re-check reaches them
// all. A new site must be added here deliberately, which is where its routing
// behind that middleware is confirmed.
var streamSiteFloor = []string{
	"dashboard/handlers_sse.go",
	"handlers/network.go",
	"handlers/network_export_stream.go",
	"orchestrator/handlers_instances.go",
	"handlers/screencast.go",
	"proxy/proxy_ws.go",
}

// streamSitePlumbing are the ResponseWriter Hijack forwarders: they relay Hijack
// to the underlying writer rather than opening a stream of their own, so they
// match the hijack matcher without being endpoints. Recorded so a genuinely new
// endpoint cannot hide among them.
var streamSitePlumbing = map[string]string{
	"httpx/httpx.go":         "StatusWriter.Hijack forwards to the underlying ResponseWriter",
	"handlers/middleware.go": "streamAuthWriter.Hijack forwards; this file is the stream-aware middleware itself",
}

func basicString(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

// streamSiteKinds parses one source file and reports whether it commits a stream:
// an SSE Content-Type header set, a ws.UpgradeHTTP call, or a Hijack call. It
// matches AST call expressions, so a comment or a request-side Accept header set
// (Set("Accept", ...)) is not a false positive.
func streamSiteKinds(t *testing.T, name, src string) (sse, upgrade, hijack bool) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "UpgradeHTTP":
			upgrade = true
		case "Hijack":
			hijack = true
		case "Set":
			if len(call.Args) == 2 && basicString(call.Args[0]) == "Content-Type" && basicString(call.Args[1]) == "text/event-stream" {
				sse = true
			}
		}
		return true
	})
	return
}

func TestStreamSiteCensusStaysBehindStreamAwareMiddleware(t *testing.T) {
	floor := map[string]bool{}
	for _, name := range streamSiteFloor {
		floor[name] = false
	}

	for _, f := range srccensus.Tree(t, "..", 150) {
		sse, upgrade, hijack := streamSiteKinds(t, f.Name, f.Text)
		if !sse && !upgrade && !hijack {
			continue
		}
		if _, ok := floor[f.Name]; ok {
			floor[f.Name] = true
			continue
		}
		if _, ok := streamSitePlumbing[f.Name]; ok {
			continue
		}
		t.Errorf("%s opens a stream (sse=%v upgrade=%v hijack=%v) but is not classified; confirm it is served behind AuthMiddlewareWithSessions and add it to streamSiteFloor", f.Name, sse, upgrade, hijack)
	}

	for name, seen := range floor {
		if !seen {
			t.Errorf("floor stream site %s was not found; the census floor of %d is not met", name, len(floor))
		}
	}

	if len(srccensus.Load(t, filepath.Join("..", "server"), 8).CallsAllowingNone("handlers.AuthMiddlewareWithSessions")) == 0 {
		t.Fatal("front door no longer mounts AuthMiddlewareWithSessions; the stream-aware credential re-check does not reach the stream sites")
	}
}

func TestStreamSiteCensusMatcherDetectsPlantedSites(t *testing.T) {
	sse, _, _ := streamSiteKinds(t, "planted.go", "package p\nimport \"net/http\"\nfunc H(w http.ResponseWriter) { w.Header().Set(\"Content-Type\", \"text/event-stream\") }\n")
	if !sse {
		t.Error("matcher missed a planted SSE Content-Type site")
	}

	_, upgrade, _ := streamSiteKinds(t, "planted.go", "package p\nfunc H(r, w any) { ws.UpgradeHTTP(r, w) }\n")
	if !upgrade {
		t.Error("matcher missed a planted ws.UpgradeHTTP site")
	}

	_, _, hijack := streamSiteKinds(t, "planted.go", "package p\nfunc H(hj any) { hj.Hijack() }\n")
	if !hijack {
		t.Error("matcher missed a planted Hijack site")
	}

	noise, _, _ := streamSiteKinds(t, "planted.go", "package p\nimport \"net/http\"\nfunc H(req *http.Request) { req.Header.Set(\"Accept\", \"text/event-stream\") }\n")
	if noise {
		t.Error("matcher treated a request-side Accept header as an SSE stream site")
	}
}
