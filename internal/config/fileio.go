package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// applyConfigPerms enforces 0600 on the config file and 0700 on its parent
// directory. It only chmods when current perms differ from the target, so
// repeat calls on already-tight perms are no-ops and safe on directories
// the process doesn't own (e.g. /tmp).
//
// Strictness is the caller's choice. SaveFileConfig bubbles errors because
// it just wrote secrets and a dir we can't tighten is a real problem. The
// load path swallows the return: chmod can fail on read-only FS, foreign-
// owned files, or filesystems that don't honor unix perms, none of which
// should block reading config.
func applyConfigPerms(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm() != 0600 {
		if err := os.Chmod(path, 0600); err != nil {
			return fmt.Errorf("failed to set config file permissions: %w", err)
		}
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err == nil && fi.Mode().Perm() != 0700 {
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("failed to set config directory permissions: %w", err)
		}
	}
	return nil
}

// tightenConfigPermsOnRead narrows a leaky config on the read path but never touches a
// file the user made read-only.
//
// Tightening 0644 to 0600 is worth doing: the file holds an auth token. Doing it
// unconditionally was not, because it also restored the owner-write bit on a 0444 file —
// which is precisely why a later write against a config the user had protected
// SUCCEEDED instead of failing, replacing it silently and handing it back writable. The
// absence of owner-write is a deliberate signal, so it is the one mode this leaves
// alone; a genuinely leaky file is still narrowed, and the write paths tighten on their
// own after they write.
func tightenConfigPermsOnRead(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0200 == 0 {
		return nil
	}
	return applyConfigPerms(path)
}

// savedConfigBaseline is what a file's ABSENT keys mean: the shipped defaults, with
// ConfigVersion zeroed because an absent configVersion is a first-install signal that
// NeedsWizard reads — not the current version.
//
// One owner on purpose, shared by the load path and by the minimal-diff save. Using
// DefaultFileConfig() directly as the save baseline suppressed exactly the key the
// startup write exists to add: configVersion equals the shipped value, so the stamp was
// treated as "same as default, not on disk, skip" and never landed, leaving the wizard
// to re-run on every single start.
func savedConfigBaseline() *FileConfig {
	defaults := DefaultFileConfig()
	defaults.ConfigVersion = ""
	defaults.Security.IDPI = nil
	return &defaults
}

// LoadFileConfig loads a FileConfig from the default or specified path.
// Returns the config and the path it was loaded from.
func LoadFileConfig() (*FileConfig, string, error) {
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No file on disk — the effective config is the defaults. Returning
			// a populated DefaultFileConfig() (rather than a zero FileConfig{})
			// means callers that subsequently SaveFileConfig don't write a
			// half-empty file with all-zero security/instance fields, which
			// previously caused IDPI and other defaults to be silently disabled
			// on first-run auto-config. ConfigVersion is reset so NeedsWizard()
			// still treats this as a first-install state.
			return savedConfigBaseline(), configPath, nil
		}
		return nil, configPath, fmt.Errorf("failed to read config file: %w", err)
	}

	_ = tightenConfigPermsOnRead(configPath)

	if isLegacyConfig(data) {
		fc, err := loadLegacyFileConfig(data)
		return fc, configPath, err
	}

	fc := savedConfigBaseline()
	if err := json.Unmarshal(data, fc); err != nil {
		return nil, configPath, fmt.Errorf("failed to parse config: %w", err)
	}
	NormalizeFileConfigAliasesFromJSON(fc, data)

	return fc, configPath, nil
}

// SaveFileConfig saves a FileConfig to the specified path, writing only the keys the
// file already had plus the ones that actually changed.
//
// Marshalling the struct wholesale was the amplifier behind a whole class of damage:
// LoadFileConfig unmarshals the user's file ON TOP OF DefaultFileConfig(), so the
// in-memory FileConfig is always fully populated, and any writer then materialised
// every default into the user's file. A 50-byte config became 3.8kB, host-absolute
// paths were baked in, and — worst — every untouched setting became explicitly set, so
// a shipped default change could never reach that install again.
//
// So the shape of the file on disk is authoritative for which keys exist, and this
// function only ever adds a key when its value differs from the shipped default.
func SaveFileConfig(fc *FileConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config file before writing: %w", err)
	}

	// Creating a config and updating one are different acts. With nothing on disk there
	// is no user shape to respect and the caller is `init` or the daemon's ensure-config,
	// whose whole job is to lay down a complete starter file — schema URL, bind, the
	// works. Only an UPDATE has a file whose shape is authoritative, and that is where
	// materialising defaults did the damage.
	var data []byte
	if len(existing) == 0 {
		if data, err = json.MarshalIndent(fc, "", "  "); err != nil {
			return fmt.Errorf("failed to serialize config: %w", err)
		}
		data = append(data, '\n')
	} else if data, err = renderMinimalConfig(fc, existing); err != nil {
		return err
	}

	// A save that changes nothing does not touch the file. This is what makes a
	// no-op write byte-identical by construction rather than by matching the user's
	// formatting: an inline array or a hand-wrapped section is only ever re-rendered
	// when something in it actually changed. It also means a read-only config raises
	// no error for a write that had nothing to say.
	if len(existing) > 0 && sameJSONDocument(existing, data) {
		return nil
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return applyConfigPerms(path)
}

func configAsMap(fc *FileConfig) (map[string]any, error) {
	raw, err := json.Marshal(fc)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize config: %w", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("failed to serialize config: %w", err)
	}
	return out, nil
}

func sameJSONValue(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	if errLeft != nil || errRight != nil {
		return false
	}
	return string(left) == string(right)
}

func loadLegacyFileConfig(data []byte) (*FileConfig, error) {
	var lc legacyFileConfig
	if err := json.Unmarshal(data, &lc); err != nil {
		return nil, fmt.Errorf("failed to parse legacy config: %w", err)
	}

	defaults := DefaultFileConfig()
	fc := &defaults
	legacy := convertLegacyConfig(&lc)
	if legacy.Server.Port != "" {
		fc.Server.Port = legacy.Server.Port
	}
	if legacy.Server.Token != "" {
		fc.Server.Token = legacy.Server.Token
	}
	if legacy.Server.StateDir != "" {
		fc.Server.StateDir = legacy.Server.StateDir
	}
	if legacy.InstanceDefaults.Mode != "" {
		fc.InstanceDefaults.Mode = legacy.InstanceDefaults.Mode
	}
	if legacy.InstanceDefaults.NoRestore != nil {
		fc.InstanceDefaults.NoRestore = legacy.InstanceDefaults.NoRestore
	}
	if legacy.InstanceDefaults.MaxTabs != nil {
		fc.InstanceDefaults.MaxTabs = legacy.InstanceDefaults.MaxTabs
	}
	if legacy.Profiles.BaseDir != "" {
		fc.Profiles.BaseDir = legacy.Profiles.BaseDir
	}
	if legacy.Profiles.DefaultProfile != "" {
		fc.Profiles.DefaultProfile = legacy.Profiles.DefaultProfile
	}
	if legacy.Security.AllowEvaluate != nil {
		fc.Security.AllowEvaluate = legacy.Security.AllowEvaluate
	}
	if legacy.Security.AllowMacro != nil {
		fc.Security.AllowMacro = legacy.Security.AllowMacro
	}
	if legacy.Security.AllowScreencast != nil {
		fc.Security.AllowScreencast = legacy.Security.AllowScreencast
	}
	if legacy.Security.AllowDownload != nil {
		fc.Security.AllowDownload = legacy.Security.AllowDownload
	}
	if legacy.Security.AllowCookies != nil {
		fc.Security.AllowCookies = legacy.Security.AllowCookies
	}
	if legacy.Security.AllowUpload != nil {
		fc.Security.AllowUpload = legacy.Security.AllowUpload
	}
	if legacy.Timeouts.ActionSec != 0 {
		fc.Timeouts.ActionSec = legacy.Timeouts.ActionSec
	}
	if legacy.Timeouts.NavigateSec != 0 {
		fc.Timeouts.NavigateSec = legacy.Timeouts.NavigateSec
	}
	if legacy.MultiInstance.InstancePortStart != nil {
		fc.MultiInstance.InstancePortStart = legacy.MultiInstance.InstancePortStart
	}
	if legacy.MultiInstance.InstancePortEnd != nil {
		fc.MultiInstance.InstancePortEnd = legacy.MultiInstance.InstancePortEnd
	}

	return fc, nil
}

// aliasRaw* are the config keys this normaliser accepts. They are package types
// rather than function-local ones so the unknown-key walk can derive its
// exemptions from them: security.idpi.allowedDomains is a supported alias that
// no FileConfig field declares, and a strict pass reports it as a typo unless
// the exemption comes from this same declaration.
type aliasRawIDPI struct {
	AllowedDomains *[]string `json:"allowedDomains"`
}

type aliasRawSecurity struct {
	AllowedDomains *[]string     `json:"allowedDomains"`
	IDPI           *aliasRawIDPI `json:"idpi"`
}

type aliasRawConfig struct {
	Security *aliasRawSecurity `json:"security"`
}

func NormalizeFileConfigAliasesFromJSON(fc *FileConfig, data []byte) {
	if fc == nil {
		return
	}

	var raw aliasRawConfig
	if err := json.Unmarshal(data, &raw); err != nil || raw.Security == nil {
		return
	}

	switch {
	case raw.Security.AllowedDomains != nil:
		fc.Security.AllowedDomains = append([]string(nil), (*raw.Security.AllowedDomains)...)
	case raw.Security.IDPI != nil && raw.Security.IDPI.AllowedDomains != nil:
		fc.Security.AllowedDomains = append([]string(nil), (*raw.Security.IDPI.AllowedDomains)...)
	}
}
