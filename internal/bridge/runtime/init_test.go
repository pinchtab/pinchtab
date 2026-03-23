package runtime

import (
	"os"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestChromeNeedsNoSandbox(t *testing.T) {
	origGOOS := runtimeGOOS
	origGeteuid := osGeteuid
	origMarker := containerMarkerPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osGeteuid = origGeteuid
		containerMarkerPath = origMarker
	})

	runtimeGOOS = "linux"
	osGeteuid = func() int { return 1000 }
	containerMarkerPath = t.TempDir() + "/missing-dockerenv"

	if chromeNeedsNoSandbox() {
		t.Fatal("expected no-sandbox compatibility to be disabled without root, env override, or container marker")
	}

	t.Setenv(config.ChromeNoSandboxEnvVar(), "1")
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected env override to enable no-sandbox compatibility")
	}
	t.Setenv(config.ChromeNoSandboxEnvVar(), "")

	osGeteuid = func() int { return 0 }
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected root user to enable no-sandbox compatibility")
	}
	osGeteuid = func() int { return 1000 }

	containerMarkerPath = t.TempDir() + "/.dockerenv"
	if err := os.WriteFile(containerMarkerPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected container marker to enable no-sandbox compatibility")
	}
}
