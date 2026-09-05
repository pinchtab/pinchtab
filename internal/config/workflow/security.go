package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// SettingChange is one config key a preset writes, as the file sees it: Old and
// New are the key's compact JSON values, "" meaning the key is absent. The token
// never travels in either; it is reported as generated or changed.
type SettingChange struct {
	Path string
	Old  string
	New  string
}

// PresetResult is what a preset did or, under dry run, would do. Changes is the
// diff between the file on disk and the file the preset renders, so it names
// exactly the keys the file gains, loses or changes; Written is false when the
// diff was empty or the run was a preview.
type PresetResult struct {
	ConfigPath string
	Changes    []SettingChange
	Written    bool
}

func (r PresetResult) Changed() bool { return len(r.Changes) > 0 }

// setting is one config key a preset writes by name; everything a preset does
// not name survives untouched, by construction.
type setting struct {
	path  string
	value string
}

func applySettings(fc *config.FileConfig, settings []setting) error {
	for _, item := range settings {
		if err := config.SetConfigValue(fc, item.path, item.value); err != nil {
			return fmt.Errorf("set %s: %w", item.path, err)
		}
	}
	return nil
}

// capabilitySettings names every gated capability's setting except the ones in
// skip, so both presets derive the capability half from routes rather than
// transcribing it.
func capabilitySettings(value string, skip ...routes.Capability) ([]setting, error) {
	settings := make([]setting, 0)
	for cap := range routes.CapabilityEndpoints() {
		if slices.Contains(skip, cap) {
			continue
		}
		meta, ok := routes.Meta(cap)
		if !ok {
			return nil, fmt.Errorf("capability %q gates routes but routes.Meta does not describe it", cap)
		}
		settings = append(settings, setting{path: meta.Setting, value: value})
	}
	sort.Slice(settings, func(i, j int) bool { return settings[i].path < settings[j].path })
	return settings, nil
}

// recommendedSecuritySettings is the whole of what security up writes: the
// loopback bind, every capability off, attach off and local-only, and the IDPI
// flags on, each valued from DefaultFileConfig. Allowlists, CIDR lists, custom
// patterns, tuned thresholds and the state encryption key are not named here
// and so are preserved.
func recommendedSecuritySettings() ([]setting, error) {
	defaults := config.DefaultFileConfig()
	idpi := defaults.Security.EffectiveIDPI()
	settings := []setting{{path: "server.bind", value: defaults.Server.Bind}}
	capabilities, err := capabilitySettings("false")
	if err != nil {
		return nil, err
	}
	settings = append(settings, capabilities...)
	return append(settings,
		setting{path: "security.attach.enabled", value: strconv.FormatBool(*defaults.Security.Attach.Enabled)},
		setting{path: "security.attach.allowHosts", value: strings.Join(defaults.Security.Attach.AllowHosts, ",")},
		setting{path: "security.idpi.enabled", value: strconv.FormatBool(idpi.Enabled)},
		setting{path: "security.idpi.strictMode", value: strconv.FormatBool(idpi.StrictMode)},
		setting{path: "security.idpi.scanContent", value: strconv.FormatBool(idpi.ScanContent)},
		setting{path: "security.idpi.wrapContent", value: strconv.FormatBool(idpi.WrapContent)},
	), nil
}

func ApplyRecommendedSecurityDefaults(fc *config.FileConfig) error {
	if fc == nil {
		return fmt.Errorf("nil file config")
	}
	settings, err := recommendedSecuritySettings()
	if err != nil {
		return err
	}
	return applySettings(fc, settings)
}

func RestoreSecurityDefaults(dryRun bool) (PresetResult, error) {
	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return PresetResult{}, err
	}
	if err := ApplyRecommendedSecurityDefaults(fc); err != nil {
		return PresetResult{}, err
	}
	if _, err := config.ProvisionFileToken(fc, configPath); err != nil {
		return PresetResult{}, err
	}
	return commitPreset(fc, configPath, dryRun)
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

// guardsDownExcludedCapability is the one capability the guards-down preset does not
// bulk-enable: stateExport writes cookies and browser storage to disk, and enabling
// disk export as a side effect of "turn the guards off for local dev" is a surprise
// no other capability carries. The census in this package's tests holds this exclusion
// so a ninth capability is enabled by default while this one stays a deliberate opt-out.
const guardsDownExcludedCapability = routes.CapStateExport

// BuildGuardsDownConfig mutates fc in memory to apply the guards-down preset.
// It does not persist anything.
func BuildGuardsDownConfig(fc *config.FileConfig) error {
	if fc == nil {
		return fmt.Errorf("nil file config")
	}
	if _, err := config.EnsureFileToken(fc); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	capabilities, err := capabilitySettings("true", guardsDownExcludedCapability)
	if err != nil {
		return err
	}
	if err := applySettings(fc, append(capabilities,
		setting{path: "server.bind", value: "127.0.0.1"},
		setting{path: "security.attach.enabled", value: "true"},
		setting{path: "security.attach.allowHosts", value: "127.0.0.1,localhost,::1"},
		setting{path: "security.attach.allowSchemes", value: "ws,wss"},
		setting{path: "security.idpi.enabled", value: "false"},
		setting{path: "security.idpi.strictMode", value: "false"},
		setting{path: "security.idpi.scanContent", value: "false"},
		setting{path: "security.idpi.wrapContent", value: "false"},
	)); err != nil {
		return err
	}

	if errs := config.ValidateFileConfig(fc); len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func GuardsDownPostureActive(cfg *config.RuntimeConfig) bool {
	if cfg == nil {
		return false
	}
	for cap := range routes.CapabilityEndpoints() {
		if cap == guardsDownExcludedCapability {
			continue
		}
		if !cfg.CapabilityEnabled(cap) {
			return false
		}
	}
	return cfg.AttachEnabled && !cfg.IDPI.Enabled
}

func ApplyGuardsDownPreset(dryRun bool) (PresetResult, error) {
	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return PresetResult{}, fmt.Errorf("load config: %w", err)
	}
	if err := BuildGuardsDownConfig(fc); err != nil {
		return PresetResult{}, err
	}
	return commitPreset(fc, configPath, dryRun)
}

// commitPreset is the one diff both presets share: the file on disk against the
// bytes the save would write. Under dryRun the diff is the whole result.
func commitPreset(fc *config.FileConfig, configPath string, dryRun bool) (PresetResult, error) {
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return PresetResult{}, fmt.Errorf("read config: %w", err)
	}
	next, err := config.RenderFileConfig(fc, existing)
	if err != nil {
		return PresetResult{}, err
	}
	changes, err := settingChanges(existing, next)
	if err != nil {
		return PresetResult{}, err
	}
	result := PresetResult{ConfigPath: configPath, Changes: changes}
	if dryRun || len(changes) == 0 {
		return result, nil
	}
	if err := config.SaveFileConfig(fc, configPath); err != nil {
		return PresetResult{}, fmt.Errorf("save config: %w", err)
	}
	result.Written = true
	return result, nil
}

const tokenSetting = "server.token"

func settingChanges(before, after []byte) ([]SettingChange, error) {
	old, err := flattenConfigJSON(before)
	if err != nil {
		return nil, fmt.Errorf("parse config on disk: %w", err)
	}
	next, err := flattenConfigJSON(after)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(next))
	for path := range old {
		paths = append(paths, path)
	}
	for path := range next {
		if _, seen := old[path]; !seen {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	changes := make([]SettingChange, 0)
	for _, path := range paths {
		if old[path] == next[path] {
			continue
		}
		changes = append(changes, redactToken(SettingChange{Path: path, Old: old[path], New: next[path]}))
	}
	return changes, nil
}

func redactToken(c SettingChange) SettingChange {
	if c.Path != tokenSetting {
		return c
	}
	if c.Old == `""` {
		c.Old = ""
	}
	if c.Old != "" {
		c.Old = "<set>"
	}
	if c.New != "" {
		c.New = "<generated>"
	}
	return c
}

// flattenConfigJSON maps every leaf of a config document to its dotted path,
// with the leaf's compact JSON as the value; an empty document has no leaves.
func flattenConfigJSON(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(bytes.TrimSpace(data)) == 0 {
		return out, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		if object, ok := value.(map[string]any); ok && len(object) > 0 {
			for key, child := range object {
				walk(strings.TrimPrefix(prefix+"."+key, "."), child)
			}
			return
		}
		raw, _ := json.Marshal(value)
		out[prefix] = string(raw)
	}
	walk("", doc)
	return out, nil
}
