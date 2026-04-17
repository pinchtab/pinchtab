package orchestrator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func envMap(items []string) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func stubPortAvailability(t *testing.T, fn func(int) bool) {
	t.Helper()
	old := portAvailableFunc
	portAvailableFunc = fn
	t.Cleanup(func() {
		portAvailableFunc = old
	})
}

func TestOrchestrator_Launch_Lifecycle(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.Launch("profile1", "9001", true, nil)
	if err != nil {
		t.Fatalf("First launch failed: %v", err)
	}
	if inst.Mode != "headless" {
		t.Fatalf("Mode = %q, want %q", inst.Mode, "headless")
	}
	if inst.Status != "starting" {
		t.Errorf("expected status starting, got %s", inst.Status)
	}

	_, err = o.Launch("profile1", "9002", true, nil)
	if err == nil {
		t.Error("expected error when launching duplicate profile")
	}

	runner.portAvail = false
	_, err = o.Launch("profile2", "9001", true, nil)
	if err == nil {
		t.Error("expected error when launching on occupied port")
	}
}

func TestOrchestrator_ListAndStop(t *testing.T) {
	alive := true
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return alive }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, _ := o.Launch("p1", "9001", true, nil)

	if len(o.List()) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(o.List()))
	}

	alive = false
	err := o.Stop(inst.ID)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	instances := o.List()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}
}

func TestOrchestrator_Launch_UsesConfiguredBindInInstanceURL(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{Bind: "192.168.1.50"})

	inst, err := o.Launch("profile1", "9001", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if inst.URL != "http://192.168.1.50:9001" {
		t.Fatalf("URL = %q, want %q", inst.URL, "http://192.168.1.50:9001")
	}
}

func TestOrchestrator_StopProfile(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	o.mu.Lock()
	instID := o.idMgr.InstanceID(o.idMgr.ProfileID("p1"), "p1")
	o.instances[instID] = &InstanceInternal{
		Instance: bridge.Instance{
			ID:          instID,
			ProfileID:   o.idMgr.ProfileID("p1"),
			ProfileName: "p1",
			Port:        "9001",
			Status:      "running",
		},
		URL: "http://localhost:9001",
	}
	o.mu.Unlock()

	processAliveFunc = func(pid int) bool { return false }

	err := o.StopProfile("p1")
	if err != nil {
		t.Fatalf("StopProfile failed: %v", err)
	}

	instances := o.List()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances after stop, got %d", len(instances))
	}
}

// === Security Validation Tests ===

func TestOrchestrator_Launch_RejectsPathTraversal(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	badNames := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"double dot prefix", "../malicious", "cannot contain '..'"},
		{"double dot suffix", "test/..", "cannot contain '..'"},
		{"double dot middle", "test/../other", "cannot contain '..'"},
		{"forward slash", "test/nested", "cannot contain '/'"},
		{"backslash", "test\\nested", "cannot contain '/'"},
		{"empty name", "", "cannot be empty"},
		{"absolute path attempt", "../../../etc/passwd", "cannot contain"},
		{"powershell metacharacter", "poc';calc", "contains invalid character"},
		{"reserved windows device name", "CON", "reserved device name"},
	}

	for _, tt := range badNames {
		t.Run(tt.name, func(t *testing.T) {
			_, err := o.Launch(tt.input, "9999", true, nil)
			if err == nil {
				t.Errorf("Launch(%q) should have returned error", tt.input)
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("Launch(%q) error = %q, want containing %q", tt.input, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestOrchestrator_Launch_AcceptsValidNames(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	validNames := []string{
		"simple",
		"with-dash",
		"with_underscore",
		"with.dot",
		"Work Profile",
		"CamelCase",
		"123numeric",
		"a",
	}

	for i, name := range validNames {
		t.Run(name, func(t *testing.T) {
			port := 9100 + i
			inst, err := o.Launch(name, strconv.Itoa(port), true, nil)
			if err != nil {
				t.Errorf("Launch(%q) unexpected error: %v", name, err)
				return
			}
			if inst.ProfileName != name {
				t.Errorf("Launch(%q) profileName = %q", name, inst.ProfileName)
			}
		})
	}
}

func TestOrchestrator_Launch_RejectsInvalidPort(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	for _, raw := range []string{"abc", "0x1234", "65536"} {
		if _, err := o.Launch("profile1", raw, true, nil); err == nil {
			t.Fatalf("Launch should reject invalid port %q", raw)
		}
	}
}

func TestOrchestrator_Launch_ReservesDistinctChromeDebugPort(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		Token:             "child-token",
		InstancePortStart: 9900,
		InstancePortEnd:   9903,
	})

	inst, err := o.Launch("profile1", "", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	if inst.Port != "9900" {
		t.Fatalf("bridge port = %s, want 9900", inst.Port)
	}

	cfgPath := envMap(runner.env)["PINCHTAB_CONFIG"]
	if cfgPath == "" {
		t.Fatal("PINCHTAB_CONFIG missing from child env")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", cfgPath, err)
	}

	var fc config.FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("Unmarshal child config error = %v", err)
	}
	if fc.Browser.ChromeDebugPort == nil {
		t.Fatal("child config missing browser.remoteDebuggingPort")
	}
	if *fc.Browser.ChromeDebugPort != 9901 {
		t.Fatalf("chrome debug port = %d, want 9901", *fc.Browser.ChromeDebugPort)
	}
	if *fc.Browser.ChromeDebugPort == 9900 {
		t.Fatal("chrome debug port should differ from bridge port")
	}

	gotPorts := o.portAllocator.AllocatedPorts()
	if len(gotPorts) != 2 {
		t.Fatalf("allocated ports = %v, want 2 reserved ports", gotPorts)
	}
	if !o.portAllocator.IsAllocated(9900) || !o.portAllocator.IsAllocated(9901) {
		t.Fatalf("expected ports 9900 and 9901 reserved, got %v", gotPorts)
	}
}

func TestOrchestrator_Launch_ExplicitPortAlsoReservesDistinctChromeDebugPort(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		InstancePortStart: 9910,
		InstancePortEnd:   9913,
	})

	inst, err := o.Launch("profile1", "9911", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}
	if inst.Port != "9911" {
		t.Fatalf("bridge port = %s, want 9911", inst.Port)
	}

	cfgPath := envMap(runner.env)["PINCHTAB_CONFIG"]
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", cfgPath, err)
	}

	var fc config.FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("Unmarshal child config error = %v", err)
	}
	if fc.Browser.ChromeDebugPort == nil {
		t.Fatal("child config missing browser.remoteDebuggingPort")
	}
	if *fc.Browser.ChromeDebugPort == 9911 {
		t.Fatalf("chrome debug port = %d, must differ from bridge port", *fc.Browser.ChromeDebugPort)
	}
	if !o.portAllocator.IsAllocated(9911) {
		t.Fatal("explicit bridge port should remain reserved in allocator while instance is active")
	}
}

func TestOrchestrator_Launch_DoesNotInjectSharedActivityStateDir(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	root := t.TempDir()
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(root, runner)
	sharedActivityStateDir := filepath.Join(root, "dashboard-state")
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		StateDir:          sharedActivityStateDir,
		InstancePortStart: 9930,
		InstancePortEnd:   9933,
	})

	inst, err := o.Launch("profile1", "", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	cfgPath := envMap(runner.env)["PINCHTAB_CONFIG"]
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", cfgPath, err)
	}

	var fc config.FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("Unmarshal child config error = %v", err)
	}

	wantChildStateDir := filepath.Join(root, "profile1", ".pinchtab-state")
	if fc.Server.StateDir != wantChildStateDir {
		t.Fatalf("child Server.StateDir = %q, want %q", fc.Server.StateDir, wantChildStateDir)
	}
	if fc.Observability.Activity.StateDir != "" {
		t.Fatalf("child Observability.Activity.StateDir = %q, want empty", fc.Observability.Activity.StateDir)
	}
	if fc.Observability.Activity.Enabled == nil || *fc.Observability.Activity.Enabled {
		t.Fatalf("child Observability.Activity.Enabled = %v, want explicit false", fc.Observability.Activity.Enabled)
	}
	if got := envMap(runner.env)["PINCHTAB_INTERNAL_ACTIVITY_STATE_DIR"]; got != "" {
		t.Fatalf("PINCHTAB_INTERNAL_ACTIVITY_STATE_DIR = %q, want empty", got)
	}
	if inst.Port != "9930" {
		t.Fatalf("bridge port = %s, want 9930", inst.Port)
	}
}

func TestOrchestrator_Stop_ReleasesBridgeAndChromeDebugPorts(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return false }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		InstancePortStart: 9920,
		InstancePortEnd:   9923,
	})

	inst, err := o.Launch("profile1", "", true, nil)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	if err := o.Stop(inst.ID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if got := o.portAllocator.AllocatedPorts(); len(got) != 0 {
		t.Fatalf("allocated ports after stop = %v, want none", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestOrchestrator_Attach(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	cdpURL := "ws://localhost:9222/devtools/browser/abc123"
	inst, err := o.Attach("my-external-chrome", cdpURL)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	if !inst.Attached {
		t.Error("expected Attached to be true")
	}
	if inst.CdpURL != cdpURL {
		t.Errorf("expected CdpURL %q, got %q", cdpURL, inst.CdpURL)
	}
	if inst.Status != "running" {
		t.Errorf("expected status running, got %s", inst.Status)
	}
	if inst.ProfileName != "my-external-chrome" {
		t.Errorf("expected ProfileName %q, got %q", "my-external-chrome", inst.ProfileName)
	}

	// Check it appears in list
	list := o.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}
	if !list[0].Attached {
		t.Error("instance in list should have Attached=true")
	}
}

func TestOrchestrator_Attach_DuplicateName(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	_, err := o.Attach("chrome1", "ws://localhost:9222/a")
	if err != nil {
		t.Fatalf("First attach failed: %v", err)
	}

	_, err = o.Attach("chrome1", "ws://localhost:9222/b")
	if err == nil {
		t.Error("expected error when attaching duplicate name")
	}
}

func TestOrchestrator_AttachBridge(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, created, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868", "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}
	if !created {
		t.Fatal("expected new bridge to be created")
	}
	if !inst.Attached {
		t.Fatal("expected attached instance")
	}
	if inst.AttachType != "bridge" {
		t.Fatalf("AttachType = %q, want %q", inst.AttachType, "bridge")
	}
	if inst.URL != "http://10.0.0.8:9868" {
		t.Fatalf("URL = %q, want %q", inst.URL, "http://10.0.0.8:9868")
	}
	if inst.CdpURL != "" {
		t.Fatalf("CdpURL = %q, want empty", inst.CdpURL)
	}
}

func TestOrchestrator_AttachBridge_UpsertsExistingBridge(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	first, _, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868", "bridge-token-1")
	if err != nil {
		t.Fatalf("first AttachBridge failed: %v", err)
	}

	// Same token → upsert succeeds
	second, created, err := o.AttachBridge("bridge1", "http://10.0.0.9:9868", "bridge-token-1")
	if err != nil {
		t.Fatalf("second AttachBridge failed: %v", err)
	}
	if created {
		t.Fatal("expected upsert, not create")
	}

	if second.ID != first.ID {
		t.Fatalf("ID = %q, want %q", second.ID, first.ID)
	}
	if second.URL != "http://10.0.0.9:9868" {
		t.Fatalf("URL = %q, want %q", second.URL, "http://10.0.0.9:9868")
	}

	o.mu.RLock()
	internal := o.instances[first.ID]
	o.mu.RUnlock()
	if internal == nil {
		t.Fatalf("attached instance %q missing from orchestrator", first.ID)
		return
	}
	if internal.authToken != "bridge-token-1" {
		t.Fatalf("authToken = %q, want %q", internal.authToken, "bridge-token-1")
	}

	list := o.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}
}

func TestOrchestrator_AttachBridge_RejectsTokenMismatch(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	_, _, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868", "bridge-token-1")
	if err != nil {
		t.Fatalf("first AttachBridge failed: %v", err)
	}

	// Different token → rejected
	_, _, err = o.AttachBridge("bridge1", "http://10.0.0.9:9868", "bridge-token-2")
	if err == nil {
		t.Fatal("expected error for token mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "token mismatch") {
		t.Fatalf("expected token mismatch error, got: %v", err)
	}
}

func TestOrchestrator_AttachBridge_RemovesUnhealthyBridge(t *testing.T) {
	unhealthy := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path = %q, want /health", r.URL.Path)
		}
		if unhealthy {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = backend.Client()
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{backendURL.Hostname()},
		AttachAllowSchemes: []string{"http"},
	})

	inst, _, err := o.AttachBridge("bridge1", backend.URL, "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}

	unhealthy = true

	o.mu.RLock()
	internal := o.instances[inst.ID]
	o.mu.RUnlock()
	if internal == nil {
		t.Fatalf("attached instance %q missing from orchestrator", inst.ID)
	}

	if o.checkAttachedBridgeHealth(internal) {
		t.Fatal("expected unhealthy attached bridge to stop monitoring")
	}
	if len(o.List()) != 0 {
		t.Fatalf("expected attached bridge to be removed, got %d instances", len(o.List()))
	}
}

func TestValidateAttachURL_AllowsBridgeHTTP(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http", "ws"},
	})

	if err := o.validateAttachURL("http://10.0.0.8:9868"); err != nil {
		t.Fatalf("validateAttachURL returned error: %v", err)
	}
}

func TestValidateAttachURL_RejectsBridgeBaseURLWithPath(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	err := o.validateAttachURL("http://10.0.0.8:9868/api")
	if err == nil {
		t.Fatal("expected error for attach bridge URL with path")
	}
	if !strings.Contains(err.Error(), "must not include a path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAttachURL_RejectsBridgeBaseURLWithUserinfo(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	err := o.validateAttachURL("http://user:pass@10.0.0.8:9868")
	if err == nil {
		t.Fatal("expected error for attach bridge URL with userinfo")
	}
	if !strings.Contains(err.Error(), "must not include userinfo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAttachURL_RejectsBridgeBaseURLWithQueryOrFragment(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"10.0.0.8"},
		AttachAllowSchemes: []string{"http"},
	})

	for _, raw := range []string{
		"http://10.0.0.8:9868?token=secret",
		"http://10.0.0.8:9868#debug",
	} {
		err := o.validateAttachURL(raw)
		if err == nil {
			t.Fatalf("expected error for attach bridge URL %q", raw)
		}
		if !strings.Contains(err.Error(), "must not include query or fragment") {
			t.Fatalf("unexpected error for %q: %v", raw, err)
		}
	}
}

func TestOrchestrator_AttachBridge_NormalizesBaseURL(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, _, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868/?debug=1#frag", "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}
	if inst.URL != "http://10.0.0.8:9868" {
		t.Fatalf("URL = %q, want %q", inst.URL, "http://10.0.0.8:9868")
	}
}

func TestValidateAttachURL_WildcardHost(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"*"},
		AttachAllowSchemes: []string{"http", "ws"},
	})

	if err := o.validateAttachURL("http://192.168.1.100:9868"); err != nil {
		t.Fatalf("wildcard host should allow any host, got: %v", err)
	}
	if err := o.validateAttachURL("http://bridge-container:9868"); err != nil {
		t.Fatalf("wildcard host should allow hostname, got: %v", err)
	}
}

func TestOrchestrator_RegisterHandlers_CacheRoutes(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{})

	mux := http.NewServeMux()
	o.RegisterHandlers(mux)

	routes := []struct {
		method string
		path   string
		route  string
	}{
		{"POST", "/instances/inst1/cache/clear", "POST /instances/{id}/cache/clear"},
		{"GET", "/instances/inst1/cache/status", "GET /instances/{id}/cache/status"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		_, pattern := mux.Handler(req)
		if pattern != rt.route {
			t.Errorf("expected route %q for %s %s, got %q", rt.route, rt.method, rt.path, pattern)
		}
	}
}

func TestOrchestrator_RegisterHandlers_LocksSensitiveRoutes(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{})

	mux := http.NewServeMux()
	o.RegisterHandlers(mux)

	tests := []struct {
		method  string
		path    string
		body    string
		setting string
	}{
		{method: "POST", path: "/tabs/tab1/evaluate", body: `{"expression":"1+1"}`, setting: "security.allowEvaluate"},
		{method: "GET", path: "/tabs/tab1/download", setting: "security.allowDownload"},
		{method: "POST", path: "/tabs/tab1/upload", body: `{}`, setting: "security.allowUpload"},
		{method: "GET", path: "/instances/inst1/screencast", setting: "security.allowScreencast"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("%s %s expected 403, got %d", tt.method, tt.path, w.Code)
		}
		if !strings.Contains(w.Body.String(), tt.setting) {
			t.Fatalf("%s %s expected setting %s in response, got %s", tt.method, tt.path, tt.setting, w.Body.String())
		}
	}
}
