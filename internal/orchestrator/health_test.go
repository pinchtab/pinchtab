package orchestrator

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
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
				Body:       io.NopCloser(strings.NewReader(`{"memory":{"jsHeapUsedMB":1}}`)),
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
