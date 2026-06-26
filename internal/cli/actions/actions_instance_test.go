package actions

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func newInstanceStartCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().String("port", "", "")
	cmd.Flags().String("stealth-level", "", "")
	cmd.Flags().StringArray("extension", nil, "")
	return cmd
}

func TestInstanceStartSendsStealthLevel(t *testing.T) {
	m := newMockServer()
	defer m.close()

	cmd := newInstanceStartCmd()
	_ = cmd.Flags().Set("profile", "rcs-luke")
	_ = cmd.Flags().Set("mode", "headless")
	_ = cmd.Flags().Set("port", "9870")
	_ = cmd.Flags().Set("stealth-level", "full")

	InstanceStart(m.server.Client(), m.base(), "", cmd)

	if m.lastMethod != "POST" {
		t.Fatalf("method = %s, want POST", m.lastMethod)
	}
	if m.lastPath != "/instances/start" {
		t.Fatalf("path = %s, want /instances/start", m.lastPath)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["profileId"] != "rcs-luke" {
		t.Fatalf("profileId = %v, want rcs-luke", body["profileId"])
	}
	if body["stealthLevel"] != "full" {
		t.Fatalf("stealthLevel = %v, want full", body["stealthLevel"])
	}
}
