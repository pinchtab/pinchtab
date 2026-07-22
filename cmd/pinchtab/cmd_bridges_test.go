package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridgeregistry"
)

func TestWriteBridgesTextIncludesDiagnosticIdentity(t *testing.T) {
	state := bridgeregistry.State{
		Record: bridgeregistry.Record{
			PID:          4242,
			Address:      "127.0.0.1",
			Port:         "9878",
			CDPIdentity:  bridgeregistry.SafeCDPIdentity("ws://user:password@127.0.0.1:9222/devtools/browser/browser-guid?token=secret"),
			BrowserType:  "chrome",
			BrowserLabel: "work-profile",
			RegisteredAt: time.Unix(1, 0).UTC(),
		},
		Status:    "live",
		PIDStatus: "alive",
		Reachable: true,
		Live:      true,
	}
	var out bytes.Buffer
	if err := writeBridges(&out, []bridgeregistry.State{state}, false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"live", "pid=4242", "pidStatus=alive", "reachable=true", "bridge=http://127.0.0.1:9878", `browser="chrome"`, `label="work-profile"`, `cdp="ws://127.0.0.1:9222/devtools/browser/`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q missing %q", got, want)
		}
	}
	for _, secret := range []string{"password", "browser-guid", "token=secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("output %q leaked %q", got, secret)
		}
	}
}

func TestWriteBridgesJSONAndEmptyText(t *testing.T) {
	states := []bridgeregistry.State{{
		Record:    bridgeregistry.Record{PID: 9, Address: "::1", Port: "9871", BrowserType: "cloak"},
		Status:    "stale_pid",
		PIDStatus: "dead",
		Stale:     true,
		Pruned:    true,
	}}
	var out bytes.Buffer
	if err := writeBridges(&out, states, true); err != nil {
		t.Fatal(err)
	}
	var decoded []bridgeregistry.State
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if len(decoded) != 1 || decoded[0].PID != 9 || !decoded[0].Pruned || decoded[0].Status != "stale_pid" {
		t.Fatalf("decoded output = %+v", decoded)
	}

	out.Reset()
	if err := writeBridges(&out, nil, false); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "No registered standalone bridges.\n" {
		t.Fatalf("empty output = %q", got)
	}
}

func TestBridgesCommandContract(t *testing.T) {
	if bridgesCmd.GroupID != "management" {
		t.Fatalf("bridges group = %q", bridgesCmd.GroupID)
	}
	if bridgesListCmd.Flags().Lookup("json") == nil || bridgesListCmd.Flags().Lookup("prune") == nil {
		t.Fatal("bridges list must expose --json and --prune")
	}
}
