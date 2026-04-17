package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func TestHandleLaunchByNameRejectsNameField(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"name":"work","mode":"headed"}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "name is not supported on /instances/launch") {
		t.Fatalf("body = %q, want unsupported-name message", w.Body.String())
	}
}

func TestHandleLaunchByNameAliasesStartSemantics(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	baseDir := t.TempDir()
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(baseDir, runner)
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatalf("CreateWithMeta: %v", err)
	}
	o.profiles = pm

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"profileId":"work","mode":"headed"}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if !runner.runCalled {
		t.Fatal("expected instance launch to invoke the runner")
	}

	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if inst.ProfileName != "work" {
		t.Fatalf("ProfileName = %q, want %q", inst.ProfileName, "work")
	}
	if inst.Mode != "headed" {
		t.Fatalf("Mode = %q, want %q", inst.Mode, "headed")
	}
	if inst.Headless {
		t.Fatal("Headless = true, want false for mode=headed")
	}
}

func TestHandleStartInstanceRejectsExtensionPaths(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(`{"extensionPaths":["/tmp/malicious-ext"]}`))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "extensionPaths are not supported on instance start requests") {
		t.Fatalf("body = %q, want extensionPaths rejection message", w.Body.String())
	}
}

func TestHandleLaunchByNameRejectsExtensionPaths(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"extensionPaths":["/tmp/malicious-ext"]}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "extensionPaths are not supported on instance start requests") {
		t.Fatalf("body = %q, want extensionPaths rejection message", w.Body.String())
	}
}

func TestHandleStartInstance_AppliesSecurityPolicyOverride(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AllowedDomains: []string{"127.0.0.1", "localhost"},
	})

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(`{"mode":"headed","securityPolicy":{"allowedDomains":["wikipedia.org","localhost"]}}`))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if !runner.runCalled {
		t.Fatal("expected instance launch to invoke the runner")
	}

	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if inst.Mode != "headed" {
		t.Fatalf("Mode = %q, want %q", inst.Mode, "headed")
	}
	if inst.SecurityPolicy == nil {
		t.Fatal("expected securityPolicy on instance response")
	}
	want := []string{"127.0.0.1", "localhost", "wikipedia.org"}
	if len(inst.SecurityPolicy.AllowedDomains) != len(want) {
		t.Fatalf("securityPolicy.allowedDomains = %v, want %v", inst.SecurityPolicy.AllowedDomains, want)
	}
	for i := range want {
		if inst.SecurityPolicy.AllowedDomains[i] != want[i] {
			t.Fatalf("securityPolicy.allowedDomains = %v, want %v", inst.SecurityPolicy.AllowedDomains, want)
		}
	}

	cfgPath := envMap(runner.env)["PINCHTAB_CONFIG"]
	if cfgPath == "" {
		t.Fatal("PINCHTAB_CONFIG missing from child env")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read child config: %v", err)
	}
	var childCfg config.FileConfig
	if err := json.Unmarshal(data, &childCfg); err != nil {
		t.Fatalf("decode child config: %v", err)
	}
	if len(childCfg.Security.AllowedDomains) != len(want) {
		t.Fatalf("child security.allowedDomains = %v, want %v", childCfg.Security.AllowedDomains, want)
	}
	for i := range want {
		if childCfg.Security.AllowedDomains[i] != want[i] {
			t.Fatalf("child security.allowedDomains = %v, want %v", childCfg.Security.AllowedDomains, want)
		}
	}
}

func TestHandleStartInstance_RejectsInvalidSecurityPolicyOverride(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(`{"securityPolicy":{"allowedDomains":["bad domain"]}}`))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid securityPolicy.allowedDomains") {
		t.Fatalf("body = %q, want securityPolicy validation message", w.Body.String())
	}
}
