package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newPruneTestCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "prune"}
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().Bool("json", false, "")
	_ = cmd.ParseFlags(args)
	return cmd
}

// pruneStub answers /profiles/prune and records the body it was sent, so the assertions
// can be about what the CLI ASKED for rather than only what it printed.
func pruneStub(t *testing.T, response string, sent *pruneQuarantinedBody, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/profiles/prune" {
			t.Errorf("CLI requested %s, want /profiles/prune", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, sent); err != nil {
			t.Errorf("CLI sent a body that is not the documented shape (%v): %s", err, body)
		}
		_, _ = w.Write([]byte(response))
	}))
}

type pruneQuarantinedBody struct {
	Confirm bool   `json:"confirm"`
	Profile string `json:"profile"`
}

// Without --confirm the CLI must send confirm:false. The server decides what that means,
// but a CLI that sent true would delete on the invocation documented as a dry run, and no
// assertion about printed text could see it.
func TestProfilesPruneWithoutConfirmAsksForADryRun(t *testing.T) {
	var sent pruneQuarantinedBody
	var hits atomic.Int32
	srv := pruneStub(t, `{"removed":false,"count":2,"totalBytes":1500,"profiles":[
		{"name":"default.quarantine-1700000001","bytes":500},
		{"name":"work.quarantine-1700000002","bytes":1000}]}`, &sent, &hits)
	defer srv.Close()

	out := captureStdout(t, func() {
		ProfilesPrune(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, newPruneTestCmd())
	})

	if hits.Load() != 1 {
		t.Fatalf("server saw %d requests, want 1", hits.Load())
	}
	if sent.Confirm {
		t.Error("the bare command sent confirm:true, so it would delete on the invocation documented as a dry run")
	}
	if !strings.Contains(out, "re-run with --confirm") {
		t.Errorf("dry-run output does not say how to actually reclaim: %q", out)
	}
	if strings.Contains(out, "Reclaimed") {
		t.Errorf("dry-run output claims it reclaimed something: %q", out)
	}
	if !strings.Contains(out, "1.5 KB reclaimable") {
		t.Errorf("dry-run output does not report the reclaimable total: %q", out)
	}
}

func TestProfilesPruneWithConfirmAsksForRemovalAndReportsWhatWasFreed(t *testing.T) {
	var sent pruneQuarantinedBody
	var hits atomic.Int32
	srv := pruneStub(t, `{"removed":true,"count":1,"totalBytes":2048,"profiles":[
		{"name":"default.quarantine-1700000001","bytes":2048}]}`, &sent, &hits)
	defer srv.Close()

	out := captureStdout(t, func() {
		ProfilesPrune(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, newPruneTestCmd("--confirm"))
	})

	if !sent.Confirm {
		t.Error("--confirm did not reach the request body, so the server would answer a dry run")
	}
	if !strings.Contains(out, "Reclaimed 1 quarantined profile(s), 2.0 KB freed") {
		t.Errorf("output does not report what was freed: %q", out)
	}
}

func TestProfilesPruneForwardsTheNamedProfile(t *testing.T) {
	var sent pruneQuarantinedBody
	var hits atomic.Int32
	srv := pruneStub(t, `{"removed":true,"count":1,"totalBytes":10,"profiles":[{"name":"work.quarantine-1700000002","bytes":10}]}`, &sent, &hits)
	defer srv.Close()

	captureStdout(t, func() {
		ProfilesPrune(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, newPruneTestCmd("--confirm", "--profile", "work.quarantine-1700000002"))
	})

	if sent.Profile != "work.quarantine-1700000002" {
		t.Errorf("request carried profile %q, want the name the user passed — a dropped selector reclaims everything instead of one directory", sent.Profile)
	}
}

// An empty backlog is the common case on a healthy machine and must not print an empty
// list that reads as an error.
func TestProfilesPruneSaysSoWhenThereIsNothingToReclaim(t *testing.T) {
	var sent pruneQuarantinedBody
	var hits atomic.Int32
	srv := pruneStub(t, `{"removed":false,"count":0,"totalBytes":0,"profiles":[]}`, &sent, &hits)
	defer srv.Close()

	out := captureStdout(t, func() {
		ProfilesPrune(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, newPruneTestCmd())
	})

	if !strings.Contains(out, "No quarantined profiles to reclaim") {
		t.Errorf("output = %q, want it to say there is nothing to reclaim", out)
	}
}
