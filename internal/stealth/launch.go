package stealth

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

type LaunchContract struct {
	Args  []string
	Flags map[string]bool
}

func BuildLaunchContract(cfg *config.RuntimeConfig, level Level) LaunchContract {
	if config.PinchTabStealthDefaultsDisabled(cfg) {
		return LaunchContract{
			Flags: map[string]bool{
				"nativeCloakBrowser":          true,
				"pinchtabStealthArgsDisabled": true,
			},
		}
	}

	persona := BrowserPersona{}
	customUA := ""
	headless := false
	if cfg != nil {
		persona = BuildPersona(cfg.UserAgent, cfg.BrowserVersion)
		customUA = strings.TrimSpace(cfg.UserAgent)
		headless = cfg.Headless
	}

	args := []string{
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
	}
	// Pin --user-agent when EITHER:
	//   - the operator configured an explicit custom UA, OR
	//   - Chrome runs HEADLESS — its native navigator.userAgent contains
	//     "HeadlessChrome/..." (a loud fingerprint tell), and its native UA-CH
	//     is already degraded under --headless=new, so the UA-CH realism cost
	//     of pinning does not apply.
	//
	// In HEADED mode with no custom UA we leave --user-agent off so Chrome's
	// native, self-consistent UA + high-entropy UA Client Hints are served
	// (passing --user-agent otherwise empties them).
	pinUA := persona.UserAgent != "" && (customUA != "" || headless)
	if pinUA {
		args = append(args, "--user-agent="+persona.UserAgent)
	}
	if persona.Language != "" {
		args = append(args, "--lang="+persona.Language)
	}

	flags := map[string]bool{
		"automationControlledDisabled": true,
		"enableAutomationFalse":        true,
		"downlinkMaxFlag":              true,
		"globalUserAgent":              pinUA,
		"globalLanguage":               persona.Language != "",
	}
	if config.CloakBrowserActive(cfg) {
		flags["nativeCloakBrowser"] = true
	}

	return LaunchContract{
		Args:  args,
		Flags: flags,
	}
}

func HasLaunchArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func HasLaunchArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
