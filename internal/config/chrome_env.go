package config

// chromeNoSandboxEnvVar is the environment variable that forces Chrome's
// --no-sandbox runtime-compatibility flag. It is read by the bridge and must
// be re-forwarded to per-instance child processes by the orchestrator, which
// otherwise strips every PINCHTAB_-prefixed var from the child environment.
const chromeNoSandboxEnvVar = "PINCHTAB_CHROME_NO_SANDBOX"

// ChromeNoSandboxEnvVar returns the no-sandbox compatibility env var name.
func ChromeNoSandboxEnvVar() string { return chromeNoSandboxEnvVar }
