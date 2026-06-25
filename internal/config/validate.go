package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config/geo"
)

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

	if fc.Server.Engine != "" {
		errs = append(errs, fmt.Errorf("server.engine is no longer supported; use browsers.default instead"))
	}
	if fc.Browser.Provider != "" {
		errs = append(errs, fmt.Errorf("browser.provider is no longer supported; use browsers.default instead (e.g. \"browsers\": {\"default\": %q})", fc.Browser.Provider))
	}
	errs = append(errs, validateCloakBrowserConfig(fc.Browser.Cloak)...)
	errs = append(errs, ValidateBrowserProxy("browser.proxy", fc.Browser.Proxy)...)
	errs = append(errs, ValidateBrowserTargets(fc.Browser)...)
	errs = append(errs, validateBrowsersBlock(*fc)...)

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

	for _, scheme := range fc.Security.Attach.AllowSchemes {
		if !isValidAttachScheme(scheme) {
			errs = append(errs, ValidationError{
				Field:   "security.attach.allowSchemes",
				Message: fmt.Sprintf("invalid value %q (must be ws, wss, http, or https)", scheme),
			})
		}
	}

	if fc.Browser.BrowserExtraFlags != "" {
		errs = append(errs, validateBrowserExtraFlags(fc.Browser.BrowserExtraFlags)...)
	}

	errs = append(errs, validateIDPIConfig(fc.Security.IDPI, effectiveSecurityAllowedDomains(fc.Security))...)
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
	// Intentionally loose — the OS will reject truly invalid addresses.
	return nil
}

var cloakFingerprintSeedRegex = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func validateCloakBrowserConfig(cloak CloakBrowserConfig) []error {
	return validateCloakBrowserConfigAt("browser.cloak", cloak)
}

func validateCloakBrowserConfigAt(fieldPrefix string, cloak CloakBrowserConfig) []error {
	var errs []error
	if cloak.Platform != "" && !isValidCloakPlatform(cloak.Platform) {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix + ".platform",
			Message: fmt.Sprintf("invalid value %q (must be windows, macos, or linux)", cloak.Platform),
		})
	}
	if cloak.StorageQuotaMB != nil && *cloak.StorageQuotaMB < 0 {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix + ".storageQuotaMB",
			Message: fmt.Sprintf("must be >= 0 (got %d)", *cloak.StorageQuotaMB),
		})
	}
	if seed := strings.TrimSpace(cloak.FingerprintSeed); seed != "" && !cloakFingerprintSeedRegex.MatchString(seed) {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix + ".fingerprintSeed",
			Message: "must be 1-128 characters of letters, numbers, dot, underscore, colon, or hyphen",
		})
	}
	if err := geo.Validate(geo.Info{Timezone: cloak.Timezone, Locale: cloak.Locale}); err != nil {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix,
			Message: err.Error(),
		})
	}
	if ip := strings.TrimSpace(cloak.WebRTCIP); ip != "" && !strings.EqualFold(ip, "auto") && net.ParseIP(ip) == nil {
		errs = append(errs, ValidationError{
			Field:   fieldPrefix + ".webrtcIP",
			Message: fmt.Sprintf("webrtcIP %q must be \"auto\" or a valid IP address", cloak.WebRTCIP),
		})
	}
	if dir := strings.TrimSpace(cloak.FontsDir); dir != "" {
		clean := filepath.Clean(dir)
		if clean != dir {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + ".fontsDir",
				Message: fmt.Sprintf("must be a clean path (got %q, clean form is %q)", dir, clean),
			})
		} else if st, err := os.Stat(dir); err != nil {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + ".fontsDir",
				Message: fmt.Sprintf("must be an existing directory: %v", err),
			})
		} else if !st.IsDir() {
			errs = append(errs, ValidationError{
				Field:   fieldPrefix + ".fontsDir",
				Message: "must be an existing directory",
			})
		}
	}
	return errs
}

// Enumerated config option sets are defined exactly once here; both the
// isValid* membership checks and the exported Valid*() lists derive from these
// slices so the two can't drift apart.
var (
	cloakPlatforms     = []string{"windows", "macos", "linux"}
	stealthLevels      = []string{"light", "medium", "full"}
	evictionPolicies   = []string{"reject", "close_oldest", "close_lru"}
	lifecyclePolicies  = []string{"keep", "close_idle"}
	strategies         = []string{"simple", "explicit", "simple-autorestart", "always-on", "no-instance"}
	allocationPolicies = []string{"fcfs", "round_robin", "random"}
	attachSchemes      = []string{"ws", "wss", "http", "https"}
)

func isValidCloakPlatform(platform string) bool {
	return slices.Contains(cloakPlatforms, strings.ToLower(strings.TrimSpace(platform)))
}

func isValidStealthLevel(level string) bool {
	return slices.Contains(stealthLevels, level)
}

func isValidEvictionPolicy(policy string) bool {
	return slices.Contains(evictionPolicies, policy)
}

func isValidLifecyclePolicy(policy string) bool {
	return slices.Contains(lifecyclePolicies, policy)
}

func ValidLifecyclePolicies() []string {
	return slices.Clone(lifecyclePolicies)
}

func isValidStrategy(strategy string) bool {
	return slices.Contains(strategies, strategy)
}

func isValidAllocationPolicy(policy string) bool {
	return slices.Contains(allocationPolicies, policy)
}

func isValidAttachScheme(scheme string) bool {
	return slices.Contains(attachSchemes, scheme)
}

func ValidStealthLevels() []string {
	return slices.Clone(stealthLevels)
}

func ValidEvictionPolicies() []string {
	return slices.Clone(evictionPolicies)
}

func ValidStrategies() []string {
	return slices.Clone(strategies)
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

func ValidAllocationPolicies() []string {
	return slices.Clone(allocationPolicies)
}

func ValidAttachSchemes() []string {
	return slices.Clone(attachSchemes)
}

func validateBrowsersBlock(fc FileConfig) []error {
	bc := fc.Browsers
	if bc.Default == "" && len(bc.Available) == 0 && len(bc.Config) == 0 {
		return nil
	}

	var errs []error

	// Validate default is a known browser. The registry is keyed by lowercase
	// IDs; trim+lowercase like target validation does so "Cloak" validates
	// the same everywhere.
	if bc.Default != "" {
		if _, ok := browsers.Get(strings.ToLower(strings.TrimSpace(bc.Default))); !ok {
			errs = append(errs, fmt.Errorf("browsers.default: unknown browser %q (known: %v)", bc.Default, browsers.IDs()))
		}
	}

	for i, name := range bc.Available {
		if _, ok := browsers.Get(strings.ToLower(strings.TrimSpace(name))); !ok {
			errs = append(errs, fmt.Errorf("browsers.available[%d]: unknown browser %q (known: %v)", i, name, browsers.IDs()))
		}
	}

	if bc.Default != "" && len(bc.Available) > 0 {
		found := false
		wantDefault := strings.ToLower(strings.TrimSpace(bc.Default))
		for _, name := range bc.Available {
			if strings.ToLower(strings.TrimSpace(name)) == wantDefault {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("browsers.default %q is not in browsers.available %v", bc.Default, bc.Available))
		}
	}

	// browsers.config was accepted but never applied anywhere; reject it with
	// guidance (mirroring the browser.provider retirement) rather than letting
	// the overrides be silently ignored.
	if len(bc.Config) > 0 {
		errs = append(errs, fmt.Errorf("browsers.config is no longer supported; use browser.targets.<name> instead (e.g. \"browser\": {\"targets\": {\"cloak\": {\"binary\": \"/opt/cloak/bin\"}}})"))
	}

	return errs
}
