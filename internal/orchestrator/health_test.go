package orchestrator

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

type exitingMockCmd struct {
	waitErr error
}

func (m *exitingMockCmd) Wait() error { return m.waitErr }
func (m *exitingMockCmd) PID() int    { return 4242 }
func (m *exitingMockCmd) Cancel()     {}

func TestMonitor_SetsLastFailureReasonOnProcessExit(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	// Stub the orchestrator HTTP client so the health probe never accidentally
	// connects to a real service on the test port (e.g. a developer's running
	// PinchTab on :9999). The test needs the monitor to land in the
	// "exited early without ever becoming healthy" branch.
	o.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("stub: connection refused")
		}),
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:     "inst_test0001",
			Port:   "9999",
			Status: "starting",
		},
		URL:    "http://127.0.0.1:9999",
		cmd:    &exitingMockCmd{waitErr: errors.New("signal: killed")},
		logBuf: newRingBuffer(1024),
	}
	o.mu.Lock()
	o.instances[inst.ID] = inst
	o.mu.Unlock()

	o.monitor(inst)

	o.mu.RLock()
	defer o.mu.RUnlock()
	if inst.lastFailureReason != ReasonProcessExited {
		t.Fatalf("lastFailureReason = %q, want %q", inst.lastFailureReason, ReasonProcessExited)
	}
	if !strings.Contains(inst.Error, "process exited before health check") {
		t.Errorf("expected process-exit error message, got %q", inst.Error)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// waitForChildBridgeHealthy must only promote starting -> running. If the
// concurrent monitor() goroutine already moved the instance to a terminal
// "error" state, a health 200 must not resurrect it to "running".
func TestWaitForChildBridgeHealthy_DoesNotOverwriteErrorStatus(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_err0001", Port: "9999", Status: "error"},
		URL:      "http://127.0.0.1:9999",
	}
	o.mu.Lock()
	o.instances[inst.ID] = inst
	o.mu.Unlock()

	if err := o.waitForChildBridgeHealthy(inst, time.Second); err != nil {
		t.Fatalf("waitForChildBridgeHealthy returned error on 200: %v", err)
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if inst.Status != "error" {
		t.Fatalf("status = %q, want error (health 200 must not resurrect a terminal state)", inst.Status)
	}
}

func TestWaitForChildBridgeHealthy_PromotesStartingToRunning(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_ok0001", Port: "9999", Status: "starting"},
		URL:      "http://127.0.0.1:9999",
	}
	o.mu.Lock()
	o.instances[inst.ID] = inst
	o.mu.Unlock()

	if err := o.waitForChildBridgeHealthy(inst, time.Second); err != nil {
		t.Fatalf("waitForChildBridgeHealthy returned error: %v", err)
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if inst.Status != "running" {
		t.Fatalf("status = %q, want running", inst.Status)
	}
}

func TestIsInstanceHealthyStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{http.StatusOK, true},
		{http.StatusNotFound, true},
		{http.StatusBadRequest, true},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{0, false},
	}

	for _, tt := range tests {
		if got := isInstanceHealthyStatus(tt.code); got != tt.want {
			t.Errorf("isInstanceHealthyStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestInstanceBaseURLs(t *testing.T) {
	urls := instanceBaseURLs("", 1234)

	expected := []string{
		"http://127.0.0.1:1234",
		"http://[::1]:1234",
		"http://localhost:1234",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d", len(expected), len(urls))
	}

	for i, url := range urls {
		if url != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, url, expected[i])
		}
	}
}

func TestInstanceBaseURLs_IncludesConfiguredBindFirst(t *testing.T) {
	urls := instanceBaseURLs("192.168.1.50", 1234)

	expected := []string{
		"http://192.168.1.50:1234",
		"http://127.0.0.1:1234",
		"http://[::1]:1234",
		"http://localhost:1234",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d", len(expected), len(urls))
	}

	for i, url := range urls {
		if url != expected[i] {
			t.Errorf("url[%d] = %q, want %q", i, url, expected[i])
		}
	}
}

func TestValidatedHealthProbeBaseURL_AllowsConfiguredChildBindHost(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{Bind: "192.168.1.50"})

	baseURL, err := o.validatedHealthProbeBaseURL("http://192.168.1.50:9872", "", healthProbePolicyLoopback)
	if err != nil {
		t.Fatalf("validatedHealthProbeBaseURL() error = %v", err)
	}
	if got := baseURL.String(); got != "http://192.168.1.50:9872" {
		t.Fatalf("validatedHealthProbeBaseURL() = %q, want %q", got, "http://192.168.1.50:9872")
	}
}

func TestProbeInstanceHealth_AllowsConfiguredAttachedBridgeHost(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	var requestedURL string
	o.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedURL = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			Attached:   true,
			AttachType: "bridge",
		},
		URL: "http://10.0.0.8:9868",
	}

	healthy, resolvedURL, lastProbe := o.probeInstanceHealth(inst)
	if !healthy {
		t.Fatalf("expected attached bridge to probe healthy, got healthy=false lastProbe=%q", lastProbe)
	}
	if resolvedURL != inst.URL {
		t.Fatalf("resolvedURL = %q, want %q", resolvedURL, inst.URL)
	}
	if requestedURL != inst.URL+"/health" {
		t.Fatalf("requested URL = %q, want %q", requestedURL, inst.URL+"/health")
	}
}

func TestProbeInstanceHealth_RejectsUnsupportedScheme(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	called := false
	o.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	healthy, resolvedURL, lastProbe := o.probeInstanceHealth(&InstanceInternal{
		Instance: bridge.Instance{
			Attached:   true,
			AttachType: "bridge",
		},
		URL: "ftp://10.0.0.8:9868",
	})
	if healthy {
		t.Fatal("expected unsupported scheme to be rejected")
	}
	if resolvedURL != "" {
		t.Fatalf("resolvedURL = %q, want empty", resolvedURL)
	}
	if called {
		t.Fatal("probe should not issue a request for an unsupported scheme")
	}
	if !strings.Contains(lastProbe, "not an HTTP bridge") {
		t.Fatalf("lastProbe = %q, want invalid HTTP bridge message", lastProbe)
	}
}

func TestProbeInstanceHealth_TagsBridgeHealthProbeAsOrchestrator(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	var source string
	o.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			source = req.Header.Get("X-PinchTab-Source")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	healthy, _, _ := o.probeInstanceHealth(&InstanceInternal{
		Instance: bridge.Instance{
			Attached:   true,
			AttachType: "bridge",
		},
		URL: "http://10.0.0.8:9868",
	})
	if !healthy {
		t.Fatal("expected probe to succeed")
	}
	if source != "orchestrator" {
		t.Fatalf("X-PinchTab-Source = %q, want orchestrator", source)
	}
}

func TestFetchTabs_TagsBridgeRequestAsOrchestrator(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	var source string
	o.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			source = req.Header.Get("X-PinchTab-Source")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"tabs":[]}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err := o.fetchTabs(&InstanceInternal{
		Instance: bridge.Instance{ID: "inst-1"},
		URL:      "http://127.0.0.1:9868",
	})
	if err != nil {
		t.Fatalf("fetchTabs() error = %v", err)
	}
	if source != "orchestrator" {
		t.Fatalf("X-PinchTab-Source = %q, want orchestrator", source)
	}
}

func TestFetchMetrics_TagsBridgeRequestAsOrchestrator(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	var source string
	o.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			source = req.Header.Get("X-PinchTab-Source")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"memory":{"memoryMB":1}}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err := o.fetchMetrics(&InstanceInternal{
		Instance: bridge.Instance{ID: "inst-1"},
		URL:      "http://127.0.0.1:9868",
	})
	if err != nil {
		t.Fatalf("fetchMetrics() error = %v", err)
	}
	if source != "orchestrator" {
		t.Fatalf("X-PinchTab-Source = %q, want orchestrator", source)
	}
}

type blockingWaitCmd struct {
	pid    int
	waitCh chan error
}

func (c *blockingWaitCmd) Wait() error { return <-c.waitCh }
func (c *blockingWaitCmd) PID() int    { return c.pid }
func (c *blockingWaitCmd) Cancel()     {}

func TestProbeChildInstanceReady_RequiresWarmTabSuccess(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/tab":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"error","error":"create target: context canceled"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", srv.URL, err)
	}

	ready, probe := o.probeChildInstanceReady(&InstanceInternal{Instance: bridge.Instance{ID: "inst_1"}}, baseURL)
	if ready {
		t.Fatal("expected warmup failure to keep instance unready")
	}
	if !strings.Contains(probe, "create target: context canceled") {
		t.Fatalf("probe = %q, want create-target failure", probe)
	}
}

func TestProbeChildInstanceReady_WarmsTabLifecycle(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	var closeCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/tab":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tabId":"tab-1","url":"about:blank","title":""}`))
		case "/close":
			atomic.AddInt32(&closeCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"closed":true,"tabId":"tab-1"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", srv.URL, err)
	}

	ready, probe := o.probeChildInstanceReady(&InstanceInternal{Instance: bridge.Instance{ID: "inst_1"}}, baseURL)
	if !ready {
		t.Fatalf("expected readiness success, got probe %q", probe)
	}
	if probe != "ready" {
		t.Fatalf("probe = %q, want ready", probe)
	}
	if atomic.LoadInt32(&closeCalls) != 1 {
		t.Fatalf("close calls = %d, want 1", closeCalls)
	}
}

func TestMonitor_DelaysRunningUntilWarmTabSucceeds(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	var tabAttempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/tab":
			attempt := atomic.AddInt32(&tabAttempts, 1)
			if attempt == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"error","error":"create target: context canceled"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tabId":"tab-2","url":"about:blank","title":""}`))
		case "/close":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"closed":true,"tabId":"tab-2"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", srv.URL, err)
	}
	host, portStr, ok := strings.Cut(parsed.Host, ":")
	if !ok {
		t.Fatalf("expected host:port in %q", parsed.Host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", portStr, err)
	}

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{Bind: host})
	o.client = srv.Client()

	cmd := &blockingWaitCmd{pid: 4321, waitCh: make(chan error, 1)}
	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:          "inst_1",
			Port:        strconv.Itoa(port),
			Status:      "starting",
			StartTime:   time.Now(),
			ProfileName: "profile1",
		},
		cmd: cmd,
	}

	done := make(chan struct{})
	go func() {
		o.monitor(inst)
		close(done)
	}()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		o.mu.RLock()
		status := inst.Status
		o.mu.RUnlock()
		if status == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	o.mu.RLock()
	status := inst.Status
	o.mu.RUnlock()
	if status != "running" {
		cmd.waitCh <- nil
		<-done
		t.Fatalf("status = %q, want running", status)
	}
	if atomic.LoadInt32(&tabAttempts) < 2 {
		cmd.waitCh <- nil
		<-done
		t.Fatalf("tab attempts = %d, want at least 2", tabAttempts)
	}

	cmd.waitCh <- nil
	<-done
}

// TestMonitor_ChildExitAfterRunningTransitionsToStopped covers the other half
// of monitor()'s lifecycle: once an instance has genuinely become healthy,
// the test-controlled child bridge exiting on its own (e.g. the external
// browser disconnected) must drive the instance to "stopped", not leave it
// stuck at "running" or misreport it as "error".
func TestMonitor_ChildExitAfterRunningTransitionsToStopped(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/tab":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tabId":"tab-1","url":"about:blank","title":""}`))
		case "/close":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"closed":true,"tabId":"tab-1"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", srv.URL, err)
	}
	host, portStr, ok := strings.Cut(parsed.Host, ":")
	if !ok {
		t.Fatalf("expected host:port in %q", parsed.Host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", portStr, err)
	}

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{Bind: host})
	o.client = srv.Client()

	cmd := &blockingWaitCmd{pid: 4321, waitCh: make(chan error, 1)}
	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:          "inst_1",
			Port:        strconv.Itoa(port),
			Status:      "starting",
			StartTime:   time.Now(),
			ProfileName: "profile1",
		},
		cmd: cmd,
	}

	done := make(chan struct{})
	go func() {
		o.monitor(inst)
		close(done)
	}()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		o.mu.RLock()
		status := inst.Status
		o.mu.RUnlock()
		if status == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	o.mu.RLock()
	status := inst.Status
	o.mu.RUnlock()
	if status != "running" {
		cmd.waitCh <- nil
		<-done
		t.Fatalf("status before child exit = %q, want running", status)
	}

	// Simulate the test-controlled child bridge exiting on its own now that
	// the instance is confirmed healthy.
	cmd.waitCh <- nil

	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("monitor did not return after child exit")
	}

	o.mu.RLock()
	status = inst.Status
	o.mu.RUnlock()
	if status != "stopped" {
		t.Fatalf("status after child exit = %q, want stopped", status)
	}
}

// compactBody cuts arbitrary remote bytes into an operator-facing error message. It used
// to slice at a raw byte offset, so any non-ASCII body landed mid-rune and the message
// carried U+FFFD where the server's own words should be. The fixture puts a multi-byte
// rune ACROSS the old cut offset on purpose: an ASCII body cannot tell the two
// implementations apart, which is how this survived a census.
func TestCompactBodyCutsOnARuneBoundary(t *testing.T) {
	// An all-euro body puts a rune boundary only every three bytes, so a naive cut lands
	// mid-rune unless the budget happens to be a multiple of three. Both cut points are
	// checked, because the marked form cuts at budget-minus-marker, not at the budget.
	const runeWidth = len("€")
	for _, cut := range []int{maxCompactBodyBytes, maxCompactBodyBytes - len(sanitize.TruncationSuffix)} {
		if cut%runeWidth == 0 {
			t.Fatalf("budget %d puts cut %d on a rune boundary, so this fixture cannot tell a naive cut from a rune-safe one; widen the fixture rune",
				maxCompactBodyBytes, cut)
		}
	}
	body := strings.Repeat("€", maxCompactBodyBytes)

	got := compactBody([]byte(body))

	if !utf8.ValidString(got) {
		t.Errorf("compactBody returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("compactBody returned U+FFFD, so the cut split a rune: %q", got)
	}
	if !strings.HasSuffix(got, sanitize.TruncationSuffix) {
		t.Errorf("compactBody returned %q without the truncation marker; an operator cannot tell a cut body from a short one", got)
	}
	// A rune-safe cut honours the budget as a CEILING, so bound both sides: over means
	// the marker is not counted inside the budget, far under means the cut fires early.
	if len(got) > maxCompactBodyBytes {
		t.Errorf("compactBody returned %d bytes, over the %d budget", len(got), maxCompactBodyBytes)
	}
	if len(got) < maxCompactBodyBytes-utf8.UTFMax {
		t.Errorf("compactBody returned only %d bytes for a %d budget; the cut fires too early", len(got), maxCompactBodyBytes)
	}
}

func TestCompactBodyLeavesShortAndEmptyBodiesAlone(t *testing.T) {
	short := "instance refused: profile is locked"
	if got := compactBody([]byte(short)); got != short {
		t.Errorf("compactBody(%q) = %q, want it unchanged", short, got)
	}
	if got := compactBody([]byte("  " + short + "\n")); got != short {
		t.Errorf("compactBody did not trim surrounding space: %q", got)
	}
	// Exactly at the budget is not truncation, so no marker.
	atCap := strings.Repeat("c", maxCompactBodyBytes)
	if got := compactBody([]byte(atCap)); got != atCap {
		t.Errorf("a body exactly at the budget was altered: %d bytes in, %d out", len(atCap), len(got))
	}
	for _, empty := range []string{"", "   ", "\n\t "} {
		if got := compactBody([]byte(empty)); got != "<empty>" {
			t.Errorf("compactBody(%q) = %q, want <empty>", empty, got)
		}
	}
}

// The three callers embed compactBody in an operator-facing error, and until now only
// the helper was driven — a correct helper says nothing about the sentence an operator
// reads, which is the artefact this card is about. Each caller is driven against an
// instance that answers its own route with a long non-ASCII body, and the assertion is
// on the ERROR STRING: valid UTF-8, no U+FFFD, and the marker present so a cut body is
// distinguishable from a short one.
func TestOperatorFacingErrorsCarryAValidUTF8BodyFragment(t *testing.T) {
	const runeWidth = len("€")
	// The fixture must straddle a rune at the cut or a byte slice and a rune-safe cut
	// agree and this test cannot tell them apart — the same trap the helper test guards.
	for _, cut := range []int{maxCompactBodyBytes, maxCompactBodyBytes - len(sanitize.TruncationSuffix)} {
		if cut%runeWidth == 0 {
			t.Fatalf("budget %d puts cut %d on a rune boundary; widen the fixture rune", maxCompactBodyBytes, cut)
		}
	}
	remoteMessage := strings.Repeat("€", maxCompactBodyBytes)

	failOn := func(path string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != path {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"tabId":"tab_warm"}`)),
					Header:     http.Header{},
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(remoteMessage)),
				Header:     http.Header{},
			}, nil
		})}
	}

	base, err := url.Parse("http://127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		path   string
		prefix string
		drive  func(*Orchestrator, *InstanceInternal) error
	}{
		{
			name:   "create tab",
			path:   "/tab",
			prefix: "create tab HTTP 500: ",
			drive: func(o *Orchestrator, inst *InstanceInternal) error {
				_, err := o.warmInstanceTabLifecycle(inst, base)
				return err
			},
		},
		{
			name:   "close warmup tab",
			path:   "/close",
			prefix: "close warmup tab HTTP 500: ",
			drive: func(o *Orchestrator, inst *InstanceInternal) error {
				_, err := o.warmInstanceTabLifecycle(inst, base)
				return err
			},
		},
		{
			name:   "ensure browser",
			path:   "/browser/ensure",
			prefix: "ensure-browser HTTP 500: ",
			drive: func(o *Orchestrator, inst *InstanceInternal) error {
				target := *base
				target.Path = "/browser/ensure"
				return o.ensureInstanceChrome(inst, &target)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
			o.client = failOn(tc.path)
			inst := &InstanceInternal{
				Instance: bridge.Instance{ID: "inst_utf80001", Port: "9999", Status: "running"},
				URL:      base.String(),
				logBuf:   newRingBuffer(1024),
			}

			err := tc.drive(o, inst)
			if err == nil {
				t.Fatalf("%s returned no error, so the refusal path was never reached", tc.name)
			}
			message := err.Error()
			if !strings.HasPrefix(message, tc.prefix) {
				t.Fatalf("message %q is not the %s refusal; the fixture drove the wrong path", message, tc.name)
			}
			if !utf8.ValidString(message) {
				t.Errorf("operator message is not valid UTF-8: %q", message)
			}
			if strings.ContainsRune(message, utf8.RuneError) {
				t.Errorf("operator message carries U+FFFD where the instance's own words should be: %q", message)
			}
			if !strings.HasSuffix(message, sanitize.TruncationSuffix) {
				t.Errorf("operator message %q does not end in the truncation marker, so a cut body reads as a complete one", message)
			}
		})
	}
}
