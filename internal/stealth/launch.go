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
	persona := BrowserPersona{}
	if cfg != nil {
		persona = BuildPersona(cfg.UserAgent, cfg.ChromeVersion)
	}

	args := []string{
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
	}
	if persona.UserAgent != "" {
		args = append(args, "--user-agent="+persona.UserAgent)
	}
	if persona.Language != "" {
		args = append(args, "--lang="+persona.Language)
	}

	return LaunchContract{
		Args: args,
		Flags: map[string]bool{
			"automationControlledDisabled": true,
			"enableAutomationFalse":        true,
			"downlinkMaxFlag":              true,
			"globalUserAgent":              persona.UserAgent != "",
			"globalLanguage":               persona.Language != "",
		},
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
