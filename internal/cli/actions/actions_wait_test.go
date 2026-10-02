package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newWaitCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("text", "", "")
	cmd.Flags().String("not-text", "", "")
	cmd.Flags().String("url", "", "")
	cmd.Flags().String("load", "", "")
	cmd.Flags().String("fn", "", "")
	cmd.Flags().String("state", "", "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Int("timeout-ms", 0, "")
	cmd.Flags().Int("timeout", 0, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newNavTimeoutCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Float64("timeout", 0, "")
	for _, name := range []string{"new-tab", "block-images", "block-ads", "dismiss-banners"} {
		cmd.Flags().Bool(name, false, "")
	}
	return cmd
}

func newScrapeTimeoutCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "json", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("cookies-file", "", "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().StringArray("cookie", nil, "")
	cmd.Flags().StringArray("include", nil, "")
	cmd.Flags().StringArray("exclude", nil, "")
	cmd.Flags().StringArray("only", nil, "")
	cmd.Flags().Int("max-pages", 0, "")
	cmd.Flags().Int("max-per-pattern", 0, "")
	cmd.Flags().Int("concurrency", 0, "")
	cmd.Flags().Int("timeout", 0, "")
	for _, name := range []string{"enrich-all", "no-browser", "preview", "json"} {
		cmd.Flags().Bool(name, false, "")
	}
	return cmd
}

// The card's core: the bare --timeout flag meant different units per verb. This
// pins the unit each verb's timeout flag feeds to the request — wait in
// milliseconds (via the new --timeout-ms and the deprecated --timeout alias),
// nav and scrape in seconds — so the 1000x divergence cannot silently return.
func TestTimeoutFlagUnitsPerVerb(t *testing.T) {
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = nil
		_ = json.Unmarshal(b, &lastBody)
		if strings.HasSuffix(r.URL.Path, "/wait") {
			_ = json.NewEncoder(w).Encode(map[string]any{"waited": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{}) // a valid, empty scrape report
	}))
	defer srv.Close()
	base, client := srv.URL, srv.Client()

	t.Run("wait --timeout-ms feeds milliseconds", func(t *testing.T) {
		cmd := newWaitCmd()
		_ = cmd.Flags().Set("timeout-ms", "2500")
		Wait(apiclient.Conn{Client: client, Base: base}, []string{"#x"}, cmd)
		if lastBody["timeout"] != float64(2500) {
			t.Fatalf("wait --timeout-ms 2500 sent timeout=%v, want 2500 (ms)", lastBody["timeout"])
		}
	})

	t.Run("wait --timeout (deprecated alias) still feeds milliseconds", func(t *testing.T) {
		cmd := newWaitCmd()
		_ = cmd.Flags().Set("timeout", "2500")
		Wait(apiclient.Conn{Client: client, Base: base}, []string{"#x"}, cmd)
		if lastBody["timeout"] != float64(2500) {
			t.Fatalf("wait --timeout 2500 sent timeout=%v, want 2500 (ms, back-compat)", lastBody["timeout"])
		}
	})

	t.Run("nav --timeout feeds seconds", func(t *testing.T) {
		cmd := newNavTimeoutCmd()
		_ = cmd.Flags().Set("timeout", "10")
		req := buildNavigateRequest("http://example.test", cmd)
		if req.body["timeout"] != float64(10) {
			t.Fatalf("nav --timeout 10 built timeout=%v, want 10 (seconds)", req.body["timeout"])
		}
	})

	t.Run("scrape --timeout feeds seconds on the unit-named key", func(t *testing.T) {
		cmd := newScrapeTimeoutCmd()
		_ = cmd.Flags().Set("timeout", "10")
		if err := Scrape(apiclient.Conn{Client: client, Base: base}, cmd, "http://example.test"); err != nil {
			t.Fatalf("Scrape: %v", err)
		}
		if lastBody["timeoutSeconds"] != float64(10) {
			t.Fatalf("scrape --timeout 10 sent timeoutSeconds=%v, want 10 (seconds)", lastBody["timeoutSeconds"])
		}
		if _, carriesBareTimeout := lastBody["timeout"]; carriesBareTimeout {
			t.Errorf("scrape must not send a bare ms-ambiguous timeout key: %v", lastBody)
		}
	})
}

func TestWait_ForwardsEveryConditionAndState(t *testing.T) {
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = nil
		_ = json.Unmarshal(b, &lastBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"waited": true})
	}))
	defer srv.Close()
	conn := apiclient.Conn{Client: srv.Client(), Base: srv.URL}

	for _, tc := range []struct {
		name  string
		args  []string
		flags map[string]string
		want  map[string]any
	}{
		{"text with state", nil, map[string]string{"text": "Order 42", "state": "hidden"}, map[string]any{"text": "Order 42", "state": "hidden"}},
		{"selector with text", []string{"#nope"}, map[string]string{"text": "X"}, map[string]any{"selector": "#nope", "text": "X"}},
		{"ms with state", []string{"500"}, map[string]string{"state": "hidden"}, map[string]any{"ms": float64(500), "state": "hidden"}},
		{
			"every flag",
			[]string{"#x"},
			map[string]string{"text": "a", "not-text": "b", "url": "c", "load": "d", "fn": "e", "state": "hidden"},
			map[string]any{"selector": "#x", "text": "a", "notText": "b", "url": "c", "load": "d", "fn": "e", "state": "hidden"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newWaitCmd()
			for flag, value := range tc.flags {
				_ = cmd.Flags().Set(flag, value)
			}
			Wait(conn, tc.args, cmd)
			if !reflect.DeepEqual(lastBody, tc.want) {
				t.Fatalf("wait sent %v, want %v", lastBody, tc.want)
			}
		})
	}
}

const waitChildEnv = "PINCHTAB_TEST_WAIT_MODE"

// runWaitChild serves a /wait response for the case named by mode ("met"/"timeout"
// prefix, "json" suffix) and drives Wait against it. A met wait returns normally,
// so it exits 0 explicitly — without that the test framework's trailing "PASS"
// output would land on stdout and break the single-JSON-object assertion. A timed
// out wait never reaches the exit here: Wait's output.Error already os.Exit(4)s.
func runWaitChild(mode string) {
	waited := strings.HasPrefix(mode, "met")
	jsonMode := strings.HasSuffix(mode, "json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"waited": waited, "elapsed": 1000}
		if !waited {
			resp["error"] = "timeout after 1000ms waiting for selector"
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cmd := newWaitCmd()
	if jsonMode {
		_ = cmd.Flags().Set("json", "true")
	}
	_ = cmd.Flags().Set("timeout", "1000")
	Wait(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, []string{"#neverever"}, cmd)
	os.Exit(0)
}

// The whole point of the card, asserted as exit codes: a met wait exits 0 and a
// timed-out wait exits ExitTimeout (4) — in BOTH terse and --json modes, so a
// script gating on the exit status keeps working when it adds --json. Driven in a
// child process because Wait's timeout path calls os.Exit.
func TestWait_ExitCodeTracksOutcomeNotTheFlag(t *testing.T) {
	if mode := os.Getenv(waitChildEnv); mode != "" {
		runWaitChild(mode)
		return
	}

	for _, tc := range []struct {
		mode     string
		wantExit int
		json     bool
	}{
		{"met-terse", 0, false},
		{"met-json", 0, true},
		{"timeout-terse", 4, false},
		{"timeout-json", 4, true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestWait_ExitCodeTracksOutcomeNotTheFlag", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), waitChildEnv+"="+tc.mode)
			var stdout, stderr bytes.Buffer
			child.Stdout = &stdout
			child.Stderr = &stderr

			err := child.Run()
			exitCode := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("child failed to run: %v (stderr: %s)", err, stderr.String())
			}

			if exitCode != tc.wantExit {
				t.Fatalf("%s exit = %d, want %d (stdout=%q stderr=%q)", tc.mode, exitCode, tc.wantExit, stdout.String(), stderr.String())
			}

			if tc.json {
				// AC: --json prints exactly one full JSON object, jq-parseable, before exiting.
				dec := json.NewDecoder(strings.NewReader(stdout.String()))
				var obj map[string]any
				if err := dec.Decode(&obj); err != nil {
					t.Fatalf("%s stdout is not one JSON object: %v\nstdout=%q", tc.mode, err, stdout.String())
				}
				if dec.More() {
					t.Fatalf("%s stdout has content after the JSON object:\nstdout=%q", tc.mode, stdout.String())
				}
				if _, ok := obj["waited"].(bool); !ok {
					t.Errorf("%s JSON object lacks a boolean waited field: %s", tc.mode, stdout.String())
				}
			} else if strings.Contains(stdout.String(), "{") {
				t.Errorf("%s terse stdout must not carry a JSON object: %q", tc.mode, stdout.String())
			}
		})
	}
}
