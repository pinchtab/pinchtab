package server

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the given URL in the user's default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start() // #nosec G204
	case "darwin":
		return exec.Command("open", url).Start() // #nosec G204
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start() // #nosec G204
	default:
		return exec.Command("xdg-open", url).Start() // #nosec G204
	}
}
