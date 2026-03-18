package orchestrator

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestOrchestrator_Launch_Lifecycle(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	inst, err := o.Launch("profile1", "9001", true, nil)
	if err != nil {
		t.Fatalf("First launch failed: %v", err)
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

func TestOrchestrator_Launch_DisabledByRuntimeConfig(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{DisableLocalLaunch: true})

	_, err := o.Launch("profile1", "9001", true, nil)
	if err == nil || !strings.Contains(err.Error(), "local instance launch is disabled") {
		t.Fatalf("Launch() error = %v, want local launch disabled error", err)
	}
}

func TestOrchestrator_ListAndStop(t *testing.T) {
	alive := true
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return alive }
	defer func() { processAliveFunc = old }()

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

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	validNames := []string{
		"simple",
		"with-dash",
		"with_underscore",
		"with.dot",
		"CamelCase",
		"123numeric",
		"a",
	}

	for i, name := range validNames {
		t.Run(name, func(t *testing.T) {
			port := 9100 + i
			inst, err := o.Launch(name, string(rune('0'+port%10))+string(rune('0'+(port/10)%10))+string(rune('0'+(port/100)%10))+string(rune('0'+(port/1000)%10)), true, nil)
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

	inst, err := o.AttachBridge("bridge1", "http://10.0.0.8:9868", "bridge-token")
	if err != nil {
		t.Fatalf("AttachBridge failed: %v", err)
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
