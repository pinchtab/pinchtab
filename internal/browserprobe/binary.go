package browserprobe

import (
	"os"
	"os/exec"
)

// BinaryDiscovery captures both the selected executable and the search space
// inspected, so callers can produce useful diagnostics without duplicating
// discovery logic.
type BinaryDiscovery struct {
	Found  string
	Probed []string
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
