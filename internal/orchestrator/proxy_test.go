package orchestrator

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func startLocalHTTPServer(t *testing.T, h http.Handler) (*httptest.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(h)
	server.Listener = ln
	server.Start()
	return server, fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
}

func TestProxyTabRequest_FallsBackToOnlyRunningInstance(t *testing.T) {
	backend, port := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer backend.Close()

	o := NewOrchestrator(t.TempDir())
	o.client = backend.Client()
	o.instances["inst_1"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_1", Status: "running", Port: port},
		URL:      "http://localhost:" + port,
		cmd:      &mockCmd{pid: 1234, isAlive: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/tabs/ABC123/snapshot", nil)
	req.SetPathValue("id", "ABC123")
	w := httptest.NewRecorder()

	orig := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = orig }()

	o.proxyTabRequest(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSingleRunningInstance_MultipleInstancesReturnsNil(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.instances["inst_1"] = &InstanceInternal{Instance: bridge.Instance{ID: "inst_1", Status: "running"}, cmd: &mockCmd{pid: 1, isAlive: true}}
	o.instances["inst_2"] = &InstanceInternal{Instance: bridge.Instance{ID: "inst_2", Status: "running"}, cmd: &mockCmd{pid: 2, isAlive: true}}

	orig := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = orig }()

	if got := o.singleRunningInstance(); got != nil {
		t.Fatalf("expected nil, got %v", got.ID)
	}
}

func TestSingleRunningInstance_IgnoresStopped(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.instances["inst_1"] = &InstanceInternal{Instance: bridge.Instance{ID: "inst_1", Status: "running"}, cmd: &mockCmd{pid: 1, isAlive: true}}
	o.instances["inst_2"] = &InstanceInternal{Instance: bridge.Instance{ID: "inst_2", Status: "stopped"}}

	orig := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid == 1 }
	defer func() { processAliveFunc = orig }()

	got := o.singleRunningInstance()
	if got == nil || got.ID != "inst_1" {
		t.Fatalf("got %#v, want inst_1", got)
	}
}

func TestProxyToURL_UsesAttachedBridgeOriginAndAuth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer bridge-token" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer bridge-token")
		}
		if r.URL.Path != "/tabs/tab-1/snapshot" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `ok`)
	}))
	defer backend.Close()

	o := NewOrchestrator(t.TempDir())
	o.client = backend.Client()
	attached, _, err := o.AttachBridge("bridge1", backend.URL, "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tabs/tab-1/snapshot", nil)
	req.SetPathValue("id", "tab-1")
	w := httptest.NewRecorder()

	targetURL, err := o.instancePathURLFromBridge(attached, "/tabs/tab-1/snapshot", "")
	if err != nil {
		t.Fatalf("instancePathURLFromBridge failed: %v", err)
	}
	o.proxyToURL(w, req, targetURL)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestProxyToTarget_InjectsChildAuthAndStripsCookie(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer child-token" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer child-token")
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatalf("cookie = %q, want empty", got)
		}
		if r.URL.Path != "/action" {
			t.Fatalf("path = %q, want /action", r.URL.Path)
		}
		_, _ = io.WriteString(w, `ok`)
	}))
	defer backend.Close()

	o := NewOrchestrator(t.TempDir())
	o.client = backend.Client()
	o.childAuthToken = "child-token"
	o.instances["inst_1"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_1", Status: "running"},
		URL:      backend.URL,
		cmd:      &mockCmd{pid: 1234, isAlive: true},
	}

	req := httptest.NewRequest(http.MethodPost, "/action", nil)
	req.Header.Set("Cookie", "pinchtab_auth_token=session-secret")
	w := httptest.NewRecorder()

	o.ProxyToTarget(w, req, backend.URL+"/action")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
