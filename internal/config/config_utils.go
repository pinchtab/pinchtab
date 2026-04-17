package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var defaultLocalAllowedDomains = []string{"127.0.0.1", "localhost", "::1"}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	h, _ := os.UserHomeDir()
	return h
}

func userConfigDir() string {
	home := homeDir()
	legacyPath := filepath.Join(home, ".pinchtab")

	// On macOS and Linux, keep config and state rooted in ~/.pinchtab so the
	// CLI, npm-managed binary, and config path all resolve to the same location.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		return legacyPath
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return legacyPath
	}

	return filepath.Join(configDir, "pinchtab")
}

// DefaultConfigPath returns the default config file location used when
// PINCHTAB_CONFIG is not explicitly set.
func DefaultConfigPath() string {
	return filepath.Join(userConfigDir(), "config.json")
}

func defaultExtensionsDir(baseDir string) string {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = userConfigDir()
	}
	return filepath.Join(baseDir, "extensions")
}

func (c *RuntimeConfig) ListenAddr() string {
	return c.Bind + ":" + c.Port
}

func GenerateAuthToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// EnsureFileToken guarantees that a persisted config carries a non-empty
// server token. It returns true when a new token was generated.
func EnsureFileToken(fc *FileConfig) (bool, error) {
	if fc == nil {
		return false, fmt.Errorf("file config is nil")
	}
	if strings.TrimSpace(fc.Server.Token) != "" {
		return false, nil
	}
	token, err := GenerateAuthToken()
	if err != nil {
		return false, err
	}
	fc.Server.Token = token
	return true, nil
}

func effectiveSecurityAllowedDomains(s SecurityConfig) []string {
	if len(s.AllowedDomains) > 0 {
		return append([]string(nil), s.AllowedDomains...)
	}
	if s.AllowedDomains != nil {
		return []string{}
	}
	return nil
}

func MaskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// NeedsWizard returns true if the config has no version or an older version than current.
func NeedsWizard(cfg *FileConfig) bool {
	if cfg.ConfigVersion == "" {
		return true
	}
	return CompareVersions(cfg.ConfigVersion, CurrentConfigVersion) < 0
}

// IsFirstRun returns true if the config has never been through the wizard.
func IsFirstRun(cfg *FileConfig) bool {
	return cfg.ConfigVersion == ""
}

// CompareVersions compares two semver-like version strings.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) int {
	aParts := splitVersion(a)
	bParts := splitVersion(b)
	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) [3]int {
	parts := [3]int{}
	segs := strings.SplitN(v, ".", 3)
	for i, s := range segs {
		if i >= 3 {
			break
		}
		n := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		parts[i] = n
	}
	return parts
}
