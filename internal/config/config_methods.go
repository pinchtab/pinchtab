package config

// EnabledSensitiveEndpoints returns the names of sensitive endpoint families
// that are currently enabled in the runtime configuration.
func (cfg *RuntimeConfig) EnabledSensitiveEndpoints() []string {
	if cfg == nil {
		return nil
	}

	enabled := make([]string, 0, 5)
	if cfg.AllowEvaluate {
		enabled = append(enabled, "evaluate")
	}
	if cfg.AllowMacro {
		enabled = append(enabled, "macro")
	}
	if cfg.AllowScreencast {
		enabled = append(enabled, "screencast")
	}
	if cfg.AllowDownload {
		enabled = append(enabled, "download")
	}
	if cfg.AllowUpload {
		enabled = append(enabled, "upload")
	}
	return enabled
}

// ActivityStateDir returns the directory root used for activity log storage.
// When unset, activity logs live under the main server state directory.
func (cfg *RuntimeConfig) ActivityStateDir() string {
	if cfg == nil {
		return ""
	}
	return cfg.StateDir
}
