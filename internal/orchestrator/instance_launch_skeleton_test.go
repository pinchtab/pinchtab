package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLaunchOfAnUnknownProfileSeedsChromiumPreferences(t *testing.T) {
	stubPortAvailability(t, func(int) bool { return true })
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})

	inst, err := o.Launch("never-created", "", true, nil)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	t.Cleanup(func() { _ = o.Stop(inst.ID) })

	prefs := filepath.Join(baseDir, "never-created", "Default", "Preferences")
	if data, err := os.ReadFile(prefs); err != nil || string(data) != "{}" {
		t.Fatalf("Preferences at %s = %q (err=%v), want the seeded {}", prefs, data, err)
	}
}
