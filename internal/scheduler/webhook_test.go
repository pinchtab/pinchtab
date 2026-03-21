package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

func configureWebhookTestTarget(t *testing.T, serverURL string, timeout time.Duration) (string, *atomic.Value, func()) {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	targetAddr := parsed.Host
	callbackURL := parsed.Scheme + "://callback.example"
	if parsed.Port() != "" {
		callbackURL += ":" + parsed.Port()
	}
	callbackURL += parsed.Path

	origResolver := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		if strings.EqualFold(host, "callback.example") {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return origResolver(ctx, network, host)
	}

	dialedAddr := &atomic.Value{}
	dialedAddr.Store("")
	pinnedDialAddr := net.JoinHostPort("93.184.216.34", parsed.Port())

	origTimeout := webhookHTTPTimeout
	webhookHTTPTimeout = timeout

	origDial := dialWebhookAddress
	dialWebhookAddress = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialedAddr.Store(addr)
		if addr == pinnedDialAddr {
			addr = targetAddr
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}

	cleanup := func() {
		dialWebhookAddress = origDial
		webhookHTTPTimeout = origTimeout
		netguard.ResolveHostIPs = origResolver
	}
	return callbackURL, dialedAddr, cleanup
}

func TestSendWebhookSuccess(t *testing.T) {
	var received atomic.Bool
	var gotBody []byte
	var gotHeaders http.Header
	var gotHost string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		gotHeaders = r.Header.Clone()
		gotHost = r.Host
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	callbackURL, dialedAddr, cleanup := configureWebhookTestTarget(t, srv.URL, 10*time.Second)
	defer cleanup()

	task := &Task{
		ID:      "tsk_webhook_1",
		AgentID: "a1",
		Action:  "click",
		State:   StateDone,
	}

	sendWebhook(callbackURL, task)

	if !received.Load() {
		t.Fatal("webhook was never received")
	}

	// Verify headers.
	if ct := gotHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	if ev := gotHeaders.Get("X-PinchTab-Event"); ev != "task.completed" {
		t.Errorf("expected task.completed event, got %s", ev)
	}
	if tid := gotHeaders.Get("X-PinchTab-Task-ID"); tid != "tsk_webhook_1" {
		t.Errorf("expected task ID header, got %s", tid)
	}
	if !strings.HasPrefix(gotHost, "callback.example") {
		t.Fatalf("expected request host to preserve callback hostname, got %q", gotHost)
	}
	if !strings.HasPrefix(dialedAddr.Load().(string), "93.184.216.34:") {
		t.Fatalf("expected webhook dial to use pinned IP, got %q", dialedAddr.Load().(string))
	}

	// Verify body is a valid task snapshot.
	var snap Task
	if err := json.Unmarshal(gotBody, &snap); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if snap.ID != "tsk_webhook_1" {
		t.Errorf("expected task ID in body, got %s", snap.ID)
	}
}

func TestSendWebhookEmptyURL(t *testing.T) {
	// Should be a no-op, no panic.
	sendWebhook("", &Task{ID: "tsk_empty"})
}

func TestSendWebhookUnsupportedScheme(t *testing.T) {
	sendWebhook("file:///etc/passwd", &Task{ID: "tsk_scheme_1"})
	sendWebhook("ftp://malicious.host/data", &Task{ID: "tsk_scheme_2"})
}

func TestSendWebhookInvalidURL(t *testing.T) {
	sendWebhook("://bad-url", &Task{ID: "tsk_bad_url"})
}

func TestSendWebhookRejectedHost(t *testing.T) {
	sendWebhook("http://127.0.0.1/hook", &Task{ID: "tsk_blocked"})
	sendWebhook("http://169.254.169.254/latest/meta-data", &Task{ID: "tsk_blocked2"})
}

func TestValidateCallbackURLRejectsResolvedBlockedHost(t *testing.T) {
	origResolver := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.8")}, nil
	}
	defer func() { netguard.ResolveHostIPs = origResolver }()

	if err := validateCallbackURL("http://callback.example/hook"); err == nil {
		t.Fatal("expected resolved private host to be rejected")
	}
}

func TestSendWebhookServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	callbackURL, _, cleanup := configureWebhookTestTarget(t, srv.URL, 10*time.Second)
	defer cleanup()

	sendWebhook(callbackURL, &Task{ID: "tsk_500", State: StateFailed})
}

func TestSendWebhookRedirectNotFollowed(t *testing.T) {
	var firstHit atomic.Bool
	var secondHit atomic.Bool

	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHit.Store(true)
		w.WriteHeader(200)
	}))
	defer redirectTarget.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHit.Store(true)
		http.Redirect(w, r, redirectTarget.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	callbackURL, _, cleanup := configureWebhookTestTarget(t, srv.URL, 10*time.Second)
	defer cleanup()

	sendWebhook(callbackURL, &Task{ID: "tsk_redirect"})

	if !firstHit.Load() {
		t.Fatal("expected initial webhook target to receive the request")
	}
	if secondHit.Load() {
		t.Fatal("redirect target should not be reached when redirects are disabled")
	}
}

func TestSendWebhookTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	callbackURL, _, cleanup := configureWebhookTestTarget(t, srv.URL, 10*time.Millisecond)
	defer cleanup()

	sendWebhook(callbackURL, &Task{ID: "tsk_timeout"})
}

func TestWebhookFiredOnFinishTask(t *testing.T) {
	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	callbackURL, _, cleanup := configureWebhookTestTarget(t, srv.URL, 10*time.Second)
	defer cleanup()

	s, executor := newTestScheduler(t)
	defer executor.Close()

	task := &Task{
		ID:          "tsk_cb_1",
		AgentID:     "a1",
		Action:      "click",
		State:       StateDone,
		CallbackURL: callbackURL,
	}
	s.live["tsk_cb_1"] = task

	s.finishTask(task)

	// Give goroutine time to fire.
	time.Sleep(200 * time.Millisecond)

	if !received.Load() {
		t.Error("webhook should have been fired from finishTask")
	}
}

func TestWebhookNotFiredWithoutCallback(t *testing.T) {
	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s, executor := newTestScheduler(t)
	defer executor.Close()

	task := &Task{
		ID:      "tsk_no_cb",
		AgentID: "a1",
		State:   StateDone,
	}
	s.live["tsk_no_cb"] = task

	s.finishTask(task)
	time.Sleep(100 * time.Millisecond)

	if received.Load() {
		t.Error("webhook should not fire when no callbackUrl")
	}
}
