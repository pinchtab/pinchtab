package browsers

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
)

// DoctorBinary is the browser a doctor check will exercise: the configured
// override when set, otherwise what discovery finds. Every check resolves the
// same way, so none of them can skip on a default install that discovery serves.
type DoctorBinary struct {
	Path       string
	Overridden bool
	Probed     []string
}

func ResolveDoctorBinary(cfg interface{}, names, paths []string) DoctorBinary {
	if env, ok := cfg.(*DoctorEnv); ok && env != nil {
		if override := strings.TrimSpace(env.Binary); override != "" {
			return DoctorBinary{Path: override, Overridden: true}
		}
	}
	d := browserprobe.DiscoverBinary(names, paths)
	return DoctorBinary{Path: d.Found, Probed: d.Probed}
}

func DoctorSandboxArgs(cfg interface{}) []string {
	if env, ok := cfg.(*DoctorEnv); ok && env != nil && env.NoSandbox {
		return []string{"--no-sandbox"}
	}
	return nil
}
