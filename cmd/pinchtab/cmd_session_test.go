package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sessionServer captures the last request and serves a canned JSON response.
type sessionServer struct {
	server     *httptest.Server
	lastMethod string
	lastPath   string
	lastBody   string
	response   string
	status     int
}

func newSessionServer(response string, status int) *sessionServer {
	s := &sessionServer{response: response, status: status}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastMethod = r.Method
		s.lastPath = r.URL.Path
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			s.lastBody = string(body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.response))
	}))
	return s
}

func (s *sessionServer) close() { s.server.Close() }

func (s *sessionServer) rt() cliRuntime {
	return cliRuntime{
		Client: &http.Client{},
		Base:   s.server.URL,
		Token:  "test-token",
	}
}

func TestSessionCLI_Create_SendsCorrectRequest(t *testing.T) {
	srv := newSessionServer(
		`{"id":"ses_abc","agentId":"agent-1","label":"ci","sessionToken":"ses_newtoken","status":"active"}`,
		http.StatusCreated,
	)
	defer srv.close()

	out := captureStdout(t, func() {
		rt := srv.rt()
		rt.Post("/sessions", map[string]any{"agentId": "agent-1", "label": "ci"})
	})

	if srv.lastMethod != "POST" {
		t.Fatalf("method = %q, want POST", srv.lastMethod)
	}
	if srv.lastPath != "/sessions" {
		t.Fatalf("path = %q, want /sessions", srv.lastPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(srv.lastBody), &sent); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if sent["agentId"] != "agent-1" {
		t.Fatalf("agentId = %q, want agent-1", sent["agentId"])
	}
	if sent["label"] != "ci" {
		t.Fatalf("label = %q, want ci", sent["label"])
	}
	if !strings.Contains(out, "ses_newtoken") {
		t.Fatalf("expected sessionToken in output, got:\n%s", out)
	}
}

func TestSessionCLI_Create_NoLabel_OmitsField(t *testing.T) {
	srv := newSessionServer(
		`{"id":"ses_abc","agentId":"agent-1","sessionToken":"ses_newtoken","status":"active"}`,
		http.StatusCreated,
	)
	defer srv.close()

	captureStdout(t, func() {
		rt := srv.rt()
		rt.Post("/sessions", map[string]any{"agentId": "agent-1"})
	})

	var sent map[string]any
	if err := json.Unmarshal([]byte(srv.lastBody), &sent); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if _, hasLabel := sent["label"]; hasLabel {
		t.Fatal("expected label to be absent when not provided")
	}
}

func TestSessionCLI_List_SendsCorrectRequest(t *testing.T) {
	srv := newSessionServer(`[]`, http.StatusOK)
	defer srv.close()

	captureStdout(t, func() {
		rt := srv.rt()
		rt.Get("/sessions", nil)
	})

	if srv.lastMethod != "GET" {
		t.Fatalf("method = %q, want GET", srv.lastMethod)
	}
	if srv.lastPath != "/sessions" {
		t.Fatalf("path = %q, want /sessions", srv.lastPath)
	}
}

func TestSessionCLI_Info_SendsCorrectRequest(t *testing.T) {
	srv := newSessionServer(`{"id":"ses_abc","agentId":"agent-1","status":"active"}`, http.StatusOK)
	defer srv.close()

	captureStdout(t, func() {
		rt := srv.rt()
		rt.Get("/sessions/me", nil)
	})

	if srv.lastMethod != "GET" {
		t.Fatalf("method = %q, want GET", srv.lastMethod)
	}
	if srv.lastPath != "/sessions/me" {
		t.Fatalf("path = %q, want /sessions/me", srv.lastPath)
	}
}

func TestSessionCLI_Revoke_SendsCorrectRequest(t *testing.T) {
	srv := newSessionServer(`{"status":"ok"}`, http.StatusOK)
	defer srv.close()

	const sessionID = "ses_abc123"

	captureStdout(t, func() {
		rt := srv.rt()
		rt.Post("/sessions/"+sessionID+"/revoke", nil)
	})

	if srv.lastMethod != "POST" {
		t.Fatalf("method = %q, want POST", srv.lastMethod)
	}
	if srv.lastPath != "/sessions/"+sessionID+"/revoke" {
		t.Fatalf("path = %q, want /sessions/%s/revoke", srv.lastPath, sessionID)
	}
}

func TestSessionCLI_Info_SendsSessionAuthHeader(t *testing.T) {
	srv := newSessionServer(`{"id":"ses_abc","agentId":"agent-1","status":"active"}`, http.StatusOK)
	defer srv.close()

	var gotAuth string
	srv.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ses_abc","agentId":"agent-1","status":"active"}`))
	})

	const sessionToken = "ses_mytoken"
	captureStdout(t, func() {
		rt := cliRuntime{
			Client: &http.Client{},
			Base:   srv.server.URL,
			Token:  sessionToken,
		}
		rt.Get("/sessions/me", nil)
	})

	if gotAuth != "Session "+sessionToken {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Session "+sessionToken)
	}
}

// `session create` is documented as `export PINCHTAB_SESSION=$(pinchtab session create
// ...)`, so stdout must stay the token alone — anything else lands in the variable. The
// id is the handle `session revoke` takes, so it goes to stderr where the capture cannot
// swallow it: withholding it is what forced a list-and-correlate detour.
func TestSessionCreatePrintsTheTokenOnStdoutAndTheIDOnStderr(t *testing.T) {
	stdout, stderr := captureStdoutStderr(t, func() {
		printSessionCreated(sessionCreateResult{ID: "ses_0123456789abcdef", Token: "ses_thetoken", Status: "active"})
	})

	if stdout != "ses_thetoken\n" {
		t.Errorf("stdout = %q, want the token alone; $(...) captures this", stdout)
	}
	if !strings.Contains(stderr, "ses_0123456789abcdef") {
		t.Errorf("stderr = %q, want the session id", stderr)
	}
	if !strings.Contains(stderr, "session revoke") {
		t.Errorf("stderr = %q, want it to name the verb the id is for", stderr)
	}
}

// A response without an id gets no hint rather than a hint naming nothing.
func TestSessionCreateSaysNothingExtraWithoutAnID(t *testing.T) {
	stdout, stderr := captureStdoutStderr(t, func() {
		printSessionCreated(sessionCreateResult{Token: "ses_thetoken", Status: "active"})
	})

	if stdout != "ses_thetoken\n" {
		t.Errorf("stdout = %q, want the token alone", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing when the response carries no id", stderr)
	}
}
