package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
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

func allowAttachForTest(o *Orchestrator, schemes, hosts []string) {
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowSchemes: schemes,
		AttachAllowHosts:   hosts,
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

func TestOrchestrator_Launch_ReservesDistinctBrowserDebugPort(t *testing.T) {
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
	if fc.Browser.BrowserDebugPort == nil {
		t.Fatal("child config missing browser.remoteDebuggingPort")
	}
	if *fc.Browser.BrowserDebugPort != 9901 {
		t.Fatalf("chrome debug port = %d, want 9901", *fc.Browser.BrowserDebugPort)
	}
	if *fc.Browser.BrowserDebugPort == 9900 {
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

func TestOrchestrator_Launch_ExplicitPortAlsoReservesDistinctBrowserDebugPort(t *testing.T) {
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
	if fc.Browser.BrowserDebugPort == nil {
		t.Fatal("child config missing browser.remoteDebuggingPort")
	}
	if *fc.Browser.BrowserDebugPort == 9911 {
		t.Fatalf("chrome debug port = %d, must differ from bridge port", *fc.Browser.BrowserDebugPort)
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

func TestOrchestrator_Stop_ReleasesBridgeAndBrowserDebugPorts(t *testing.T) {
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

func stubAttachBridgeHealthy(t *testing.T) {
	t.Helper()
	old := waitForChildBridgeHealthyFunc
	waitForChildBridgeHealthyFunc = func(o *Orchestrator, inst *InstanceInternal, timeout time.Duration) error {
		o.mu.Lock()
		inst.Status = "running"
		o.mu.Unlock()
		return nil
	}
	t.Cleanup(func() { waitForChildBridgeHealthyFunc = old })
}

func TestOrchestrator_Attach(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"localhost"})
	o.attachHealthCheckTimeout = 50 * time.Millisecond

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
	if inst.AttachType != "cdp-bridge" {
		t.Errorf("expected AttachType cdp-bridge, got %q", inst.AttachType)
	}
	if !strings.HasPrefix(inst.URL, "http://") {
		t.Errorf("expected http:// bridge URL, got %q", inst.URL)
	}
	if inst.URL == cdpURL {
		t.Errorf("URL must not be the raw cdpUrl")
	}
	if inst.ProfileName != "my-external-chrome" {
		t.Errorf("expected ProfileName %q, got %q", "my-external-chrome", inst.ProfileName)
	}

	list := o.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}
	if !list[0].Attached {
		t.Error("instance in list should have Attached=true")
	}
}

func TestOrchestrator_Attach_SpawnsChildBridgeWithCDPFlags(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"127.0.0.1"})
	o.attachHealthCheckTimeout = 30 * time.Millisecond

	cdpURL := "ws://127.0.0.1:9222/devtools/browser/abc"
	if _, err := o.AttachWithProvider("ext-cloak", cdpURL, "cloak"); err != nil {
		t.Fatalf("AttachWithProvider failed: %v", err)
	}
	if !runner.runCalled {
		t.Fatal("expected runner.Run to be called for child bridge")
	}
	wantArgs := []string{"bridge", "--cdp-attach", cdpURL, "--browser", "cloak", "--remote-browser-name", "ext-cloak"}
	if !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("child args = %v, want supported bridge command surface %v", runner.args, wantArgs)
	}
}

func TestOrchestrator_AttachWithProviderRejectsUnknownProvider(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"127.0.0.1"})

	_, err := o.AttachWithProvider("ext-cloak", "ws://127.0.0.1:9222/devtools/browser/abc", "cloack")
	if err == nil {
		t.Fatal("expected unknown browser error")
	}
	if !strings.Contains(err.Error(), "unknown browser") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.runCalled {
		t.Fatal("invalid provider should be rejected before starting child bridge")
	}
}

func TestOrchestrator_Attach_PreservesCdpURLMetadata(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"127.0.0.1"})
	o.attachHealthCheckTimeout = 30 * time.Millisecond

	cdpURL := "ws://127.0.0.1:9222/devtools/browser/xyz"
	inst, err := o.Attach("ext", cdpURL)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	if inst.CdpURL != cdpURL {
		t.Fatalf("CdpURL not preserved: got %q want %q", inst.CdpURL, cdpURL)
	}
	if inst.URL == cdpURL {
		t.Fatal("instance URL must be the HTTP bridge URL, not the raw cdpUrl")
	}
	if !strings.HasPrefix(inst.URL, "http://") {
		t.Fatalf("expected http:// bridge URL, got %q", inst.URL)
	}
}

func TestOrchestrator_Attach_DuplicateName(t *testing.T) {
	stubAttachBridgeHealthy(t)
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"localhost"})
	o.attachHealthCheckTimeout = 50 * time.Millisecond

	_, err := o.Attach("chrome1", "ws://localhost:9222/devtools/browser/a")
	if err != nil {
		t.Fatalf("First attach failed: %v", err)
	}

	_, err = o.Attach("chrome1", "ws://localhost:9222/devtools/browser/b")
	if err == nil {
		t.Error("expected error when attaching duplicate name")
	}
}

func TestOrchestrator_Attach_HealthFailureTearsDownChild(t *testing.T) {
	oldHealth := waitForChildBridgeHealthyFunc
	waitForChildBridgeHealthyFunc = func(o *Orchestrator, inst *InstanceInternal, timeout time.Duration) error {
		return fmt.Errorf("boom")
	}
	defer func() { waitForChildBridgeHealthyFunc = oldHealth }()

	oldAlive := processAliveFunc
	processAliveFunc = func(pid int) bool { return false }
	defer func() { processAliveFunc = oldAlive }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"localhost"})
	o.attachHealthCheckTimeout = 30 * time.Millisecond

	_, err := o.Attach("unhealthy", "ws://localhost:9222/devtools/browser/a")
	if err == nil {
		t.Fatal("expected attach to fail when child bridge health fails")
	}
	if !strings.Contains(err.Error(), "did not become healthy") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := o.List(); len(got) != 0 {
		t.Fatalf("unhealthy attach child was not removed: %+v", got)
	}
}

func TestOrchestrator_FirstRunningURLForRequest_UsesBrowserTarget(t *testing.T) {
	oldAlive := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = oldAlive }()

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	now := time.Now()
	o.instances["chrome"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "chrome", URL: "http://chrome.local", Status: "running", Browser: config.BrowserChrome, StartTime: now},
		URL:      "http://chrome.local",
		cmd:      &mockCmd{pid: 111, isAlive: true},
	}
	o.instances["cloak"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "cloak", URL: "http://cloak.local", Status: "running", Browser: config.BrowserCloak, StartTime: now.Add(time.Second)},
		URL:      "http://cloak.local",
		cmd:      &mockCmd{pid: 222, isAlive: true},
	}

	req := httptest.NewRequest(http.MethodPost, "/navigate?browser=cloak", nil)

	url, status, err := o.FirstRunningURLForRequest(req)
	if err != nil {
		t.Fatalf("FirstRunningURLForRequest error status=%d err=%v", status, err)
	}
	if url != "http://cloak.local" {
		t.Fatalf("url = %q, want cloak target URL", url)
	}
}

func TestOrchestrator_FirstRunningURLForRequest_UsesDefaultTargetWhenOmitted(t *testing.T) {
	oldAlive := processAliveFunc
	processAliveFunc = func(pid int) bool { return true }
	defer func() { processAliveFunc = oldAlive }()

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "cloak",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	now := time.Now()
	o.instances["chrome"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "chrome", URL: "http://chrome.local", Status: "running", Browser: config.BrowserChrome, StartTime: now},
		URL:      "http://chrome.local",
		cmd:      &mockCmd{pid: 111, isAlive: true},
	}
	o.instances["cloak"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "cloak", URL: "http://cloak.local", Status: "running", Browser: config.BrowserCloak, StartTime: now.Add(time.Second)},
		URL:      "http://cloak.local",
		cmd:      &mockCmd{pid: 222, isAlive: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/text", nil)
	url, status, err := o.FirstRunningURLForRequest(req)
	if err != nil {
		t.Fatalf("FirstRunningURLForRequest error status=%d err=%v", status, err)
	}
	if url != "http://cloak.local" {
		t.Fatalf("url = %q, want default target URL", url)
	}
}

func TestOrchestrator_FirstRunningURLForRequest_RejectsUnknownBrowser(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "cloak-1",
		Targets: config.BrowserTargetsConfig{
			"cloak-1": {Provider: config.BrowserCloak},
		},
	})
	// "ghost" normalizes to "chrome" via NormalizeBrowser, but no target
	// has provider "chrome" in this config → no match → 400.
	req := httptest.NewRequest(http.MethodGet, "/navigate?browser=ghost", nil)

	_, status, err := o.FirstRunningURLForRequest(req)
	if err == nil {
		t.Fatal("expected unknown browser error")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestOrchestrator_FirstRunningURLForRequest_RejectsUnknownBrowserNoTargets(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	// No targets configured — the no-targets fallback path must still reject unknowns.
	req := httptest.NewRequest(http.MethodGet, "/navigate?browser=chrme", nil)

	_, status, err := o.FirstRunningURLForRequest(req)
	if err == nil {
		t.Fatal("expected unknown browser error for typo without targets")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestOrchestrator_FirstRunningURLForRequest_RequestedTargetNotRunningConflicts(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/navigate?browser=cloak", nil)

	_, status, err := o.FirstRunningURLForRequest(req)
	if err == nil {
		t.Fatal("expected no-running browser error")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
}

func TestOrchestrator_AttachBridge(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"http"}, []string{"10.0.0.8"})

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
	allowAttachForTest(o, []string{"http"}, []string{"10.0.0.8", "10.0.0.9"})

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

func TestOrchestrator_AttachBridgeWithOptions_UpsertUpdatesBrowserTarget(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowSchemes: []string{"http"},
		AttachAllowHosts:   []string{"10.0.0.8", "10.0.0.9"},
		DefaultTarget:      "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})

	first, _, err := o.AttachBridgeWithOptions("bridge1", "http://10.0.0.8:9868", "bridge-token", AttachOptions{Browser: config.BrowserChrome})
	if err != nil {
		t.Fatalf("first AttachBridgeWithOptions failed: %v", err)
	}
	if first.Browser != config.BrowserChrome {
		t.Fatalf("first browser = %q, want chrome", first.Browser)
	}

	second, created, err := o.AttachBridgeWithOptions("bridge1", "http://10.0.0.9:9868", "bridge-token", AttachOptions{Browser: config.BrowserCloak})
	if err != nil {
		t.Fatalf("second AttachBridgeWithOptions failed: %v", err)
	}
	if created {
		t.Fatal("expected upsert, not create")
	}
	if second.ID != first.ID {
		t.Fatalf("ID = %q, want %q", second.ID, first.ID)
	}
	if second.Browser != config.BrowserCloak {
		t.Fatalf("second browser = %q, want cloak", second.Browser)
	}

	list := o.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}
	if list[0].Browser != config.BrowserCloak {
		t.Fatalf("listed browser = %q, want cloak", list[0].Browser)
	}
}

func TestOrchestrator_AttachBridge_UpsertWithoutTargetPreservesExistingBrowserTarget(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowSchemes: []string{"http"},
		AttachAllowHosts:   []string{"10.0.0.8", "10.0.0.9"},
		DefaultTarget:      "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})

	first, _, err := o.AttachBridgeWithOptions("bridge1", "http://10.0.0.8:9868", "bridge-token", AttachOptions{Browser: config.BrowserCloak})
	if err != nil {
		t.Fatalf("first AttachBridgeWithOptions failed: %v", err)
	}
	second, created, err := o.AttachBridge("bridge1", "http://10.0.0.9:9868", "bridge-token")
	if err != nil {
		t.Fatalf("second AttachBridge failed: %v", err)
	}
	if created {
		t.Fatal("expected upsert, not create")
	}
	if second.ID != first.ID {
		t.Fatalf("ID = %q, want %q", second.ID, first.ID)
	}
	if second.Browser != config.BrowserCloak {
		t.Fatalf("Browser = %q, want preserved cloak", second.Browser)
	}
}

func TestOrchestrator_AttachBridge_RejectsTokenMismatch(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"http"}, []string{"10.0.0.8", "10.0.0.9"})

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
	if !strings.Contains(err.Error(), "bare origin or end with /json/version") {
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
	if !strings.Contains(err.Error(), "userinfo") {
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
		if !strings.Contains(err.Error(), "query or fragment") {
			t.Fatalf("unexpected error for %q: %v", raw, err)
		}
	}
}

func TestOrchestrator_AttachBridge_NormalizesBaseURL(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"http"}, []string{"10.0.0.8"})

	inst, _, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868/", "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
	}
	if inst.URL != "http://10.0.0.8:9868" {
		t.Fatalf("URL = %q, want %q", inst.URL, "http://10.0.0.8:9868")
	}
}

func TestValidateAttachURL_AttachDisabled(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      false,
		AttachAllowHosts:   []string{"*"},
		AttachAllowSchemes: []string{"ws"},
	})
	err := o.validateAttachURL("ws://127.0.0.1:9222/devtools/browser/x")
	if err == nil {
		t.Fatal("expected error when attach is disabled")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAttachURL_DisallowedHost(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"127.0.0.1"},
		AttachAllowSchemes: []string{"ws"},
	})
	if err := o.validateAttachURL("ws://evil.example/devtools/browser/x"); err == nil {
		t.Fatal("expected error for disallowed host")
	}
}

func TestValidateAttachURL_DisallowedScheme(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"*"},
		AttachAllowSchemes: []string{"ws"},
	})
	if err := o.validateAttachURL("http://127.0.0.1:9222"); err == nil {
		t.Fatal("expected error for disallowed scheme")
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

func TestOrchestrator_LaunchWithOptions_BrowserFieldSet(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.LaunchWithOptions("browser-test", "9050", true, LaunchOptions{
		Browser: "cloak",
	})
	if err != nil {
		t.Fatalf("Launch with Browser=cloak failed: %v", err)
	}
	if inst.Browser != "cloak" {
		t.Fatalf("Browser = %q, want %q", inst.Browser, "cloak")
	}
}

func TestOrchestrator_LaunchWithOptions_BrowserFieldEmpty(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.LaunchWithOptions("browser-empty", "9051", true, LaunchOptions{})
	if err != nil {
		t.Fatalf("Launch with empty Browser failed: %v", err)
	}
	if inst.Browser != "" {
		t.Fatalf("Browser = %q, want empty", inst.Browser)
	}
}

func TestOrchestrator_LaunchWithOptions_BrowserFieldInvalid(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	_, err := o.LaunchWithOptions("browser-invalid", "9052", true, LaunchOptions{
		Browser: "nonexistent-browser",
	})
	if err == nil {
		t.Fatal("expected error for invalid browser, got nil")
	}
	if !strings.Contains(err.Error(), "invalid browser") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if runner.runCalled {
		t.Fatal("runner should not have been called for invalid browser")
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
		{method: "GET", path: "/tabs/tab1/cookies", setting: "security.allowCookies"},
		{method: "DELETE", path: "/tabs/tab1/cookies", setting: "security.allowCookies"},
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

// With two targets sharing the chrome provider, an explicit TargetName must
// promote that exact target's config into the child config — provider-based
// re-derivation would pick the default target's binary.
func TestLaunchWithOptions_TargetNamePromotesExactTarget(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		InstancePortStart: 9900,
		InstancePortEnd:   9910,
		DefaultTarget:     "chrome-local",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserChrome, Binary: "/usr/bin/chrome-a"},
			"backup":       {Provider: config.BrowserChrome, Binary: "/usr/bin/chrome-b"},
		},
	})

	if _, err := o.LaunchWithOptions("p1", "", true, LaunchOptions{
		Browser:    config.BrowserChrome,
		TargetName: "backup",
	}); err != nil {
		t.Fatalf("LaunchWithOptions: %v", err)
	}

	cfgPath := envMap(runner.env)["PINCHTAB_CONFIG"]
	if cfgPath == "" {
		t.Fatal("PINCHTAB_CONFIG missing from child env")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", cfgPath, err)
	}
	var fc config.FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("unmarshal child config: %v", err)
	}
	if fc.Browser.BrowserBinary != "/usr/bin/chrome-b" {
		t.Fatalf("child binary = %q, want backup's /usr/bin/chrome-b", fc.Browser.BrowserBinary)
	}
}

// M9 regression: a failed attach spawn must release the name reservation so
// a retry with the same name succeeds.
func TestAttach_FailedSpawnReleasesNameReservation(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true, runErr: errors.New("spawn failed")}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"localhost"})
	o.attachHealthCheckTimeout = 50 * time.Millisecond

	if _, err := o.Attach("ext", "ws://localhost:9222/devtools/browser/a"); err == nil {
		t.Fatal("expected attach to fail when the runner errors")
	}
	if n := len(o.List()); n != 0 {
		t.Fatalf("failed attach left %d residual instances (stale reservation)", n)
	}

	o.runner = &mockRunner{portAvail: true}
	if _, err := o.Attach("ext", "ws://localhost:9222/devtools/browser/a"); err != nil {
		t.Fatalf("retry after failed attach should succeed: %v", err)
	}
}

// slowAttachRunner widens the spawn window so the duplicate-name race is
// deterministic: the second attach must fail on the reservation, not spawn.
type slowAttachRunner struct {
	mockRunner
	delay time.Duration
	runs  atomic.Int32
}

func (r *slowAttachRunner) Run(ctx context.Context, binary string, args []string, env []string, stdout, stderr io.Writer) (Cmd, error) {
	r.runs.Add(1)
	time.Sleep(r.delay)
	return r.mockRunner.Run(ctx, binary, args, env, stdout, stderr)
}

// M9 regression: concurrent same-name attaches spawn at most one child.
func TestAttach_ConcurrentDuplicateNameSpawnsOneChild(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &slowAttachRunner{mockRunner: mockRunner{portAvail: true}, delay: 50 * time.Millisecond}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	allowAttachForTest(o, []string{"ws"}, []string{"localhost"})
	o.attachHealthCheckTimeout = 50 * time.Millisecond

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := o.Attach("shared", "ws://localhost:9222/devtools/browser/a")
			errs <- err
		}()
	}
	var failures int
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			failures++
			if !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("unexpected attach error: %v", err)
			}
		}
	}
	if failures != 1 {
		t.Fatalf("expected exactly one duplicate-name failure, got %d", failures)
	}
	if got := runner.runs.Load(); got != 1 {
		t.Fatalf("expected exactly one child spawn, got %d", got)
	}
}

// M9 regression: orphan cleanup must run even with an empty browser and nil
// runtime config — it defaults to the chrome sweep instead of no-opping.
func TestCleanupStoppedProfile_EmptyBrowserStillRunsCleanupHook(t *testing.T) {
	var swept []string
	providerhooks.Register("chrome", providerhooks.Hooks{CleanupProfile: func(p string) { swept = append(swept, p) }})
	t.Cleanup(func() {
		providerhooks.Register("chrome", providerhooks.Hooks{
			CleanupProfile: bridge.CleanupOrphanedChromeProcesses,
			Shutdown:       func() { bridge.KillAllPinchtabChrome() },
		})
	})

	o := &Orchestrator{baseDir: t.TempDir()}
	o.cleanupStoppedProfile("prof-x", "")
	if len(swept) != 1 {
		t.Fatalf("cleanup hook not invoked for empty browser + nil runtimeCfg: %v", swept)
	}
}

// L7(h): a browser-specific lookup must not fall back to instances of
// unknown provenance (empty Browser field).
func TestFirstRunningURLForBrowser_SkipsBrowserlessInstances(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.instances["legacy"] = &InstanceInternal{
		Instance: bridge.Instance{ID: "legacy", Status: "running", Port: "9001"},
		URL:      "http://localhost:9001",
	}

	if url := o.FirstRunningURLForBrowser("cloak"); url != "" {
		t.Fatalf("browser-specific lookup matched a browserless instance: %q", url)
	}
	if url := o.FirstRunningURLForBrowser(""); url == "" {
		t.Fatal("non-specific lookup should still reach legacy instances")
	}
}
