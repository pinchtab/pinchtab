package config

import (
	"fmt"
	"net"
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
	if fc.Server.RetainNetworkBodyMaxBytes != nil {
		if *fc.Server.RetainNetworkBodyMaxBytes < 0 || *fc.Server.RetainNetworkBodyMaxBytes > MaxRetainNetworkBodyMaxBytes {
			errs = append(errs, ValidationError{
				Field:   "server.retainNetworkBodyMaxBytes",
				Message: fmt.Sprintf("must be between 0 and %d (got %d)", MaxRetainNetworkBodyMaxBytes, *fc.Server.RetainNetworkBodyMaxBytes),
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
	if fc.InstanceDefaults.Headless != nil && fc.InstanceDefaults.Mode != "" {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.headless",
			Message: fmt.Sprintf("both headless and mode are set; mode %q takes precedence", fc.InstanceDefaults.Mode),
		})
	}
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
	if tp := fc.InstanceDefaults.TabPolicy; tp != nil {
		if tp.Eviction != "" && !isValidEvictionPolicy(tp.Eviction) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabPolicy.eviction",
				Message: fmt.Sprintf("invalid value %q (must be reject, close_oldest, or close_lru)", tp.Eviction),
			})
		}
		if tp.Lifecycle != "" && !isValidLifecyclePolicy(tp.Lifecycle) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabPolicy.lifecycle",
				Message: fmt.Sprintf("invalid value %q (must be keep or close_idle)", tp.Lifecycle),
			})
		}
		if tp.CloseDelaySec != nil && *tp.CloseDelaySec < 0 {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabPolicy.closeDelaySec",
				Message: fmt.Sprintf("must be >= 0 (got %d)", *tp.CloseDelaySec),
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

	if fc.Browser.ChromeExtraFlags != "" {
		errs = append(errs, validateChromeExtraFlags(fc.Browser.ChromeExtraFlags)...)
	}

	// IDPI validation
	errs = append(errs, validateIDPIConfig(fc.Security.IDPI, effectiveSecurityAllowedDomains(fc.Security))...)
	errs = append(errs, validateTransactionPolicyConfig(fc.Security.TransactionPolicy)...)
	if fc.Security.TransactionPolicy.Enabled && len(fc.Browser.ExtensionPaths) != 0 {
		errs = append(errs, ValidationError{Field: "browser.extensionPaths", Message: "must be empty when security.transactionPolicy is enabled"})
	}
	errs = append(errs, validateAllowedDomainList("security.downloadAllowedDomains", fc.Security.DownloadAllowedDomains)...)
	errs = append(errs, validateTrustedCIDRList("security.trustedProxyCIDRs", fc.Security.TrustedProxyCIDRs)...)
	errs = append(errs, validateTrustedCIDRList("security.trustedResolveCIDRs", fc.Security.TrustedResolveCIDRs)...)
	errs = append(errs, validatePositiveIntLimit("security.downloadMaxBytes", fc.Security.DownloadMaxBytes, MaxDownloadMaxBytes)...)
	errs = append(errs, validatePositiveIntLimit("security.uploadMaxRequestBytes", fc.Security.UploadMaxRequestBytes, MaxUploadMaxRequestBytes)...)
	errs = append(errs, validatePositiveIntLimit("security.uploadMaxFiles", fc.Security.UploadMaxFiles, MaxUploadMaxFiles)...)
	errs = append(errs, validatePositiveIntLimit("security.uploadMaxFileBytes", fc.Security.UploadMaxFileBytes, MaxUploadMaxFileBytes)...)
	errs = append(errs, validatePositiveIntLimit("security.uploadMaxTotalBytes", fc.Security.UploadMaxTotalBytes, MaxUploadMaxTotalBytes)...)
	if fc.Security.UploadMaxFileBytes != nil && fc.Security.UploadMaxTotalBytes != nil &&
		*fc.Security.UploadMaxFileBytes > *fc.Security.UploadMaxTotalBytes {
		errs = append(errs, ValidationError{
			Field:   "security.uploadMaxFileBytes/uploadMaxTotalBytes",
			Message: fmt.Sprintf("uploadMaxFileBytes (%d) must be <= uploadMaxTotalBytes (%d)", *fc.Security.UploadMaxFileBytes, *fc.Security.UploadMaxTotalBytes),
		})
	}

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

	if fc.AutoSolver.MaxAttempts != nil && *fc.AutoSolver.MaxAttempts < 1 {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.maxAttempts",
			Message: fmt.Sprintf("must be >= 1 (got %d)", *fc.AutoSolver.MaxAttempts),
		})
	}
	if fc.AutoSolver.SolverTimeoutSec != nil && *fc.AutoSolver.SolverTimeoutSec <= 0 {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.solverTimeoutSec",
			Message: fmt.Sprintf("must be > 0 (got %d)", *fc.AutoSolver.SolverTimeoutSec),
		})
	}
	if fc.AutoSolver.RetryBaseDelayMs != nil && *fc.AutoSolver.RetryBaseDelayMs < 0 {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.retryBaseDelayMs",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.AutoSolver.RetryBaseDelayMs),
		})
	}
	if fc.AutoSolver.RetryMaxDelayMs != nil && *fc.AutoSolver.RetryMaxDelayMs < 0 {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.retryMaxDelayMs",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.AutoSolver.RetryMaxDelayMs),
		})
	}
	if fc.AutoSolver.RetryBaseDelayMs != nil && fc.AutoSolver.RetryMaxDelayMs != nil &&
		*fc.AutoSolver.RetryBaseDelayMs > *fc.AutoSolver.RetryMaxDelayMs {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.retryBaseDelayMs/retryMaxDelayMs",
			Message: fmt.Sprintf("retry base delay (%d) must be <= retry max delay (%d)", *fc.AutoSolver.RetryBaseDelayMs, *fc.AutoSolver.RetryMaxDelayMs),
		})
	}
	for _, solverName := range fc.AutoSolver.Solvers {
		if strings.TrimSpace(solverName) == "" {
			errs = append(errs, ValidationError{
				Field:   "autoSolver.solvers",
				Message: "solver names must not be empty",
			})
			break
		}
	}

	if fc.Observability.Activity.SessionIdleSec != nil && *fc.Observability.Activity.SessionIdleSec < 0 {
		errs = append(errs, ValidationError{
			Field:   "observability.activity.sessionIdleSec",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *fc.Observability.Activity.SessionIdleSec),
		})
	}
	if fc.Observability.Activity.RetentionDays != nil && *fc.Observability.Activity.RetentionDays <= 0 {
		errs = append(errs, ValidationError{
			Field:   "observability.activity.retentionDays",
			Message: fmt.Sprintf("must be > 0 (got %d)", *fc.Observability.Activity.RetentionDays),
		})
	}
	if fc.Sessions.Dashboard.IdleTimeoutSec != nil && *fc.Sessions.Dashboard.IdleTimeoutSec <= 0 {
		errs = append(errs, ValidationError{
			Field:   "sessions.dashboard.idleTimeoutSec",
			Message: fmt.Sprintf("must be > 0 (got %d)", *fc.Sessions.Dashboard.IdleTimeoutSec),
		})
	}
	if fc.Sessions.Dashboard.MaxLifetimeSec != nil && *fc.Sessions.Dashboard.MaxLifetimeSec <= 0 {
		errs = append(errs, ValidationError{
			Field:   "sessions.dashboard.maxLifetimeSec",
			Message: fmt.Sprintf("must be > 0 (got %d)", *fc.Sessions.Dashboard.MaxLifetimeSec),
		})
	}
	if fc.Sessions.Dashboard.ElevationWindowSec != nil && *fc.Sessions.Dashboard.ElevationWindowSec <= 0 {
		errs = append(errs, ValidationError{
			Field:   "sessions.dashboard.elevationWindowSec",
			Message: fmt.Sprintf("must be > 0 (got %d)", *fc.Sessions.Dashboard.ElevationWindowSec),
		})
	}
	if fc.Sessions.Dashboard.IdleTimeoutSec != nil && fc.Sessions.Dashboard.MaxLifetimeSec != nil &&
		*fc.Sessions.Dashboard.IdleTimeoutSec > *fc.Sessions.Dashboard.MaxLifetimeSec {
		errs = append(errs, ValidationError{
			Field:   "sessions.dashboard.idleTimeoutSec/maxLifetimeSec",
			Message: fmt.Sprintf("idle timeout (%d) must be <= max lifetime (%d)", *fc.Sessions.Dashboard.IdleTimeoutSec, *fc.Sessions.Dashboard.MaxLifetimeSec),
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

func isValidLifecyclePolicy(policy string) bool {
	switch policy {
	case "keep", "close_idle":
		return true
	default:
		return false
	}
}

func ValidLifecyclePolicies() []string {
	return []string{"keep", "close_idle"}
}

func isValidStrategy(strategy string) bool {
	switch strategy {
	case "simple", "explicit", "simple-autorestart", "always-on", "no-instance":
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
	return []string{"simple", "explicit", "simple-autorestart", "always-on", "no-instance"}
}

// validateIDPIConfig validates the security.idpi sub-section.
// Validation is skipped when IDPI is disabled; a zero-value IDPIConfig is always valid.
func validateIDPIConfig(cfg IDPIConfig, allowedDomains []string) []error {
	if !cfg.Enabled {
		return nil
	}

	errs := validateAllowedDomainList("security.allowedDomains", allowedDomains)

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

func validateAllowedDomainList(field string, domains []string) []error {
	var errs []error
	for _, domain := range domains {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "domain pattern must not be empty or whitespace-only",
			})
			continue
		}
		if strings.ContainsAny(trimmed, " \t") {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("domain pattern %q must not contain whitespace", trimmed),
			})
		}
		if strings.HasPrefix(trimmed, "file://") {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("domain pattern %q must not use the file:// scheme; use a hostname", trimmed),
			})
		}
	}
	return errs
}

func validateTrustedCIDRList(field string, items []string) []error {
	var errs []error
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "entry must not be empty or whitespace-only",
			})
			continue
		}
		if strings.Contains(trimmed, "/") {
			if _, _, err := net.ParseCIDR(trimmed); err != nil {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("entry %q must be a valid CIDR or IP address", trimmed),
				})
			}
			continue
		}
		if net.ParseIP(trimmed) == nil {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("entry %q must be a valid CIDR or IP address", trimmed),
			})
		}
	}
	return errs
}

func validatePositiveIntLimit(field string, value *int, max int) []error {
	if value == nil {
		return nil
	}
	if *value < 1 || *value > max {
		return []error{ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be between 1 and %d (got %d)", max, *value),
		}}
	}
	return nil
}

// ValidAllocationPolicies returns all valid allocation policy values.
func ValidAllocationPolicies() []string {
	return []string{"fcfs", "round_robin", "random"}
}

// ValidAttachSchemes returns all valid attach URL schemes.
func ValidAttachSchemes() []string {
	return []string{"ws", "wss", "http", "https"}
}

// validateTransactionPolicyConfig validates the request guard only when enabled.
func validateTransactionPolicyConfig(cfg TransactionPolicyConfig) []error {
	if !cfg.Enabled {
		return nil
	}
	var errs []error
	if len(cfg.Hosts) == 0 {
		errs = append(errs, ValidationError{Field: "security.transactionPolicy.hosts", Message: "must contain at least one host when enabled"})
	}
	if len(cfg.DenyRules) == 0 {
		errs = append(errs, ValidationError{Field: "security.transactionPolicy.denyRules", Message: "must contain at least one rule when enabled"})
	}
	for _, host := range cfg.Hosts {
		host = strings.TrimSpace(host)
		if !validTransactionHost(host) {
			errs = append(errs, ValidationError{Field: "security.transactionPolicy.hosts", Message: "host must be an exact DNS hostname or IPv4 address without a scheme, port, wildcard, trailing dot, path, or whitespace"})
		}
	}
	for _, group := range []struct {
		field string
		rules []TransactionPolicyRule
		allow bool
	}{{"security.transactionPolicy.denyRules", cfg.DenyRules, false}, {"security.transactionPolicy.allowRules", cfg.AllowRules, true}} {
		for _, rule := range group.rules {
			method := strings.ToUpper(strings.TrimSpace(rule.Method))
			if !isValidTransactionMethod(method) {
				errs = append(errs, ValidationError{Field: group.field, Message: fmt.Sprintf("invalid method %q", rule.Method)})
			}
			if !validTransactionPathPrefix(rule.PathPrefix) {
				errs = append(errs, ValidationError{Field: group.field, Message: fmt.Sprintf("pathPrefix %q must be an unescaped absolute path made of unreserved ASCII characters", rule.PathPrefix)})
			}
			if rule.PathSegment != "" && !validTransactionToken(rule.PathSegment) {
				errs = append(errs, ValidationError{Field: group.field, Message: "pathSegment must be one unescaped unreserved path segment"})
			}
			if (rule.QueryParam == "") != (rule.QueryValue == "") {
				errs = append(errs, ValidationError{Field: group.field, Message: "queryParam and queryValue must be set together"})
			}
			if (rule.QueryParam != "" && !validTransactionToken(rule.QueryParam)) || (rule.QueryValue != "" && !validTransactionToken(rule.QueryValue)) {
				errs = append(errs, ValidationError{Field: group.field, Message: "queryParam/queryValue must be unescaped unreserved ASCII"})
			}
			if group.allow && isUnsafeTransactionMethod(method) && strings.Trim(strings.TrimSpace(rule.PathPrefix), "/") == "" && rule.QueryParam == "" {
				errs = append(errs, ValidationError{Field: group.field, Message: "unsafe root-wide allow requires an exact query condition"})
			}
		}
	}
	return errs
}

func validTransactionHost(host string) bool {
	if host == "" || len(host) > 253 || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

func validTransactionPathPrefix(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." || !validTransactionToken(segment) {
			return false
		}
	}
	return true
}

func validTransactionToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._~", r)) {
			return false
		}
	}
	return true
}

func isUnsafeTransactionMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return false
	default:
		return true
	}
}

func isValidTransactionMethod(method string) bool {
	switch method {
	case "*", "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		return true
	default:
		return false
	}
}
