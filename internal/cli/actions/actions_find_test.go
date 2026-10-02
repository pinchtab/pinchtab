package actions

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newFindCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("threshold", "", "")
	cmd.Flags().Bool("explain", false, "")
	cmd.Flags().Bool("ref-only", false, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestFind_NegativeQueryForwarded(t *testing.T) {
	m := newMockServer()
	m.response = `{"best_ref":"e1","matches":[{"ref":"e1","role":"button","name":"Cancel"}]}`
	defer m.close()

	cmd := newFindCmd()
	_ = cmd.Flags().Set("ref-only", "true")
	Find(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "button not submit", cmd)

	if m.lastPath != "/find" {
		t.Errorf("expected /find, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["query"] != "button not submit" {
		t.Errorf("expected query passthrough, got %v", body["query"])
	}
}

func TestFind_VisualQueryWithTabRoute(t *testing.T) {
	m := newMockServer()
	m.response = `{"best_ref":"e2","matches":[{"ref":"e2","role":"button","name":"Action"}]}`
	defer m.close()

	cmd := newFindCmd()
	_ = cmd.Flags().Set("tab", "tab1")
	_ = cmd.Flags().Set("ref-only", "true")
	Find(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "bottom button", cmd)

	if m.lastPath != "/tabs/tab1/find" {
		t.Errorf("expected /tabs/tab1/find, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["query"] != "bottom button" {
		t.Errorf("expected visual query passthrough, got %v", body["query"])
	}
}

const noMatchPayload = `{"best_ref":"","confidence":"low","matches":[]}`

// find was the one read verb that said nothing when it had nothing to say, so an agent
// could not tell "not on this page" from "the command failed" — three situations with three
// different next actions.
func TestFind_NoMatchPrintsTheEmptyStateNamingTheQuery(t *testing.T) {
	m := newMockServer()
	m.response = noMatchPayload
	defer m.close()

	cmd := newFindCmd()
	out := captureStdout(t, func() {
		Find(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "a unicorn that does not exist", cmd)
	})

	want := `No elements matched "a unicorn that does not exist"`
	if strings.TrimSpace(out) != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// A match must still print the match and nothing else: the empty state sits in the same
// branch as the single-result format, so an else in the wrong place would print both.
func TestFind_AMatchDoesNotPrintTheEmptyState(t *testing.T) {
	m := newMockServer()
	m.response = `{"best_ref":"e2","role":"textbox","name":"Password"}`
	defer m.close()

	cmd := newFindCmd()
	out := captureStdout(t, func() {
		Find(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "the password box", cmd)
	})

	if strings.Contains(out, "No elements matched") {
		t.Errorf("stdout = %q, want the match line alone", out)
	}
	if !strings.Contains(out, `e2	textbox	"Password"`) {
		t.Errorf("stdout = %q, want the terse match line", out)
	}
}

// --json is the machine contract: the renderer edit must not add a line to it.
func TestFind_JSONOutputCarriesNoEmptyStateLine(t *testing.T) {
	m := newMockServer()
	m.response = noMatchPayload
	defer m.close()

	cmd := newFindCmd()
	_ = cmd.Flags().Set("json", "true")
	out := captureStdout(t, func() {
		Find(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "a unicorn that does not exist", cmd)
	})

	if strings.Contains(out, "No elements matched") {
		t.Fatalf("--json stdout = %q, want the payload alone", out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("--json stdout is not the payload any more (%v): %q", err, out)
	}
	if payload["confidence"] != "low" {
		t.Errorf("--json payload = %v, want the server's body unchanged", payload)
	}
}

// The two no-match contracts, asserted as exit codes because that is the whole difference:
// the default path answers 0 like every other empty read, and --ref-only fails so
// REF=$(pinchtab find … --ref-only) never receives an empty string. Both are driven in a
// child process, since cli.Fatal calls os.Exit.
func TestFind_NoMatchExitsZeroByDefaultAndNonZeroForRefOnly(t *testing.T) {
	if mode := os.Getenv(findNoMatchChildEnv); mode != "" {
		runFindNoMatchChild(mode == "ref-only")
		return
	}

	for _, tc := range []struct {
		mode      string
		wantFatal bool
	}{
		{"default", false},
		{"ref-only", true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestFind_NoMatchExitsZeroByDefaultAndNonZeroForRefOnly", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), findNoMatchChildEnv+"="+tc.mode)
			raw, err := child.CombinedOutput()

			var exitErr *exec.ExitError
			failed := errors.As(err, &exitErr)
			if failed != tc.wantFatal {
				t.Fatalf("%s exited with %v, want fatal=%v; output:\n%s", tc.mode, err, tc.wantFatal, raw)
			}
			if !strings.Contains(string(raw), `No elements matched "a unicorn that does not exist"`) {
				t.Errorf("%s said nothing about the empty result; output:\n%s", tc.mode, raw)
			}
		})
	}
}

const findNoMatchChildEnv = "PINCHTAB_TEST_FIND_NO_MATCH_MODE"

func runFindNoMatchChild(refOnly bool) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(noMatchPayload))
	}))
	defer srv.Close()

	cmd := newFindCmd()
	if refOnly {
		_ = cmd.Flags().Set("ref-only", "true")
	}
	Find(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "a unicorn that does not exist", cmd)
}

const findTieChildEnv = "PINCHTAB_TEST_FIND_TIE"

func TestFind_RefOnlyRefusesAnAmbiguousTie(t *testing.T) {
	if os.Getenv(findTieChildEnv) != "" {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"best_ref":"e3","confidence":"ambiguous","tied_refs":["e3","e1","e2"],"retry_query":"nth:<i>:role:button Add","matches":[{"ref":"e3","role":"button","name":"Add"}]}`))
		}))
		defer srv.Close()
		cmd := newFindCmd()
		_ = cmd.Flags().Set("ref-only", "true")
		Find(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, "Add button for Blue cup", cmd)
		return
	}

	child := exec.Command(os.Args[0], "-test.run=TestFind_RefOnlyRefusesAnAmbiguousTie", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
	child.Env = append(os.Environ(), findTieChildEnv+"=1")
	raw, err := child.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("--ref-only exited with %v on a tie, want non-zero; output:\n%s", err, raw)
	}
	if !strings.Contains(string(raw), "e3, e1, e2") {
		t.Errorf("output does not name the tied refs:\n%s", raw)
	}
	if !strings.Contains(string(raw), "nth:<i>:role:button Add") {
		t.Errorf("output does not offer the positional retry query:\n%s", raw)
	}
	if !strings.Contains(string(raw), "i counts from 0") {
		t.Errorf("%s does not state the zero-based nth index that action selectors use", string(raw))
	}
}

func TestAmbiguousMatchAcrossRolesDescribesEachTiedRef(t *testing.T) {
	result := map[string]any{
		"tied_refs": []any{"e0", "e1"},
		"matches": []any{
			map[string]any{"ref": "e0", "role": "button", "name": "Add item"},
			map[string]any{"ref": "e1", "role": "link", "name": "Add item"},
		},
	}

	msg := ambiguousMatch("Add item", []string{"e0", "e1"}, result)

	if !strings.Contains(msg, `e0 (button "Add item"), e1 (link "Add item")`) {
		t.Errorf("message = %q, want each tied ref with its role and name", msg)
	}
	for _, banned := range []string{"i counts from", "%!", "<nil>"} {
		if strings.Contains(msg, banned) {
			t.Errorf("message = %q, must not contain %q without a retry query", msg, banned)
		}
	}
}
