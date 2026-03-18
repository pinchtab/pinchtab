package workflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

func ApplyRecommendedSecurityDefaults(fc *config.FileConfig) {
	defaults := config.DefaultFileConfig()
	if fc == nil {
		return
	}
	fc.Server.Bind = defaults.Server.Bind
	fc.Security = defaults.Security
	_, _ = config.EnsureFileToken(fc)
}

func RestoreSecurityDefaults() (string, bool, error) {
	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return "", false, err
	}
	before := securityDefaultsSnapshot(fc)
	ApplyRecommendedSecurityDefaults(fc)
	after := securityDefaultsSnapshot(fc)
	if reflect.DeepEqual(before, after) {
		return configPath, false, nil
	}
	if err := config.SaveFileConfig(fc, configPath); err != nil {
		return "", false, err
	}
	return configPath, true, nil
}

func UpdateSensitiveEndpoints(value string) (*config.RuntimeConfig, bool, error) {
	change, err := prepareChange(func(fc *config.FileConfig) error {
		selected := map[string]bool{}
		for _, item := range splitCommaList(value) {
			selected[item] = true
		}
		for endpoint, path := range map[string]string{
			"evaluate":   "security.allowEvaluate",
			"macro":      "security.allowMacro",
			"screencast": "security.allowScreencast",
			"download":   "security.allowDownload",
			"upload":     "security.allowUpload",
		} {
			if err := config.SetConfigValue(fc, path, fmt.Sprintf("%t", selected[endpoint])); err != nil {
				return fmt.Errorf("set %s: %w", endpoint, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(change.ValidationErrors) > 0 {
		return nil, false, change.ValidationErrors[0]
	}
	if err := SavePreparedChange(change); err != nil {
		return nil, false, err
	}
	return config.Load(), true, nil
}

func UpdateContentGuard(mode string) (*config.RuntimeConfig, bool, error) {
	change, err := prepareChange(func(fc *config.FileConfig) error {
		scan := mode == "both" || mode == "scan"
		wrap := mode == "both" || mode == "wrap"
		for _, item := range []struct {
			path  string
			value bool
		}{
			{path: "security.idpi.scanContent", value: scan},
			{path: "security.idpi.wrapContent", value: wrap},
		} {
			if err := config.SetConfigValue(fc, item.path, fmt.Sprintf("%t", item.value)); err != nil {
				return fmt.Errorf("set %s: %w", item.path, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(change.ValidationErrors) > 0 {
		return nil, false, change.ValidationErrors[0]
	}
	if err := SavePreparedChange(change); err != nil {
		return nil, false, err
	}
	return config.Load(), true, nil
}

func ApplyGuardsDownPreset() (*config.RuntimeConfig, string, bool, error) {
	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return nil, "", false, fmt.Errorf("load config: %w", err)
	}
	originalJSON, err := formatFileConfigJSON(fc)
	if err != nil {
		return nil, "", false, err
	}

	original, err := config.GetConfigValue(fc, "server.token")
	if err != nil {
		return nil, "", false, fmt.Errorf("read server.token: %w", err)
	}
	if strings.TrimSpace(original) == "" {
		token, err := config.GenerateAuthToken()
		if err != nil {
			return nil, "", false, fmt.Errorf("generate token: %w", err)
		}
		if err := config.SetConfigValue(fc, "server.token", token); err != nil {
			return nil, "", false, fmt.Errorf("set server.token: %w", err)
		}
	}

	for _, item := range []struct {
		path  string
		value string
	}{
		{path: "server.bind", value: "127.0.0.1"},
		{path: "security.allowEvaluate", value: "true"},
		{path: "security.allowMacro", value: "true"},
		{path: "security.allowScreencast", value: "true"},
		{path: "security.allowDownload", value: "true"},
		{path: "security.allowUpload", value: "true"},
		{path: "security.attach.enabled", value: "true"},
		{path: "security.attach.allowHosts", value: "127.0.0.1,localhost,::1"},
		{path: "security.attach.allowSchemes", value: "ws,wss"},
		{path: "security.idpi.enabled", value: "false"},
		{path: "security.idpi.strictMode", value: "false"},
		{path: "security.idpi.scanContent", value: "false"},
		{path: "security.idpi.wrapContent", value: "false"},
	} {
		if err := config.SetConfigValue(fc, item.path, item.value); err != nil {
			return nil, "", false, fmt.Errorf("set %s: %w", item.path, err)
		}
	}

	if errs := config.ValidateFileConfig(fc); len(errs) > 0 {
		return nil, "", false, errs[0]
	}

	nextJSON, err := formatFileConfigJSON(fc)
	if err != nil {
		return nil, "", false, err
	}
	changed := originalJSON != nextJSON
	if !changed {
		return config.Load(), configPath, false, nil
	}

	if err := config.SaveFileConfig(fc, configPath); err != nil {
		return nil, "", false, fmt.Errorf("save config: %w", err)
	}
	return config.Load(), configPath, true, nil
}

func formatFileConfigJSON(fc *config.FileConfig) (string, error) {
	data, err := json.Marshal(fc)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(data), nil
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(part))
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	slices.Sort(items)
	return slices.Compact(items)
}

type securityDefaultsState struct {
	Bind     string
	Token    string
	Security securityConfigValues
}

type securityConfigValues struct {
	AllowEvaluate         bool
	AllowMacro            bool
	AllowScreencast       bool
	AllowDownload         bool
	DownloadMaxBytes      int
	AllowUpload           bool
	UploadMaxRequestBytes int
	UploadMaxFiles        int
	UploadMaxFileBytes    int
	UploadMaxTotalBytes   int
	MaxRedirects          int
	AttachEnabled         bool
	IDPI                  config.IDPIConfig
}

func securityDefaultsSnapshot(fc *config.FileConfig) securityDefaultsState {
	if fc == nil {
		return securityDefaultsState{}
	}
	s := securityDefaultsState{
		Bind:  fc.Server.Bind,
		Token: fc.Server.Token,
		Security: securityConfigValues{
			IDPI: fc.Security.IDPI,
		},
	}
	if fc.Security.AllowEvaluate != nil {
		s.Security.AllowEvaluate = *fc.Security.AllowEvaluate
	}
	if fc.Security.AllowMacro != nil {
		s.Security.AllowMacro = *fc.Security.AllowMacro
	}
	if fc.Security.AllowScreencast != nil {
		s.Security.AllowScreencast = *fc.Security.AllowScreencast
	}
	if fc.Security.AllowDownload != nil {
		s.Security.AllowDownload = *fc.Security.AllowDownload
	}
	if fc.Security.DownloadMaxBytes != nil {
		s.Security.DownloadMaxBytes = *fc.Security.DownloadMaxBytes
	}
	if fc.Security.AllowUpload != nil {
		s.Security.AllowUpload = *fc.Security.AllowUpload
	}
	if fc.Security.UploadMaxRequestBytes != nil {
		s.Security.UploadMaxRequestBytes = *fc.Security.UploadMaxRequestBytes
	}
	if fc.Security.UploadMaxFiles != nil {
		s.Security.UploadMaxFiles = *fc.Security.UploadMaxFiles
	}
	if fc.Security.UploadMaxFileBytes != nil {
		s.Security.UploadMaxFileBytes = *fc.Security.UploadMaxFileBytes
	}
	if fc.Security.UploadMaxTotalBytes != nil {
		s.Security.UploadMaxTotalBytes = *fc.Security.UploadMaxTotalBytes
	}
	if fc.Security.MaxRedirects != nil {
		s.Security.MaxRedirects = *fc.Security.MaxRedirects
	}
	if fc.Security.Attach.Enabled != nil {
		s.Security.AttachEnabled = *fc.Security.Attach.Enabled
	}
	return s
}
