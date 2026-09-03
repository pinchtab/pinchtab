package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/pinchtab/pinchtab/internal/session"
)

// The three states are produced by the REAL registrars rather than hand-written JSON, so
// this is a producer-to-consumer test: if a code, a message or a remedy changes on the
// server side, the assertion the CLI makes about it moves with it instead of drifting.
func sessionCreateResponse(t *testing.T, register func(*http.ServeMux), path string) []byte {
	t.Helper()

	mux := http.NewServeMux()
	if register != nil {
		register(mux)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	return w.Body.Bytes()
}

// This is the criterion the card exists for. All THREE states, because a test covering
// only the bridge leaves the conflation in place: what was wrong was not one bad message
// but one message serving three states, and only asserting each separately can show that
// each remedy is reachable and correct where it is printed.
func TestSessionCreateAdviceDiffersPerUnavailableState(t *testing.T) {
	for _, tc := range []struct {
		name string
		// register is nil for the unknown-path state: nothing mounts the family, which
		// is precisely what a typo or an old server looks like.
		register   func(*http.ServeMux)
		path       string
		wantAdvice bool
		// wantGuidance is matched against hint AND remedy together: the two states carry
		// their guidance in different fields, because only one of them has a single command
		// to run, and the CLI prints whichever is present.
		wantGuidance string
		banned       string
		why          string
	}{
		{
			name:         "bridge mode",
			register:     server.RegisterSessionsUnavailableInBridgeMode,
			path:         "/sessions",
			wantAdvice:   true,
			wantGuidance: "pinchtab server",
			banned:       "sessions.agent.enabled",
			why:          "no config value mounts the family on a bridge, so a config remedy is the unescapable loop this card fixes",
		},
		{
			name:         "server with sessions disabled",
			register:     func(mux *http.ServeMux) { server.RegisterSessionsDisabled(mux, []string{session.SettingEnabled}) },
			path:         "/sessions",
			wantAdvice:   true,
			wantGuidance: "sessions.agent.enabled",
			banned:       "",
			why:          "this is the one state the config edit fits, and it is a file edit rather than a command",
		},
		{
			name:       "unknown path",
			register:   nil,
			path:       "/sessions",
			wantAdvice: false,
			why:        "an unrecognised body must fall through to the generic API error rather than reusing either session remedy",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := sessionCreateResponse(t, tc.register, tc.path)

			message, hint, remedy, ok := sessionUnavailableAdvice(body)
			guidance := hint + " " + remedy
			if ok != tc.wantAdvice {
				t.Fatalf("advice recognised = %v, want %v; %s (body=%s)", ok, tc.wantAdvice, tc.why, body)
			}
			if !tc.wantAdvice {
				return
			}
			if message == "" {
				t.Errorf("no message, so the CLI prints an empty Error: line")
			}
			if !strings.Contains(guidance, tc.wantGuidance) {
				t.Errorf("guidance = %q, want it to name %q; %s", guidance, tc.wantGuidance, tc.why)
			}
			if tc.banned != "" && strings.Contains(guidance, tc.banned) {
				t.Errorf("guidance = %q, which prescribes %q; %s", guidance, tc.banned, tc.why)
			}
		})
	}
}

// The mapping must be exhaustive rather than permissive. An unrecognised code — a future
// mode, or an older server — must NOT inherit either remedy, because keying off a
// permissive default is how 404 came to serve three states.
func TestAnUnrecognisedCodeGetsNoSessionAdvice(t *testing.T) {
	for _, body := range []string{
		`{"code":"sessions_unavailable_in_some_future_mode","error":"nope","details":{"remedy":"do something"}}`,
		`{"code":"session_not_found","error":"session not found"}`,
		`{"error":"agent sessions are not enabled on this server"}`,
		`404 page not found`,
		``,
	} {
		if _, _, _, ok := sessionUnavailableAdvice([]byte(body)); ok {
			t.Errorf("body %q was treated as a known unavailable state; an unrecognised code must fall through to the generic error", body)
		}
	}
}
