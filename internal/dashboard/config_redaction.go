package dashboard

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

func (c *ConfigAPI) tokenConfigured(cfg config.FileConfig) bool {
	if c != nil && c.runtime != nil && strings.TrimSpace(c.runtime.Token) != "" {
		return true
	}
	return strings.TrimSpace(cfg.Server.Token) != ""
}

func redactToken(cfg config.FileConfig) config.FileConfig {
	cfg.Server.Token = ""
	cfg.Security.StateEncryptionKey = nil
	cfg.AutoSolver.External.CapsolverKey = ""
	cfg.AutoSolver.External.TwoCaptchaKey = ""
	cfg.AutoSolver.Credentials = config.AutoSolverCredentialsConf{}
	cfg.Browser.Proxy = cfg.Browser.Proxy.Redacted()
	if len(cfg.Browser.Targets) > 0 {
		// Copy before mutating: maps are reference types.
		copied := make(config.BrowserTargetsConfig, len(cfg.Browser.Targets))
		for name, t := range cfg.Browser.Targets {
			t.Proxy = t.Proxy.Redacted()
			copied[name] = t
		}
		cfg.Browser.Targets = copied
	}
	return cfg
}

func preserveWriteOnlyConfigFields(dst, src *config.FileConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Server.Token = src.Server.Token
	dst.Security.StateEncryptionKey = src.Security.StateEncryptionKey
	dst.AutoSolver.External.CapsolverKey = src.AutoSolver.External.CapsolverKey
	dst.AutoSolver.External.TwoCaptchaKey = src.AutoSolver.External.TwoCaptchaKey
	// Credentials are write-only: a blank or omitted credential field — which is
	// what GET echoes back, having redacted them — keeps the value already on disk.
	// A blank field does NOT clear a credential via the dashboard: missing and
	// deliberately-empty are indistinguishable after JSON decode, so for
	// credentials the safe choice is to preserve. Clear by editing the config file
	// directly. (See preserveCredString.)
	preserveCredString(&dst.AutoSolver.Credentials.Login.User, src.AutoSolver.Credentials.Login.User)
	preserveCredString(&dst.AutoSolver.Credentials.Login.Password, src.AutoSolver.Credentials.Login.Password)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Name, src.AutoSolver.Credentials.Signup.Name)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Email, src.AutoSolver.Credentials.Signup.Email)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Password, src.AutoSolver.Credentials.Signup.Password)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Field1, src.AutoSolver.Credentials.Form.Field1)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Field2, src.AutoSolver.Credentials.Form.Field2)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Email, src.AutoSolver.Credentials.Form.Email)

	// Restore the on-disk proxy password when the dashboard echoes back the redaction mask.
	preserveProxyPassword(&dst.Browser.Proxy, src.Browser.Proxy)
	for name, t := range dst.Browser.Targets {
		if srcT, ok := src.Browser.Targets[name]; ok {
			preserveProxyPassword(&t.Proxy, srcT.Proxy)
			dst.Browser.Targets[name] = t
		}
	}
}

// preserveProxyPassword keeps the on-disk password when the inbound PUT is blank or the "***" mask.
func preserveProxyPassword(dst *config.BrowserProxyConfig, src config.BrowserProxyConfig) {
	if dst == nil {
		return
	}
	if dst.Password == "" || dst.Password == "***" {
		dst.Password = src.Password
	}
}

// preserveCredString keeps the existing src value when dst is empty (i.e. the
// PUT didn't include this field because GET redacted it). A deliberate
// blank from PUT is indistinguishable from "not provided" in JSON without
// pointer types — given these are credentials, the safer default is to
// preserve. Callers can clear by writing the JSON file directly.
func preserveCredString(dst *string, src string) {
	if dst == nil {
		return
	}
	if *dst == "" {
		*dst = src
	}
}
