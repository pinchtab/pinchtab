package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// The CLI half of the grants ingress, driven through the REAL command tree in a child
// process, because that is the only way the repeatable flag, the local validation and the
// os.Exit are all the shipped ones. A test that calls apiclient directly — which is what
// the create tests beside this one do — proves the server contract and says nothing about
// whether --grant reaches it.
func runSessionCreateCLI(t *testing.T, args ...string) (string, int, int32) {
	t.Helper()

	if os.Getenv("PINCHTAB_GRANT_HELPER") == "1" {
		rootCmd.SetArgs(append([]string{"--server", os.Getenv("PINCHTAB_GRANT_SERVER")}, strings.Split(os.Getenv("PINCHTAB_GRANT_ARGS"), "\x1f")...))
		if err := rootCmd.Execute(); err != nil {
			os.Exit(commandExitCode(err))
		}
		os.Exit(0)
	}

	var requests atomic.Int32
	var body atomic.Value
	body.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		requests.Add(1)
		raw, _ := io.ReadAll(r.Body)
		body.Store(string(raw))
		// The stub echoes the grants back the way the real API does — from what it
		// stored, not from a canned literal — so the CLI's scope line is read from a
		// response rather than from the flags the caller typed.
		var sent struct {
			Grants []string `json:"grants"`
		}
		_ = json.Unmarshal(raw, &sent)
		reply := map[string]any{"id": "ses_abc", "sessionToken": "ses_token", "status": "active"}
		if len(sent.Grants) > 0 {
			reply["grants"] = sent.Grants
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)

	child := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary with fixed arguments.
	child.Env = append(os.Environ(),
		"PINCHTAB_GRANT_HELPER=1",
		"PINCHTAB_GRANT_SERVER="+srv.URL,
		"PINCHTAB_GRANT_ARGS="+strings.Join(args, "\x1f"),
		"PINCHTAB_TOKEN=test-token",
		"HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
	)
	out, err := child.CombinedOutput()

	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running the CLI failed with %v; output:\n%s", err, out)
	}
	return string(out) + body.Load().(string), code, requests.Load()
}

func TestSessionCreateSendsEveryRepeatedGrantToTheAPI(t *testing.T) {
	out, code, requests := runSessionCreateCLI(t,
		"session", "create", "--agent-id", "agent-1", "--grant", "browse", "--grant", "console")
	if os.Getenv("PINCHTAB_GRANT_HELPER") == "1" {
		return
	}

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; output:\n%s", code, out)
	}
	if requests != 1 {
		t.Fatalf("the CLI made %d create requests, want 1", requests)
	}
	var sent struct {
		AgentID string   `json:"agentId"`
		Grants  []string `json:"grants"`
	}
	payload := out[strings.LastIndex(out, "{"):]
	if err := json.Unmarshal([]byte(payload), &sent); err != nil {
		t.Fatalf("the request body is not JSON (%v): %s", err, payload)
	}
	if sent.AgentID != "agent-1" {
		t.Errorf("agentId = %q, want agent-1", sent.AgentID)
	}
	if strings.Join(sent.Grants, ",") != "browse,console" {
		t.Errorf("grants = %v, want both repeats of the flag; a session scoped to fewer groups than asked for is the failure this reports", sent.Grants)
	}
	// The echo where a human reads it. The API answers with the grants it applied, so
	// this is the line that would disagree with the request if the scope had not taken.
	if !strings.Contains(out, "grants: browse, console") {
		t.Errorf("the default create output does not report the scope the server applied:\n%s", out)
	}
}

// The same disclosure for the session that narrows nothing: silence there would read as
// "scope unknown" on the very output the scoped case uses to report one.
func TestSessionCreateReportsAnUnscopedSessionAsUnscoped(t *testing.T) {
	out, code, _ := runSessionCreateCLI(t, "session", "create", "--agent-id", "agent-1")
	if os.Getenv("PINCHTAB_GRANT_HELPER") == "1" {
		return
	}

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; output:\n%s", code, out)
	}
	if !strings.Contains(out, "unscoped") {
		t.Errorf("the default create output does not say the session is unscoped:\n%s", out)
	}
}

// The local validation the flag's help promises: a typo must cost no round trip, and the
// message must name the offender and the whole vocabulary, because the caller cannot guess
// eleven names from a rejection.
func TestSessionCreateRefusesAMistypedGrantBeforeTheWire(t *testing.T) {
	out, code, requests := runSessionCreateCLI(t,
		"session", "create", "--agent-id", "agent-1", "--grant", "brows")
	if os.Getenv("PINCHTAB_GRANT_HELPER") == "1" {
		return
	}

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out)
	}
	if requests != 0 {
		t.Errorf("the CLI issued %d request(s) for a grant it could reject locally", requests)
	}
	for _, want := range []string{"brows", "browse", "activity"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal does not carry %q — it must name the offender and the whole vocabulary:\n%s", want, out)
		}
	}
}

// An entry naming nothing, at the surface a human types it on. Before the refusal existed
// this exited 0 with an UNSCOPED session, which is the outcome the caller was trying to
// avoid by passing the flag at all.
func TestSessionCreateRefusesAGrantThatNamesNothing(t *testing.T) {
	out, code, requests := runSessionCreateCLI(t,
		"session", "create", "--agent-id", "agent-1", "--grant", "")
	if os.Getenv("PINCHTAB_GRANT_HELPER") == "1" {
		return
	}

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; an empty --grant created a session that reaches every non-admin route:\n%s", code, out)
	}
	if requests != 0 {
		t.Errorf("the CLI issued %d request(s) for an empty grant", requests)
	}
}
