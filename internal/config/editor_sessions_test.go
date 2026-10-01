package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func saveAndReload(t *testing.T, fc *FileConfig, path string) map[string]any {
	t.Helper()

	if err := SaveFileConfig(fc, path); err != nil {
		t.Fatalf("SaveFileConfig: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("config on disk is not JSON (%v): %s", err, raw)
	}
	return onDisk
}

func agentBlockOnDisk(t *testing.T, onDisk map[string]any) map[string]any {
	t.Helper()

	sessions, ok := onDisk["sessions"].(map[string]any)
	if !ok {
		t.Fatalf("no sessions block on disk: %v", onDisk)
	}
	agent, ok := sessions["agent"].(map[string]any)
	if !ok {
		t.Fatalf("no sessions.agent block on disk; the value was accepted and discarded: %v", sessions)
	}
	return agent
}

// The live defect, and the first thing to assert: `config patch` answered "Config patched
// successfully" and left the file byte-identical. The update path patches the existing
// file object with the marshalled map, so a key the wire type never carried could not
// appear however it was set.
//
// Driven through the real save against an EXISTING file, because that is the only path
// where the defect lives — a test that only marshals the struct would pass while the
// shipped command still wrote nothing.
func TestPatchingAnAgentSessionKeyPersistsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":"18899"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	fc := &FileConfig{}
	if err := PatchConfigJSON(fc, `{"sessions":{"agent":{"enabled":false,"mode":"header"}}}`); err != nil {
		t.Fatalf("PatchConfigJSON: %v", err)
	}

	agent := agentBlockOnDisk(t, saveAndReload(t, fc, path))
	if agent["enabled"] != false {
		t.Errorf("sessions.agent.enabled on disk = %v, want false", agent["enabled"])
	}
	if agent["mode"] != "header" {
		t.Errorf("sessions.agent.mode on disk = %v, want %q", agent["mode"], "header")
	}
}

// The card's original finding: the editor answered "unknown field sessions.agent.enabled",
// which claims the key is wrong rather than that it is not settable here — for the one
// switch that turns on the whole agent-session flow.
func TestTheEditorAddressesAgentSessionKeysAndTheySurviveASave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":"18899"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	fc := &FileConfig{}
	for _, tc := range []struct{ key, value, want string }{
		{"sessions.agent.enabled", "false", "false"},
		{"sessions.agent.mode", "header", "header"},
		{"sessions.agent.idleTimeoutSec", "900", "900"},
		{"sessions.agent.maxLifetimeSec", "7200", "7200"},
	} {
		if err := SetConfigValue(fc, tc.key, tc.value); err != nil {
			t.Fatalf("config set %s: %v", tc.key, err)
		}
		got, err := GetConfigValue(fc, tc.key)
		if err != nil {
			t.Fatalf("config get %s: %v", tc.key, err)
		}
		if got != tc.want {
			t.Errorf("config get %s = %q, want %q", tc.key, got, tc.want)
		}
	}

	agent := agentBlockOnDisk(t, saveAndReload(t, fc, path))
	if agent["enabled"] != false || agent["mode"] != "header" {
		t.Errorf("agent block on disk = %v, want the values just set", agent)
	}
	if agent["idleTimeoutSec"] != float64(900) || agent["maxLifetimeSec"] != float64(7200) {
		t.Errorf("agent timeouts on disk = %v, want 900 and 7200", agent)
	}
}

// A hand-written agent block is how every operator enabled this before the fix, so it must
// survive an unrelated set.
//
// WHAT THIS ACTUALLY GUARDS, measured rather than assumed. Preservation does NOT come from
// the marshal carrying the value: the save PATCHES the file key by key, so a field the
// marshal drops (nil under omitempty) leaves the existing key untouched and the old value
// survives anyway. A mutation that makes the marshal forget the value therefore leaves this
// test GREEN. What it catches is the marshal emitting an explicit OVERWRITING value — a
// zeroed *bool rather than a nil one — which patches false over the operator's true. That is
// the destruction shape, and it is the one the new Agent block could have introduced.
func TestAHandWrittenAgentBlockSurvivesAnUnrelatedSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"server":{"port":"18899"},"sessions":{"agent":{"enabled":true,"mode":"header","idleTimeoutSec":1234}}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	fc := &FileConfig{}
	if err := json.Unmarshal([]byte(original), fc); err != nil {
		t.Fatalf("load the hand-written config: %v", err)
	}
	if err := SetConfigValue(fc, "server.port", "19000"); err != nil {
		t.Fatal(err)
	}

	onDisk := saveAndReload(t, fc, path)
	agent := agentBlockOnDisk(t, onDisk)
	if agent["enabled"] != true {
		t.Errorf("sessions.agent.enabled = %v after an unrelated set, want the hand-written true preserved", agent["enabled"])
	}
	if agent["mode"] != "header" {
		t.Errorf("sessions.agent.mode = %v, want the hand-written value preserved", agent["mode"])
	}
	if agent["idleTimeoutSec"] != float64(1234) {
		t.Errorf("sessions.agent.idleTimeoutSec = %v, want the hand-written 1234 preserved", agent["idleTimeoutSec"])
	}
	server, _ := onDisk["server"].(map[string]any)
	if server["port"] != "19000" {
		t.Errorf("server.port = %v, want the set value 19000", server["port"])
	}
}

// The sessions vocabulary is checked against the DECLARED fields rather than against the
// names missing today. A hand-listed switch is exactly how `enabled` fell out of a section
// that otherwise looked complete, so listing the fix would reproduce the defect's cause.
func TestTheSessionsEditorAddressesEveryDeclaredField(t *testing.T) {
	for _, section := range []struct {
		prefix string
		typ    reflect.Type
	}{
		{"sessions.agent.", reflect.TypeFor[AgentSessionFileConfig]()},
		{"sessions.dashboard.", reflect.TypeFor[DashboardSessionFileConfig]()},
	} {
		if section.typ.NumField() == 0 {
			t.Fatalf("%s declares no fields; this census is reading the wrong type", section.prefix)
		}
		for field := range section.typ.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			key := section.prefix + name

			fc := &FileConfig{}
			if _, err := GetConfigValue(fc, key); err != nil {
				t.Errorf("config get %s: %v — the field is declared and the loader honours it, so refusing it as unknown tells the user the key is wrong", key, err)
			}
		}
	}
}

// The other direction: the editor must not invent keys the section does not declare.
// sessions.dashboard.enabled is the case that prompted this — it is refused today and the
// refusal is CORRECT, because no such field exists on the type, in the schema, or in the
// loader. Adding it would create exactly the accepted-and-discarded shape this card removes.
func TestTheSessionsEditorRefusesAKeyTheSectionDoesNotDeclare(t *testing.T) {
	fc := &FileConfig{}

	if _, ok := reflect.TypeFor[DashboardSessionFileConfig]().FieldByName("Enabled"); ok {
		t.Fatal("DashboardSessionFileConfig now declares Enabled; this test pins its ABSENCE, so either the field is real and the editor should address it, or it was added by mistake")
	}
	if _, err := GetConfigValue(fc, "sessions.dashboard.enabled"); err == nil {
		t.Error("config get sessions.dashboard.enabled was accepted, but nothing declares or honours it — a settable key nothing reads is the accepted-and-discarded shape this card exists to remove")
	}
	if _, err := GetConfigValue(fc, "sessions.agent.nonesuch"); err == nil {
		t.Error("config get sessions.agent.nonesuch was accepted; the agent branch must still refuse an undeclared field")
	}
}
