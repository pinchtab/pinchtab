package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/safelog"
)

// GetConfigValue reads a dotted-path field from FileConfig and returns its string representation.
// Pointer fields that are unset return an empty string. Slice fields are comma-separated.
// The section vocabulary lives once, in configSections.
func GetConfigValue(fc *FileConfig, path string) (string, error) {
	section, field, err := lookupConfigSection(path)
	if err != nil {
		return "", err
	}
	return section.get(fc, field)
}

func getBrowserTargetField(targets BrowserTargetsConfig, at, path string) (string, error) {
	name, field, ok := strings.Cut(path, ".")
	if !ok || name == "" || field == "" {
		return "", fmt.Errorf("invalid browser.targets path %q (expected targets.<name>.<field>)", path)
	}
	t, ok := targets[name]
	if !ok {
		return "", fmt.Errorf("browser target %q not found", name)
	}
	return browserTargetFields.get(&t, at+"."+name, field)
}

func formatBoolPtr(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "true"
	}
	return "false"
}

func formatStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatIntPtr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

// EffectiveConfigValue answers with the value in effect for a dotted path: the file
// value when the file sets one, otherwise the value the runtime resolves. Keys whose
// effective value is derived from another key, or whose default is applied by a
// consumer rather than stored in the config struct, are only visible this way.
func EffectiveConfigValue(path string) (string, error) {
	fc, _, err := LoadFileConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	value, err := GetConfigValue(fc, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) != "" && !runtimeSettlesTheValue(path) {
		return value, nil
	}
	return resolvedConfigValue(path)
}

// runtimeSettlesTheValue marks the paths whose file entry is an input rather than the
// answer, so reading the file would report something nothing acts on:
//
//	scheduler.*                      a configured zero means "use the default" here
//	observability.activity.stateDir  the path derives from server.stateDir; a file value
//	                                 is refused at config set and warned about at load
func runtimeSettlesTheValue(path string) bool {
	return strings.HasPrefix(path, "scheduler.") || path == "observability.activity.stateDir"
}

func resolvedConfigValue(path string) (string, error) {
	cfg, _, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	resolved := FileConfigFromRuntime(cfg)
	addSettledDefaults(&resolved, cfg)
	return GetConfigValue(&resolved, path)
}

// addSettledDefaults fills the sections FileConfigFromRuntime leaves out on purpose:
// it serialises configs for writing, where emitting every default would add noise to
// a file the operator did not ask for. The reader needs the opposite, so it takes the
// values back off the RuntimeConfig that has already settled them — never from a
// literal spelled here, which would be a second copy free to drift.
func addSettledDefaults(resolved *FileConfig, cfg *RuntimeConfig) {
	if strings.TrimSpace(resolved.Server.LogLevel) == "" {
		resolved.Server.LogLevel = safelog.LevelName(safelog.DefaultLevel)
	}
	// The writer omits a false tab-restore (a vanilla tab policy is left out of a file the
	// operator did not ask for) and omits the state encryption key entirely, so a generated
	// child instance config never carries a secret. Both are settled values a reader has to
	// see, taken off the RuntimeConfig rather than restated here.
	if resolved.InstanceDefaults.TabPolicy == nil {
		resolved.InstanceDefaults.TabPolicy = &TabPolicyDefaults{}
	}
	if resolved.InstanceDefaults.TabPolicy.Restore == nil {
		resolved.InstanceDefaults.TabPolicy.Restore = boolPtrValue(cfg.TabRestore)
	}
	if resolved.Security.StateEncryptionKey == nil && cfg.StateEncryptionKey != "" {
		key := cfg.StateEncryptionKey
		resolved.Security.StateEncryptionKey = &key
	}
	resolved.Scheduler = SchedulerFileConfig{
		Enabled:           boolPtrValue(cfg.Scheduler.Enabled),
		Strategy:          cfg.Scheduler.Strategy,
		MaxQueueSize:      intPtrIfPositive(cfg.Scheduler.MaxQueueSize),
		MaxPerAgent:       intPtrIfPositive(cfg.Scheduler.MaxPerAgent),
		MaxInflight:       intPtrIfPositive(cfg.Scheduler.MaxInflight),
		MaxPerAgentFlight: intPtrIfPositive(cfg.Scheduler.MaxPerAgentFlight),
		ResultTTLSec:      intPtrIfPositive(cfg.Scheduler.ResultTTLSec),
		WorkerCount:       intPtrIfPositive(cfg.Scheduler.WorkerCount),
		MaxBatchSize:      intPtrIfPositive(cfg.Scheduler.MaxBatchSize),
	}
	if resolved.InstanceDefaults.TabPolicy == nil {
		resolved.InstanceDefaults.TabPolicy = &TabPolicyDefaults{}
	}
	tabPolicy := resolved.InstanceDefaults.TabPolicy
	if tabPolicy.Eviction == "" {
		tabPolicy.Eviction = cfg.TabEvictionPolicy
	}
	if tabPolicy.Lifecycle == "" {
		tabPolicy.Lifecycle = cfg.TabLifecyclePolicy
	}
	if tabPolicy.CloseDelaySec == nil {
		tabPolicy.CloseDelaySec = intPtrIfPositive(int(cfg.TabCloseDelay / time.Second))
	}
	if strings.TrimSpace(resolved.Observability.Activity.StateDir) == "" {
		resolved.Observability.Activity.StateDir = cfg.ActivityLogDir()
	}
}
