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
			"chrome-local": {Provider: config.BrowserChrome},
			"cloak-1":      {Provider: config.BrowserCloak},
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
	// browserTarget/browserProvider are hidden from JSON (json:"-"),
	// so verify via the orchestrator's internal state instead.
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].Browser != config.BrowserCloak {
		t.Fatalf("internal Browser = %q, want cloak", instances[0].Browser)
	}
}

func TestHandleStartByID_TargetsConfigBadBrowser_Rejects400(t *testing.T) {
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
	// Only a cloak target — requesting "ghost" normalizes to "chrome",
	// which has no matching target.
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "cloak-1",
		Targets: config.BrowserTargetsConfig{
			"cloak-1": {Provider: config.BrowserCloak},
		},
	})
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatalf("CreateWithMeta: %v", err)
	}
	o.profiles = pm

	req := httptest.NewRequest(http.MethodPost, "/profiles/work/start", strings.NewReader(`{"browser":"ghost"}`))
	req.SetPathValue("id", "work")
	w := httptest.NewRecorder()

	o.handleStartByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestHandleProfileInstanceDistinguishesMissingFromStopped(t *testing.T) {
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatalf("CreateWithMeta: %v", err)
	}
	o.profiles = pm

	for _, tc := range []struct {
		name       string
		wantExists bool
		wantStatus string
	}{
		{"work", true, "stopped"},
		{"missing", false, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/profiles/"+tc.name+"/instance", nil)
			req.SetPathValue("id", tc.name)
			w := httptest.NewRecorder()
			o.handleProfileInstance(w, req)

			var body struct {
				Exists  bool   `json:"exists"`
				Status  string `json:"status"`
				Message string `json:"message"`
			}
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Exists != tc.wantExists || body.Status != tc.wantStatus {
				t.Fatalf("exists/status = %v/%q, want %v/%q", body.Exists, body.Status, tc.wantExists, tc.wantStatus)
			}
			if !tc.wantExists && (!strings.Contains(body.Message, "does not exist") || !strings.Contains(body.Message, "human setup step")) {
				t.Fatalf("missing-profile message does not stop an agent retry loop: %q", body.Message)
			}
		})
	}
}
