package config

import (
	"fmt"
	"strings"
)

const chromeNoSandboxEnvVar = "PINCHTAB_CHROME_NO_SANDBOX"

func ChromeNoSandboxEnvVar() string {
	return chromeNoSandboxEnvVar
}

// AllowedChromeExtraFlags returns the subset of browser.extraFlags that are
// allowed to reach the browser process after security and ownership checks.
func AllowedChromeExtraFlags(raw string) []string {
	allowed, _ := assessChromeExtraFlags(raw)
	return allowed
}

// SanitizeChromeExtraFlags rewrites browser.extraFlags to contain only the
// allowed subset. Invalid, unsafe, and reserved flags are dropped.
func SanitizeChromeExtraFlags(raw string) string {
	return strings.Join(AllowedChromeExtraFlags(raw), " ")
}

func validateChromeExtraFlags(raw string) []error {
	_, errs := assessChromeExtraFlags(raw)
	return errs
}

func assessChromeExtraFlags(raw string) ([]string, []error) {
	fields := strings.Fields(raw)
	allowed := make([]string, 0, len(fields))
	var errs []error

	for _, field := range fields {
		if field == "" {
			continue
		}
		if err := validateChromeExtraFlag(field); err != nil {
			errs = append(errs, err)
			continue
		}
		allowed = append(allowed, field)
	}

	return allowed, errs
}

func validateChromeExtraFlag(flag string) error {
	if !strings.HasPrefix(flag, "--") {
		return chromeExtraFlagsError(flag, "must use Chrome flag syntax like --name or --name=value")
	}

	name, value := splitChromeFlag(flag)
	lowerName := strings.ToLower(name)

	if reservedField, ok := reservedChromeExtraFlagFields[lowerName]; ok {
		return chromeExtraFlagsError(flag, fmt.Sprintf("is owned by PinchTab launch config; use %s instead", reservedField))
	}
	if msg, ok := disallowedChromeExtraFlags[lowerName]; ok {
		return chromeExtraFlagsError(flag, msg)
	}
	if lowerName == "--disable-features" && disablesSiteIsolation(value) {
		return chromeExtraFlagsError(flag, "disables site isolation, which weakens browser security and is not allowed")
	}

	return nil
}

func splitChromeFlag(flag string) (name, value string) {
	parts := strings.SplitN(flag, "=", 2)
	name = parts[0]
	if len(parts) == 2 {
		value = parts[1]
	}
	return name, value
}

func disablesSiteIsolation(value string) bool {
	for _, feature := range strings.Split(strings.ToLower(value), ",") {
		feature = strings.TrimSpace(feature)
		switch feature {
		case "siteperprocess", "site-per-process", "isolateorigins", "isolate-origins":
			return true
		}
	}
	return false
}

func chromeExtraFlagsError(flag, message string) error {
	return ValidationError{
		Field:   "browser.extraFlags",
		Message: fmt.Sprintf("flag %q %s", flag, message),
	}
}

var reservedChromeExtraFlagFields = map[string]string{
	"--disable-automation":                      "the built-in stealth launch contract",
	"--disable-blink-features":                  "the built-in stealth launch contract",
	"--enable-automation":                       "the built-in stealth launch contract",
	"--enable-network-information-downlink-max": "the built-in stealth launch contract",
	"--headless":                                "instanceDefaults.mode",
	"--load-extension":                          "browser.extensionPaths",
	"--disable-extensions-except":               "browser.extensionPaths",
	"--remote-debugging-address":                "browser.remoteDebuggingPort",
	"--remote-debugging-pipe":                   "browser.remoteDebuggingPort",
	"--remote-debugging-port":                   "browser.remoteDebuggingPort",
	"--tz":                                      "instanceDefaults.timezone",
	"--user-agent":                              "instanceDefaults.userAgent",
	"--user-data-dir":                           "profiles.baseDir / profiles.defaultProfile",
	"--window-size":                             "the built-in runtime window model",
}

var disallowedChromeExtraFlags = map[string]string{
	"--allow-running-insecure-content":      "weakens browser security and is not allowed",
	"--disable-site-isolation-for-policy":   "weakens browser security and is not allowed",
	"--disable-site-isolation-trials":       "weakens browser security and is not allowed",
	"--disable-web-security":                "weakens browser security and is not allowed",
	"--ignore-certificate-errors":           "weakens TLS validation and is not allowed",
	"--ignore-certificate-errors-spki-list": "weakens TLS validation and is not allowed",
	"--no-sandbox":                          fmt.Sprintf("is not allowed in browser.extraFlags; PinchTab enables it only through runtime compatibility when needed, or via %s=1 for container compatibility", chromeNoSandboxEnvVar),
}
