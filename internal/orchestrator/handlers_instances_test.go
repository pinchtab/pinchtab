package orchestrator

import (
	"encoding/json"
	"fmt"
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

func setupTargetsOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome-local",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserChrome},
			"cloak-1":      {Provider: config.BrowserCloak},
		},
	})
	return o
}

func TestHandleStartInstance_LegacyConfigEmptyBody_NoTargetFields(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(``))
	req.ContentLength = 0
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `"browserTarget"`) || strings.Contains(body, `"browserProvider"`) {
		t.Fatalf("legacy config must not emit target/provider keys; body=%s", body)
	}
}

func TestHandleStartInstance_TargetsConfigValid_EchoesValues(t *testing.T) {
	o := setupTargetsOrchestrator(t)

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(`{"browser":"cloak"}`))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	body := w.Body.String()
	if strings.Contains(body, `"browserTarget"`) || strings.Contains(body, `"browserProvider"`) {
		t.Fatalf("JSON response must not contain browserTarget/browserProvider; body=%s", body)
	}
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "cloak-1" {
		t.Fatalf("internal BrowserTarget = %q, want cloak-1", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserCloak {
		t.Fatalf("internal BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}
}

func TestHandleStartInstance_TargetsConfigBadBrowser_Rejects400(t *testing.T) {
	o := setupTargetsOrchestrator(t)

	// "ghost-chrome" is a recognized provider but has no configured target.
	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(`{"browser":"ghost-chrome"}`))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestHandleLaunchByName_LegacyConfigEmptyBody_NoTargetFields(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(``))
	req.ContentLength = 0
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `"browserTarget"`) || strings.Contains(body, `"browserProvider"`) {
		t.Fatalf("legacy config must not emit target/provider keys; body=%s", body)
	}
}

func TestHandleLaunchByName_TargetsConfigValid_EchoesValues(t *testing.T) {
	o := setupTargetsOrchestrator(t)

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"browser":"chrome"}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	body := w.Body.String()
	if strings.Contains(body, `"browserTarget"`) || strings.Contains(body, `"browserProvider"`) {
		t.Fatalf("JSON response must not contain browserTarget/browserProvider; body=%s", body)
	}
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "chrome-local" {
		t.Fatalf("internal BrowserTarget = %q, want chrome-local", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserChrome {
		t.Fatalf("internal BrowserProvider = %q, want chrome", instances[0].BrowserProvider)
	}
}

func TestHandleLaunchByName_TargetsConfigBadBrowser_Rejects400(t *testing.T) {
	o := setupTargetsOrchestrator(t)

	// "ghost-chrome" is a recognized provider but has no configured target.
	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"browser":"ghost-chrome"}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAttachInstanceRejectsUnknownProvider(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"127.0.0.1"},
		AttachAllowSchemes: []string{"ws"},
	})

	req := httptest.NewRequest(http.MethodPost, "/instances/attach", strings.NewReader(`{
		"cdpUrl":"ws://127.0.0.1:9222/devtools/browser/abc",
		"provider":"cloack"
	}`))
	w := httptest.NewRecorder()

	o.handleAttachInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown browser") {
		t.Fatalf("body = %q, want unknown browser message", w.Body.String())
	}
}

func TestHandleAttachInstanceUsesDefaultBrowserTargetWhenOmitted(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true}
	o := setupAttachTargetsOrchestrator(t, runner)
	o.runtimeCfg.DefaultTarget = "cloak-1"

	req := httptest.NewRequest(http.MethodPost, "/instances/attach", strings.NewReader(`{
		"name":"attached-cloak",
		"cdpUrl":"ws://127.0.0.1:9222/devtools/browser/abc"
	}`))
	w := httptest.NewRecorder()

	o.handleAttachInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 body=%s", w.Code, w.Body.String())
	}
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "cloak-1" {
		t.Fatalf("internal BrowserTarget = %q, want cloak-1", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserCloak {
		t.Fatalf("internal BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}

	target, status, err := o.FirstRunningURLForRequest(httptest.NewRequest(http.MethodGet, "/text", nil))
	if err != nil {
		t.Fatalf("FirstRunningURLForRequest error status=%d err=%v", status, err)
	}
	if target != inst.URL {
		t.Fatalf("target URL = %q, want attached URL %q", target, inst.URL)
	}
}

func TestHandleAttachInstanceExplicitBrowserTargetRoutes(t *testing.T) {
	stubAttachBridgeHealthy(t)
	runner := &mockRunner{portAvail: true}
	o := setupAttachTargetsOrchestrator(t, runner)

	req := httptest.NewRequest(http.MethodPost, "/instances/attach", strings.NewReader(`{
		"name":"attached-cloak",
		"cdpUrl":"ws://127.0.0.1:9222/devtools/browser/abc",
		"browser":"cloak"
	}`))
	w := httptest.NewRecorder()

	o.handleAttachInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 body=%s", w.Code, w.Body.String())
	}
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "cloak-1" {
		t.Fatalf("internal BrowserTarget = %q, want cloak-1", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserCloak {
		t.Fatalf("internal BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}
	if !strings.Contains(strings.Join(runner.args, " "), "--browser-provider cloak") {
		t.Fatalf("runner args = %v, want cloak provider", runner.args)
	}

	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	target, status, err := o.FirstRunningURLForRequest(httptest.NewRequest(http.MethodGet, "/text?browser=cloak", nil))
	if err != nil {
		t.Fatalf("FirstRunningURLForRequest error status=%d err=%v", status, err)
	}
	if target != inst.URL {
		t.Fatalf("target URL = %q, want attached URL %q", target, inst.URL)
	}
}

func TestHandleAttachInstanceRejectsUnknownBrowser(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := setupAttachTargetsOrchestrator(t, runner)

	// "ghost-chrome" is a recognized provider but has no configured target.
	req := httptest.NewRequest(http.MethodPost, "/instances/attach", strings.NewReader(`{
		"name":"attached-ghost",
		"cdpUrl":"ws://127.0.0.1:9222/devtools/browser/abc",
		"browser":"ghost-chrome"
	}`))
	w := httptest.NewRecorder()

	o.handleAttachInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if runner.runCalled {
		t.Fatal("unknown browser should be rejected before starting child bridge")
	}
}

func TestHandleAttachInstanceRejectsProviderTargetConflict(t *testing.T) {
	runner := &mockRunner{portAvail: true}
	o := setupAttachTargetsOrchestrator(t, runner)

	req := httptest.NewRequest(http.MethodPost, "/instances/attach", strings.NewReader(`{
		"name":"attached-conflict",
		"cdpUrl":"ws://127.0.0.1:9222/devtools/browser/abc",
		"browser":"cloak",
		"provider":"chrome"
	}`))
	w := httptest.NewRecorder()

	o.handleAttachInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "conflicts with browserTarget") {
		t.Fatalf("body = %q, want provider conflict message", w.Body.String())
	}
	if runner.runCalled {
		t.Fatal("provider conflict should be rejected before starting child bridge")
	}
}

func TestHandleAttachBridgeUsesDefaultBrowserTarget(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path = %q, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	o := setupAttachTargetsOrchestrator(t, &mockRunner{portAvail: true})
	o.client = backend.Client()
	o.runtimeCfg.DefaultTarget = "cloak-1"
	o.runtimeCfg.AttachAllowHosts = []string{"*"}
	o.runtimeCfg.AttachAllowSchemes = []string{"http"}

	req := httptest.NewRequest(http.MethodPost, "/instances/attach-bridge", strings.NewReader(fmt.Sprintf(`{
		"name":"attached-bridge",
		"baseUrl":%q
	}`, backend.URL)))
	w := httptest.NewRecorder()

	o.handleAttachBridge(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 body=%s", w.Code, w.Body.String())
	}
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "cloak-1" {
		t.Fatalf("internal BrowserTarget = %q, want cloak-1", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserCloak {
		t.Fatalf("internal BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}

	target, status, err := o.FirstRunningURLForRequest(httptest.NewRequest(http.MethodGet, "/tabs", nil))
	if err != nil {
		t.Fatalf("FirstRunningURLForRequest error status=%d err=%v", status, err)
	}
	if target != backend.URL {
		t.Fatalf("target URL = %q, want bridge URL %q", target, backend.URL)
	}
}

func setupAttachTargetsOrchestrator(t *testing.T, runner *mockRunner) *Orchestrator {
	t.Helper()
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		AttachEnabled:      true,
		AttachAllowHosts:   []string{"127.0.0.1"},
		AttachAllowSchemes: []string{"ws"},
		DefaultTarget:      "chrome-local",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserChrome},
			"cloak-1":      {Provider: config.BrowserCloak},
		},
	})
	return o
}

func setupFallbackOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome-local",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserChrome},
			"cloak-1":      {Provider: config.BrowserCloak},
			"backup":       {Provider: config.BrowserChrome},
		},
		FallbackOrder: []string{"cloak-1"},
	})
	return o
}

func TestHandleStartInstance_RequestFallbackTargets_Wins(t *testing.T) {
	fastPolling(t)
	o := setupFallbackOrchestrator(t)
	fl := fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome-local", failReason: ReasonBinaryMissing},
		{target: "backup", succeed: true},
	})

	body := `{"browser":"chrome","fallbackTargets":["backup"]}`
	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(body))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if len(fl.calls) != 2 {
		t.Fatalf("expected 2 launch calls (primary + request fallback), got %d", len(fl.calls))
	}
	if fl.calls[1].ResolvedTarget != "backup" {
		t.Fatalf("second call BrowserTarget = %q, want backup (request must win over cfg.FallbackOrder)", fl.calls[1].ResolvedTarget)
	}

	// browserTarget is hidden from JSON (json:"-"), verify internal state.
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Target != "backup" {
		t.Fatalf("internal BrowserTarget = %q, want backup", instances[0].Target)
	}
	// FallbackFrom is still in JSON, verify from response.
	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inst.FallbackFrom != "chrome-local" {
		t.Fatalf("FallbackFrom = %q, want chrome-local", inst.FallbackFrom)
	}
}

func TestHandleStartInstance_ConfigFallbackOrder_UsedWhenRequestEmpty(t *testing.T) {
	fastPolling(t)
	o := setupFallbackOrchestrator(t)
	fl := fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome-local", failReason: ReasonBinaryMissing},
		{target: "cloak-1", succeed: true},
	})

	body := `{"browser":"chrome"}`
	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(body))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if len(fl.calls) != 2 {
		t.Fatalf("expected 2 launch calls (primary + cfg fallback), got %d", len(fl.calls))
	}
	if fl.calls[1].ResolvedTarget != "cloak-1" {
		t.Fatalf("second call BrowserTarget = %q, want cloak-1 (from cfg.FallbackOrder)", fl.calls[1].ResolvedTarget)
	}
}

func TestHandleStartInstance_LegacyConfig_NoFallback(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	defer func() { processAliveFunc = old }()
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.fallbackLauncher = fakeLauncherFor(nil)

	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(``))
	req.ContentLength = 0
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestHandleStartInstance_Exhaustion_502(t *testing.T) {
	fastPolling(t)
	o := setupFallbackOrchestrator(t)
	_ = fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome-local", failReason: ReasonBinaryMissing},
		{target: "backup", failReason: ReasonStartupTimeout},
	})

	body := `{"browser":"chrome","fallbackTargets":["backup"]}`
	req := httptest.NewRequest(http.MethodPost, "/instances/start", strings.NewReader(body))
	w := httptest.NewRecorder()

	o.handleStartInstance(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["code"] != "browser_target_unavailable" {
		t.Fatalf("code = %v, want browser_target_unavailable; body=%s", payload["code"], w.Body.String())
	}
	details, ok := payload["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing; body=%s", w.Body.String())
	}
	attempts, ok := details["attempts"].([]any)
	if !ok {
		t.Fatalf("attempts missing in details; body=%s", w.Body.String())
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	got := make(map[string]string, len(attempts))
	for _, raw := range attempts {
		entry := raw.(map[string]any)
		got[entry["target"].(string)] = entry["reason"].(string)
	}
	if got["chrome-local"] != string(ReasonBinaryMissing) {
		t.Fatalf("chrome-local reason = %q, want %q", got["chrome-local"], ReasonBinaryMissing)
	}
	if got["backup"] != string(ReasonStartupTimeout) {
		t.Fatalf("backup reason = %q, want %q", got["backup"], ReasonStartupTimeout)
	}
}

func TestHandleLaunchByName_RequestFallbackTargets_Wins(t *testing.T) {
	fastPolling(t)
	o := setupFallbackOrchestrator(t)
	fl := fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome-local", failReason: ReasonBinaryMissing},
		{target: "backup", succeed: true},
	})

	body := `{"browser":"chrome","fallbackTargets":["backup"]}`
	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(body))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if len(fl.calls) != 2 {
		t.Fatalf("expected 2 launch calls, got %d", len(fl.calls))
	}
	if fl.calls[1].ResolvedTarget != "backup" {
		t.Fatalf("second call BrowserTarget = %q, want backup", fl.calls[1].ResolvedTarget)
	}
}

func TestHandleLaunchByName_Exhaustion_502(t *testing.T) {
	fastPolling(t)
	o := setupFallbackOrchestrator(t)
	_ = fakeLauncherForOrch(t, o, []scriptedOutcome{
		{target: "chrome-local", failReason: ReasonBinaryMissing},
		{target: "cloak-1", failReason: ReasonStartupTimeout},
	})

	req := httptest.NewRequest(http.MethodPost, "/instances/launch", strings.NewReader(`{"browser":"chrome"}`))
	w := httptest.NewRecorder()

	o.handleLaunchByName(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "browser_target_unavailable") {
		t.Fatalf("body missing browser_target_unavailable code: %s", w.Body.String())
	}
}
