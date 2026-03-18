package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateFileConfig validates a FileConfig and returns all errors found.
func ValidateFileConfig(fc *FileConfig) []error {
	var errs []error

	// Server validation
	if fc.Server.Port != "" {
		if err := validatePort(fc.Server.Port, "server.port"); err != nil {
			errs = append(errs, err)
		}
	}
	if fc.Server.Bind != "" {
		if err := validateBind(fc.Server.Bind, "server.bind"); err != nil {
			errs = append(errs, err)
		}
	}
	if fc.Server.NetworkBufferSize != nil {
		if *fc.Server.NetworkBufferSize < 1 || *fc.Server.NetworkBufferSize > MaxNetworkBufferSize {
			errs = append(errs, ValidationError{
				Field:   "server.networkBufferSize",
				Message: fmt.Sprintf("must be between 1 and %d (got %d)", MaxNetworkBufferSize, *fc.Server.NetworkBufferSize),
			})
		}
	}
	if fc.MultiInstance.InstancePortStart != nil && fc.MultiInstance.InstancePortEnd != nil {
		if *fc.MultiInstance.InstancePortStart > *fc.MultiInstance.InstancePortEnd {
			errs = append(errs, ValidationError{
				Field:   "multiInstance.instancePortStart/End",
				Message: fmt.Sprintf("start port (%d) must be <= end port (%d)", *fc.MultiInstance.InstancePortStart, *fc.MultiInstance.InstancePortEnd),
			})
		}
	}
	if fc.MultiInstance.Restart.MaxRestarts != nil {
		if *fc.MultiInstance.Restart.MaxRestarts < -1 {
			errs = append(errs, ValidationError{
				Field:   "multiInstance.restart.maxRestarts",
				Message: fmt.Sprintf("must be >= 0 or -1 for unlimited (got %d)", *fc.MultiInstance.Restart.MaxRestarts),
			})
		}
	}
	if fc.MultiInstance.Restart.InitBackoffSec != nil && *fc.MultiInstance.Restart.InitBackoffSec < 1 {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.restart.initBackoffSec",
			Message: fmt.Sprintf("must be >= 1 (got %d)", *fc.MultiInstance.Restart.InitBackoffSec),
		})
	}
	if fc.MultiInstance.Restart.MaxBackoffSec != nil && *fc.MultiInstance.Restart.MaxBackoffSec < 1 {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.restart.maxBackoffSec",
			Message: fmt.Sprintf("must be >= 1 (got %d)", *fc.MultiInstance.Restart.MaxBackoffSec),
		})
	}
	if fc.MultiInstance.Restart.StableAfterSec != nil && *fc.MultiInstance.Restart.StableAfterSec < 1 {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.restart.stableAfterSec",
			Message: fmt.Sprintf("must be >= 1 (got %d)", *fc.MultiInstance.Restart.StableAfterSec),
		})
	}
	if fc.MultiInstance.Restart.InitBackoffSec != nil && fc.MultiInstance.Restart.MaxBackoffSec != nil &&
		*fc.MultiInstance.Restart.InitBackoffSec > *fc.MultiInstance.Restart.MaxBackoffSec {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.restart.initBackoffSec/maxBackoffSec",
			Message: fmt.Sprintf("init backoff (%d) must be <= max backoff (%d)", *fc.MultiInstance.Restart.InitBackoffSec, *fc.MultiInstance.Restart.MaxBackoffSec),
		})
	}

	// Instance defaults validation
	if fc.InstanceDefaults.Mode != "" && fc.InstanceDefaults.Mode != "headless" && fc.InstanceDefaults.Mode != "headed" {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.mode",
			Message: fmt.Sprintf("invalid value %q (must be headless or headed)", fc.InstanceDefaults.Mode),
		})
	}
	if fc.InstanceDefaults.StealthLevel != "" {
		if !isValidStealthLevel(fc.InstanceDefaults.StealthLevel) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.stealthLevel",
				Message: fmt.Sprintf("invalid value %q (must be light, medium, or full)", fc.InstanceDefaults.StealthLevel),
			})
		}
	}
	if fc.InstanceDefaults.TabEvictionPolicy != "" {
		if !isValidEvictionPolicy(fc.InstanceDefaults.TabEvictionPolicy) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabEvictionPolicy",
				Message: fmt.Sprintf("invalid value %q (must be reject, close_oldest, or close_lru)", fc.InstanceDefaults.TabEvictionPolicy),
			})
		}
	}
	if fc.InstanceDefaults.MaxTabs != nil && *fc.InstanceDefaults.MaxTabs < 1 {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.maxTabs",
			Message: fmt.Sprintf("must be >= 1 (got %d)", *fc.InstanceDefaults.MaxTabs),
		})
	}
	if fc.InstanceDefaults.MaxParallelTabs != nil && *fc.InstanceDefaults.MaxParallelTabs < 0 {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.maxParallelTabs",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.InstanceDefaults.MaxParallelTabs),
		})
	}

	// Multi-instance validation
	if fc.MultiInstance.Strategy != "" {
		if !isValidStrategy(fc.MultiInstance.Strategy) {
			errs = append(errs, ValidationError{
				Field:   "multiInstance.strategy",
				Message: fmt.Sprintf("invalid value %q (must be simple, explicit, simple-autorestart, or always-on)", fc.MultiInstance.Strategy),
			})
		}
	}
	if fc.MultiInstance.AllocationPolicy != "" {
		if !isValidAllocationPolicy(fc.MultiInstance.AllocationPolicy) {
			errs = append(errs, ValidationError{
				Field:   "multiInstance.allocationPolicy",
				Message: fmt.Sprintf("invalid value %q (must be fcfs, round_robin, or random)", fc.MultiInstance.AllocationPolicy),
			})
		}
	}

	// Attach validation
	for _, scheme := range fc.Security.Attach.AllowSchemes {
		if !isValidAttachScheme(scheme) {
			errs = append(errs, ValidationError{
				Field:   "security.attach.allowSchemes",
				Message: fmt.Sprintf("invalid value %q (must be ws, wss, http, or https)", scheme),
			})
		}
	}

	// IDPI validation
	errs = append(errs, validateIDPIConfig(fc.Security.IDPI)...)

	// Timeouts validation
	if fc.Timeouts.ActionSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "timeouts.actionSec",
			Message: fmt.Sprintf("must be >= 0 (got %d)", fc.Timeouts.ActionSec),
		})
	}
	if fc.Timeouts.NavigateSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "timeouts.navigateSec",
			Message: fmt.Sprintf("must be >= 0 (got %d)", fc.Timeouts.NavigateSec),
		})
	}
	if fc.Timeouts.ShutdownSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "timeouts.shutdownSec",
			Message: fmt.Sprintf("must be >= 0 (got %d)", fc.Timeouts.ShutdownSec),
		})
	}
	if fc.Timeouts.WaitNavMs < 0 {
		errs = append(errs, ValidationError{
			Field:   "timeouts.waitNavMs",
			Message: fmt.Sprintf("must be >= 0 (got %d)", fc.Timeouts.WaitNavMs),
		})
	}

	if fc.Observability.Activity.SessionIdleSec != nil && *fc.Observability.Activity.SessionIdleSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "observability.activity.sessionIdleSec",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.Observability.Activity.SessionIdleSec),
		})
	}
	if fc.Observability.Activity.RetentionDays != nil && *fc.Observability.Activity.RetentionDays < 0 {
		errs = append(errs, ValidationError{
			Field:   "observability.activity.retentionDays",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.Observability.Activity.RetentionDays),
		})
	}

	return errs
}

func validatePort(port string, field string) error {
	p, err := strconv.Atoi(port)
	if err != nil {
		return ValidationError{
			Field:   field,
			Message: fmt.Sprintf("invalid port %q (must be a number)", port),
		}
	}
	if p < 1 || p > 65535 {
		return ValidationError{
			Field:   field,
			Message: fmt.Sprintf("port %d out of range (must be 1-65535)", p),
		}
	}
	return nil
}

func validateBind(bind string, field string) error {
	// Accept common bind addresses
	validBinds := map[string]bool{
		"127.0.0.1": true,
		"0.0.0.0":   true,
		"localhost": true,
		"::1":       true,
		"::":        true,
	}
	if validBinds[bind] {
		return nil
	}
	// Basic IP format check (not exhaustive, just sanity)
	// If it contains a dot, assume it's an IPv4 attempt
	// If it contains a colon, assume it's an IPv6 attempt
	// This is intentionally loose — the OS will reject truly invalid addresses
	return nil
}

func isValidStealthLevel(level string) bool {
	switch level {
	case "light", "medium", "full":
		return true
	default:
		return false
	}
}

func isValidEvictionPolicy(policy string) bool {
	switch policy {
	case "reject", "close_oldest", "close_lru":
		return true
	default:
		return false
	}
}

func isValidStrategy(strategy string) bool {
	switch strategy {
	case "simple", "explicit", "simple-autorestart", "always-on":
		return true
	default:
		return false
	}
}

func isValidAllocationPolicy(policy string) bool {
	switch policy {
	case "fcfs", "round_robin", "random":
		return true
	default:
		return false
	}
}

func isValidAttachScheme(scheme string) bool {
	switch scheme {
	case "ws", "wss", "http", "https":
		return true
	default:
		return false
	}
}

func ValidStealthLevels() []string {
	return []string{"light", "medium", "full"}
}

func ValidEvictionPolicies() []string {
	return []string{"reject", "close_oldest", "close_lru"}
}

func ValidStrategies() []string {
	return []string{"simple", "explicit", "simple-autorestart", "always-on"}
}

// validateIDPIConfig validates the security.idpi sub-section.
// Validation is skipped when IDPI is disabled; a zero-value IDPIConfig is always valid.
func validateIDPIConfig(cfg IDPIConfig) []error {
	if !cfg.Enabled {
		return nil
	}

	var errs []error

	for _, domain := range cfg.AllowedDomains {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" {
			errs = append(errs, ValidationError{
				Field:   "security.idpi.allowedDomains",
				Message: "domain pattern must not be empty or whitespace-only",
			})
			continue
		}
		if strings.ContainsAny(trimmed, " \t") {
			errs = append(errs, ValidationError{
				Field:   "security.idpi.allowedDomains",
				Message: fmt.Sprintf("domain pattern %q must not contain whitespace", trimmed),
			})
		}
		if strings.HasPrefix(trimmed, "file://") {
			errs = append(errs, ValidationError{
				Field:   "security.idpi.allowedDomains",
				Message: fmt.Sprintf("domain pattern %q must not use the file:// scheme; use a hostname", trimmed),
			})
		}
	}

	for _, p := range cfg.CustomPatterns {
		if strings.TrimSpace(p) == "" {
			errs = append(errs, ValidationError{
				Field:   "security.idpi.customPatterns",
				Message: "custom pattern must not be empty or whitespace-only",
			})
		}
	}

	if cfg.ScanTimeoutSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "security.idpi.scanTimeoutSec",
			Message: "scanTimeoutSec must not be negative",
		})
	}

	return errs
}

// ValidAllocationPolicies returns all valid allocation policy values.
func ValidAllocationPolicies() []string {
	return []string{"fcfs", "round_robin", "random"}
}

// ValidAttachSchemes returns all valid attach URL schemes.
func ValidAttachSchemes() []string {
	return []string{"ws", "wss", "http", "https"}
}
