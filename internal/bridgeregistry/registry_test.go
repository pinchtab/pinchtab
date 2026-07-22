package bridgeregistry

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSafeCDPIdentityDropsSecretsAndKeepsBrowserIdentity(t *testing.T) {
	raw := "ws://user:password@127.0.0.1:9222/devtools/browser/browser-guid?token=secret#fragment"
	got := SafeCDPIdentity(raw)
	for _, secret := range []string{"user", "password", "browser-guid", "token", "secret", "fragment"} {
		if strings.Contains(got, secret) {
			t.Fatalf("SafeCDPIdentity(%q) leaked %q in %q", raw, secret, got)
		}
	}
	if got == SafeCDPIdentity("ws://127.0.0.1:9222/devtools/browser/other-guid") {
		t.Fatal("different external browsers produced the same identity")
	}
	if got != SafeCDPIdentity("ws://127.0.0.1:9222/devtools/browser/browser-guid?other=value") {
		t.Fatal("query-only differences changed the external browser identity")
	}
	if got := SafeCDPIdentity("https://user:secret@cdp.example:9443/json/version?access_token=nope"); got != "https://cdp.example:9443" {
		t.Fatalf("HTTP identity = %q, want sanitized origin", got)
	}
}

func TestRegisterListAndClose(t *testing.T) {
	stateDir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	registration, err := Register(stateDir, Record{
		Address:      "127.0.0.1",
		Port:         strconv.Itoa(port),
		CDPIdentity:  "ws://user:password@127.0.0.1:9222/devtools/browser/browser-guid?token=secret",
		BrowserType:  "chrome",
		BrowserLabel: "work-profile",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Join(stateDir, registryDir))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("registry mode = %o, want 700", got)
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, registryDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || strings.HasSuffix(entries[0].Name(), ".tmp") {
		t.Fatalf("published registry entries = %v, want one JSON record", entries)
	}
	fileInfo, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("record mode = %o, want 600", got)
	}

	states, err := List(stateDir, false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("List() returned %d records, want 1", len(states))
	}
	state := states[0]
	if !state.Live || state.Stale || !state.Reachable || state.PIDStatus != "alive" || state.Status != "live" {
		t.Fatalf("live state = %+v", state)
	}
	if state.PID != os.Getpid() || state.ProcessStartUnixMillis <= 0 {
		t.Fatalf("process identity = pid %d start %d", state.PID, state.ProcessStartUnixMillis)
	}
	if state.CDPIdentity == "" || strings.Contains(state.CDPIdentity, "password") || strings.Contains(state.CDPIdentity, "browser-guid") {
		t.Fatalf("unsafe CDP identity %q", state.CDPIdentity)
	}

	_ = listener.Close()
	states, err = List(stateDir, true)
	if err != nil {
		t.Fatalf("List(prune) error = %v", err)
	}
	if len(states) != 1 || states[0].Status != "stale_listener" || states[0].Pruned {
		t.Fatalf("live PID with closed listener must be retained: %+v", states)
	}

	if err := registration.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := registration.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	states, err = List(stateDir, false)
	if err != nil || len(states) != 0 {
		t.Fatalf("records after Close() = %+v, err %v", states, err)
	}
}

func TestListPrunesOnlyConclusiveStalePIDs(t *testing.T) {
	stateDir := t.TempDir()
	root, err := openRegistry(stateDir, true)
	if err != nil {
		t.Fatal(err)
	}
	started, err := processStart(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	records := map[string]Record{
		"bridge-0-dead.json":    {PID: 0, Address: "127.0.0.1", Port: "1", BrowserType: "chrome"},
		"bridge-1-unknown.json": {PID: os.Getpid(), Address: "127.0.0.1", Port: "1", BrowserType: "chrome"},
		"bridge-2-reused.json":  {PID: os.Getpid(), ProcessStartUnixMillis: started + 1, Address: "127.0.0.1", Port: "1", BrowserType: "chrome"},
	}
	for name, rec := range records {
		data, marshalErr := json.Marshal(rec)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := root.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = root.Close()

	states, err := List(stateDir, false)
	if err != nil || len(states) != 3 {
		t.Fatalf("stale listing = %+v, err %v", states, err)
	}
	states, err = List(stateDir, true)
	if err != nil || len(states) != 3 {
		t.Fatalf("pruned listing = %+v, err %v", states, err)
	}
	for _, state := range states {
		wantPruned := state.PIDStatus == "dead" || state.PIDStatus == "reused"
		if state.Pruned != wantPruned {
			t.Fatalf("state %+v prune = %t, want %t", state.Record, state.Pruned, wantPruned)
		}
	}
	states, err = List(stateDir, false)
	if err != nil || len(states) != 1 || states[0].PIDStatus != "unknown" {
		t.Fatalf("records after prune = %+v, err %v", states, err)
	}
}

func TestListMissingRegistryIsEmpty(t *testing.T) {
	states, err := List(t.TempDir(), false)
	if err != nil || len(states) != 0 {
		t.Fatalf("List() = %+v, %v; want empty", states, err)
	}
}

func TestListRejectsRegistrySymlink(t *testing.T) {
	stateDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(stateDir, registryDir)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Register(stateDir, Record{Address: "127.0.0.1", Port: "9867", BrowserType: "chrome"}); err == nil {
		t.Fatal("Register() accepted a symlinked registry directory")
	}
}
