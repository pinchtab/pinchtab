package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func (f frontDoor) sessionRequest(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	req.Header.Set("Authorization", "Session "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func (f frontDoor) createSessionOverHTTP(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader([]byte(`{"agentId":"agent-under-test"}`)))
	req.Header.Set("Authorization", "Bearer "+frontDoorLiveToken)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v (body %s)", err, w.Body.String())
	}
	return body.Code
}

func agentSessionsEnabled(fc *config.FileConfig, enabled bool) {
	fc.Sessions.Agent.Enabled = &enabled
}

// Switching an authentication mechanism off is the direction that applies: both
// consumers that decide per request — the front door's session branch and the
// session API's own handlers — read the store live, so the save must move them
// and must not claim a restart is owed.
func TestDisablingAgentSessionsTakesEffectWithoutARestart(t *testing.T) {
	f := newFrontDoor(t, nil)

	_, token, err := f.agentSessions.Create("agent-under-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if before := f.sessionRequest(t, token); before.Code != http.StatusOK {
		t.Fatalf("the session token was refused before any save (%d: %s); this test never reached the state it is about",
			before.Code, before.Body.String())
	}
	if before := f.createSessionOverHTTP(t); before.Code != http.StatusCreated {
		t.Fatalf("POST /sessions answered %d before any save: %s", before.Code, before.Body.String())
	}

	f.save(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, false) })

	if f.agentSessions.Enabled() {
		t.Fatal("the store still reports agent sessions enabled; the save reached the file and not the running store")
	}
	after := f.sessionRequest(t, token)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("the existing session token still authenticated after the save (%d: %s)", after.Code, after.Body.String())
	}
	if got := errorCode(t, after); got != "session_auth_unavailable" {
		t.Errorf("refusal code = %q, want session_auth_unavailable naming the disabled mechanism", got)
	}
	create := f.createSessionOverHTTP(t)
	if create.Code != http.StatusNotFound {
		t.Fatalf("POST /sessions still minted a session after the save (%d: %s)", create.Code, create.Body.String())
	}
	if got := errorCode(t, create); got != CodeSessionsDisabled {
		t.Errorf("refusal code = %q, want %q", got, CodeSessionsDisabled)
	}
}

// The one direction that cannot apply: whether the family is mounted at all is
// decided at boot, so a process that booted with agent sessions off must say a
// restart is owed rather than report the save applied.
func TestEnablingAgentSessionsReportsARestartReason(t *testing.T) {
	f := newFrontDoor(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, false) })

	reasons := f.saveReportingReasons(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, true) })
	if !slices.Contains(reasons, "Agent sessions") {
		t.Fatalf("restartReasons = %v, want one naming the agent sessions block", reasons)
	}

	create := f.createSessionOverHTTP(t)
	if create.Code != http.StatusNotFound || errorCode(t, create) != CodeSessionsDisabled {
		t.Fatalf("POST /sessions answered %d (%s) after enabling; if the family were live the restart reason would be manufactured",
			create.Code, create.Body.String())
	}
}

// mode "off" is the other half of one question, and the docs promise it reduces
// the auth surface. It must reach exactly the state enabled false reaches — the
// same refusals, through the same assertions — or the two fields drift again.
// mode and the two timeouts have no boot-time consumer, so they apply live in
// both directions and never produce a restart reason.
func TestModeOffDisablesAgentSessionsExactlyAsEnabledFalseDoes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		disable func(*config.FileConfig)
		restore func(*config.FileConfig)
	}{
		{"enabled false", func(fc *config.FileConfig) { agentSessionsEnabled(fc, false) }, func(fc *config.FileConfig) { agentSessionsEnabled(fc, true) }},
		{"mode off", func(fc *config.FileConfig) { fc.Sessions.Agent.Mode = "off" }, func(fc *config.FileConfig) { fc.Sessions.Agent.Mode = "preferred" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFrontDoor(t, nil)

			_, token, err := f.agentSessions.Create("agent-under-test", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if before := f.sessionRequest(t, token); before.Code != http.StatusOK {
				t.Fatalf("the session token was refused before any save (%d: %s)", before.Code, before.Body.String())
			}

			f.save(t, tc.disable)

			if f.agentSessions.Enabled() {
				t.Fatal("the store still reports agent sessions enabled; Enabled() is not the one predicate both fields feed")
			}
			after := f.sessionRequest(t, token)
			if after.Code != http.StatusUnauthorized || errorCode(t, after) != "session_auth_unavailable" {
				t.Fatalf("the existing session token still authenticated (%d: %s)", after.Code, after.Body.String())
			}
			create := f.createSessionOverHTTP(t)
			if create.Code != http.StatusNotFound || errorCode(t, create) != CodeSessionsDisabled {
				t.Fatalf("POST /sessions answered %d (%s), want the disabled refusal", create.Code, create.Body.String())
			}

			// Back again, live: the family was mounted at boot, so nothing here is frozen.
			f.save(t, tc.restore)
			if !f.agentSessions.Enabled() {
				t.Fatal("the store did not come back; the restoring save reported applied and moved nothing")
			}
			if back := f.createSessionOverHTTP(t); back.Code != http.StatusCreated {
				t.Fatalf("POST /sessions answered %d (%s) after restoring", back.Code, back.Body.String())
			}
		})
	}
}

// The timeouts have no boot-time consumer either: they reach the store and never
// produce a restart reason. f.save asserts the second half.
func TestAgentSessionTimeoutsApplyLive(t *testing.T) {
	f := newFrontDoor(t, nil)

	f.save(t, func(fc *config.FileConfig) {
		idle, life := 60, 600
		fc.Sessions.Agent.IdleTimeoutSec = &idle
		fc.Sessions.Agent.MaxLifetimeSec = &life
	})
	if !f.agentSessions.Enabled() {
		t.Fatal("a timeout save disabled the store")
	}
}

// The store's config is replaced wholesale, so the mapping must carry the path
// the store was built with. Both failure modes are silent: dropping the path
// stops persistence, and recomputing it from an edited stateDir splits live
// sessions across two files.
func TestASaveKeepsTheAgentSessionStoreWritingToItsOriginalPath(t *testing.T) {
	f := newFrontDoor(t, nil)

	original := f.agentSessions.PersistPath()
	if original == "" {
		t.Fatal("the store booted with no persist path; this test cannot see a path move")
	}
	moved := t.TempDir()

	f.save(t, func(fc *config.FileConfig) { fc.Server.StateDir = moved })

	if got := f.agentSessions.PersistPath(); got != original {
		t.Fatalf("persist path after the save = %q, want %q", got, original)
	}
	id, _, err := f.agentSessions.Create("agent-after-save", "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read the original session file after the save: %v", err)
	}
	if !strings.Contains(string(data), id) {
		t.Errorf("the session created after the save is not in %s; the store stopped persisting there", original)
	}
	if _, err := os.Stat(filepath.Join(moved, "sessions.json")); err == nil {
		t.Errorf("the store started writing under the edited stateDir %s, splitting live sessions across two files", moved)
	}
}

// Both states answer sessions_disabled and both are genuinely disabled, so the code
// and the message are shared. The GUIDANCE cannot be: only the boot-disabled state
// owes a restart. A shared hint is this card's own defect one direction over — the
// machine-readable restartReasons says nothing is owed while the human-readable
// remedy in the same response family says to restart.
func TestTheTwoDisabledStatesDoNotShareGuidance(t *testing.T) {
	booted := newFrontDoor(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, false) })
	bootCode, _, bootHint, bootRemedy := decodeSessionRefusal(t, booted.createSessionOverHTTP(t))

	saved := newFrontDoor(t, nil)
	saved.save(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, false) })
	saveCode, _, saveHint, saveRemedy := decodeSessionRefusal(t, saved.createSessionOverHTTP(t))

	if bootCode != CodeSessionsDisabled || saveCode != CodeSessionsDisabled {
		t.Fatalf("codes = %q and %q, want both %q; the code is the state and both states are disabled",
			bootCode, saveCode, CodeSessionsDisabled)
	}
	if bootHint == saveHint {
		t.Fatalf("both disabled states answer the same hint %q; one of them is told to restart a server that re-enables live", bootHint)
	}
	if !strings.Contains(bootHint, "restart the server") {
		t.Errorf("the boot-disabled hint = %q, want it to say a restart is owed — the family was never mounted", bootHint)
	}
	if !strings.Contains(saveHint, "without a restart") {
		t.Errorf("the save-disabled hint = %q, want it to say no restart is owed", saveHint)
	}
	if bootRemedy != "" {
		t.Errorf("the boot-disabled remedy = %q, want none: a config edit plus a restart is not one command", bootRemedy)
	}
	if saveRemedy == "" {
		t.Error("the save-disabled state carries no remedy, though one command reverses it")
	}

	// The save-disabled hint's claim, checked rather than merely worded.
	reasons := saved.saveReportingReasons(t, func(fc *config.FileConfig) { agentSessionsEnabled(fc, true) })
	if len(reasons) != 0 {
		t.Fatalf("re-enabling after a save reported %v; the hint promises no restart is owed", reasons)
	}
	if create := saved.createSessionOverHTTP(t); create.Code != http.StatusCreated {
		t.Fatalf("POST /sessions answered %d (%s) after re-enabling; the hint promises it applies immediately",
			create.Code, create.Body.String())
	}
}
