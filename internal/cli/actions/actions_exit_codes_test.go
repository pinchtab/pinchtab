package actions

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli"
)

const setGeoChildEnv = "PINCHTAB_SET_GEO_EXIT_CHILD"

func runSetGeoChild(mode string) {
	m := newMockServer()
	defer m.close()
	args := []string{"1.5", "2.5"}
	switch mode {
	case "bad-argument":
		args = []string{"north", "2.5"}
	case "api-500":
		m.setResponse(http.MethodPost, "/emulation/geolocation", http.StatusInternalServerError, `{"error":"boom"}`)
	}
	SetGeolocation(m.server.Client(), m.base(), "", newEmulationCmd(), args)
	os.Exit(0)
}

func TestSetGeolocation_ExitCodeSeparatesUsageFromRuntimeFailure(t *testing.T) {
	if mode := os.Getenv(setGeoChildEnv); mode != "" {
		runSetGeoChild(mode)
		return
	}

	for _, tc := range []struct {
		mode     string
		wantExit int
	}{
		{"bad-argument", cli.ExitUsage},
		{"api-500", cli.ExitRuntime},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestSetGeolocation_ExitCodeSeparatesUsageFromRuntimeFailure", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), setGeoChildEnv+"="+tc.mode)
			out, err := child.CombinedOutput()

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("child err = %v, want exit %d (output: %s)", err, tc.wantExit, out)
			}
			if exitErr.ExitCode() != tc.wantExit {
				t.Fatalf("exit = %d, want %d (output: %s)", exitErr.ExitCode(), tc.wantExit, out)
			}
		})
	}
}
