package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func TestHandleStartByID_TargetsConfigDefault_EchoesValues(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	baseDir := t.TempDir()
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(baseDir, runner)
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "cloak-1",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserProviderChrome},
			"cloak-1":      {Provider: config.BrowserProviderCloak},
		},
	})
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatalf("CreateWithMeta: %v", err)
	}
	o.profiles = pm

	req := httptest.NewRequest(http.MethodPost, "/profiles/work/start", strings.NewReader(`{}`))
	req.SetPathValue("id", "work")
	w := httptest.NewRecorder()

	o.handleStartByID(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var inst bridge.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inst.ProfileName != "work" {
		t.Fatalf("ProfileName = %q, want work", inst.ProfileName)
	}
	if inst.BrowserTarget != "cloak-1" {
		t.Fatalf("BrowserTarget = %q, want cloak-1", inst.BrowserTarget)
	}
	if inst.BrowserProvider != config.BrowserProviderCloak {
		t.Fatalf("BrowserProvider = %q, want cloak", inst.BrowserProvider)
	}
}

func TestHandleStartByID_TargetsConfigBadTarget_Rejects400(t *testing.T) {
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome-local",
		Targets: config.BrowserTargetsConfig{
			"chrome-local": {Provider: config.BrowserProviderChrome},
		},
	})
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatalf("CreateWithMeta: %v", err)
	}
	o.profiles = pm

	req := httptest.NewRequest(http.MethodPost, "/profiles/work/start", strings.NewReader(`{"browserTarget":"ghost"}`))
	req.SetPathValue("id", "work")
	w := httptest.NewRecorder()

	o.handleStartByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("body = %q, want 'not found'", w.Body.String())
	}
}
