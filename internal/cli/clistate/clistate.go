// Package clistate owns where the CLI keeps its own local state: the current-tab
// file, the advisory markers, and whatever comes next. One resolver, so two
// features cannot disagree about which directory an operator has to clear.
package clistate

import (
	"os"
	"path/filepath"
)

// Dir is the CLI's state directory. It is not the server's stateDir: this is
// per-user, per-machine scratch that belongs to the command line itself, so it
// follows the XDG state convention and falls back to a path that always exists.
func Dir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "pinchtab")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "pinchtab")
	}
	return filepath.Join(os.TempDir(), "pinchtab")
}
