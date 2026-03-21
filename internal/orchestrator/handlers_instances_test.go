package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
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
	if inst.Headless {
		t.Fatal("Headless = true, want false for mode=headed")
	}
}
