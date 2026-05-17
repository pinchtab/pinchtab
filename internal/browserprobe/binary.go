package browserprobe

import (
	"os"
	"os/exec"
	"path/filepath"
)

// BinaryDiscovery captures both the selected executable and the search space
// inspected, so callers can produce useful diagnostics without duplicating
// discovery logic.
type BinaryDiscovery struct {
	Found  string
	Probed []string
}

var chromeBinaryNames = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"chrome",
}

var cloakBinaryNames = []string{
	"cloakbrowser",
}

// DiscoverChromeBinary returns the first executable Chrome/Chromium candidate
// found via PATH or known install locations for goos.
func DiscoverChromeBinary(goos string) BinaryDiscovery {
	return DiscoverBinary(chromeBinaryNames, ChromeCommonPaths(goos))
}

// DiscoverCloakBrowserBinary returns the first executable CloakBrowser
// candidate found via PATH or known install locations for goos.
func DiscoverCloakBrowserBinary(goos string) BinaryDiscovery {
	return DiscoverBinary(cloakBinaryNames, CloakCommonPaths(goos))
}

// ChromeCommonPaths returns per-OS Chrome install paths probed after PATH
// misses.
func ChromeCommonPaths(goos string) []string {
	switch goos {
	case "linux":
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/opt/google/chrome/chrome",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return nil
	}
}

// CloakCommonPaths returns per-OS CloakBrowser install paths probed after PATH
// misses.
func CloakCommonPaths(goos string) []string {
	switch goos {
	case "linux", "darwin":
		paths := []string{
			"/opt/cloakbrowser/chrome",
		}
		if home := homeDir(); home != "" {
			paths = append(paths, filepath.Join(home, ".cloakbrowser", "chrome"))
		}
		return paths
	default:
		return nil
	}
}

// DiscoverBinary returns the first executable found via PATH then fallback
// paths; Probed lists every location inspected for diagnostic messages.
func DiscoverBinary(names, paths []string) BinaryDiscovery {
	var probed []string
	for _, name := range names {
		probed = append(probed, "$PATH:"+name)
		if p, err := exec.LookPath(name); err == nil {
			return BinaryDiscovery{Found: p, Probed: probed}
		}
	}
	for _, p := range paths {
		probed = append(probed, p)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		return BinaryDiscovery{Found: p, Probed: probed}
	}
	return BinaryDiscovery{Probed: probed}
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}
