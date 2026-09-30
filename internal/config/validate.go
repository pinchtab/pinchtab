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

	"github.com/pinchtab/pinchtab/internal/autosolver/catalog"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config/geo"
	"github.com/pinchtab/pinchtab/internal/safelog"
	"github.com/pinchtab/pinchtab/internal/session"
)

// validateAgentSessionMode gates sessions.agent.mode. "required" is refused
// rather than accepted-and-ignored: it names a real posture — the session
// credential as the only accepted agent credential — that this server does not
// enforce, and an operator who sets it believes they have session-only auth while
// the bearer token and the dashboard cookie still authenticate. Refusing is
// operator-visible and small-radius (the default is preferred, so only an explicit
// setter is hit) and it replaces a silent security misapprehension with a loud one.
func validateAgentSessionMode(mode string) []error {
	switch session.NormalizeMode(mode) {
	case "", session.ModeOff, session.ModePreferred:
		return nil
	case session.ModeRequired:
		return []error{ValidationError{
			Field:       "sessions.agent.mode",
			Message:     fmt.Sprintf("%q is not implemented: the server bearer token and the dashboard cookie still authenticate, so this value cannot deliver the session-only auth it names (must be off or preferred)", session.ModeRequired),
			FatalAtLoad: true,
		}}
	default:
		return []error{ValidationError{
			Field:       "sessions.agent.mode",
			Message:     fmt.Sprintf("invalid value %q (must be off or preferred)", mode),
			FatalAtLoad: true,
		}}
	}
}

type ValidationError struct {
	Field   string
	Message string

	// FatalAtLoad stops the process at load instead of warning past the problem.
	// Membership is an explicit opt-in with a reason per field, so the class stays
	// readable in the type rather than being re-decided from a message:
	//
	//   sessions.agent.mode — an auth posture. A value the server cannot interpret
	//     would otherwise be read as the default and leave agent sessions serving,
	//     which is the misapprehension refusing it exists to end. Choosing a posture
	//     on the operator's behalf is what a load must not do.
	//
	// It is the only member. Every other validation error warns at load and gates
	// only the write paths, which is what keeps an inert key from aborting a server.
	FatalAtLoad bool
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateFileConfig returns the GATING problems in a FileConfig: values that are wrong,
// out of range, or no longer supported. Every caller that decides whether a write may
// proceed gates on this, so anything returned here blocks a save — off a TTY that means an
// agent's write is refused, which is correct for an invalid port and wrong for a key that
// merely does nothing.
//
// Advisories live in FileConfigAdvisories instead. Two severities rather than one is the
// whole point: a diagnostic about an inert key used to ride this list and abort every
// later config write, including writes to unrelated keys.
func ValidateFileConfig(fc *FileConfig) []error {
	var errs []error
	errs = append(errs, validateServer(fc.Server)...)
	errs = append(errs, validateBrowserSection(fc)...)
	errs = append(errs, validateMultiInstanceRestart(fc.MultiInstance)...)
	errs = append(errs, validateInstanceDefaults(fc.InstanceDefaults)...)
	errs = append(errs, validateMultiInstancePolicy(fc.MultiInstance)...)
	errs = append(errs, validateSecurity(fc.Security, fc.Browser.BrowserExtraFlags)...)
	errs = append(errs, validateTimeouts(fc.Timeouts)...)
	errs = append(errs, validateAutoSolver(fc.AutoSolver)...)
	errs = append(errs, validateObservability(fc.Observability)...)
	errs = append(errs, validateSessions(fc.Sessions)...)
	errs = appendIntPtrRule(errs, "profiles.quarantineKeep", fc.Profiles.QuarantineKeep)
	return errs
}

func validateServer(srv ServerConfig) []error {
	var errs []error
	if srv.Port != "" {
		if err := validatePort(srv.Port, "server.port"); err != nil {
			errs = append(errs, err)
		}
	}
	if srv.Bind != "" {
		if err := validateBind(srv.Bind, "server.bind"); err != nil {
			errs = append(errs, err)
		}
	}
	if srv.LogLevel != "" {
		if _, err := safelog.ParseLevel(srv.LogLevel); err != nil {
			errs = append(errs, ValidationError{Field: "server.logLevel", Message: err.Error()})
		}
	}
	errs = appendIntPtrRule(errs, "server.networkBufferSize", srv.NetworkBufferSize)
	errs = appendIntPtrRule(errs, "server.retainNetworkBodyMaxBytes", srv.RetainNetworkBodyMaxBytes)
	if srv.Engine != "" {
		errs = append(errs, fmt.Errorf("server.engine is no longer supported; use browsers.default instead"))
	}
	return errs
}

func validateBrowserSection(fc *FileConfig) []error {
	var errs []error
	if fc.Browser.Provider != "" {
		errs = append(errs, fmt.Errorf("browser.provider is no longer supported; use browsers.default instead (e.g. \"browsers\": {\"default\": %q})", fc.Browser.Provider))
	}
	errs = append(errs, validateCloakBrowserConfig(fc.Browser.Cloak)...)
	errs = append(errs, ValidateBrowserProxy("browser.proxy", fc.Browser.Proxy)...)
	errs = append(errs, ValidateBrowserTargets(fc.Browser)...)
	errs = append(errs, validateBrowsersBlock(*fc)...)
	return errs
}

func validateMultiInstanceRestart(mi MultiInstanceConfig) []error {
	var errs []error
	if mi.InstancePortStart != nil && mi.InstancePortEnd != nil && *mi.InstancePortStart > *mi.InstancePortEnd {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.instancePortStart/End",
			Message: fmt.Sprintf("start port (%d) must be <= end port (%d)", *mi.InstancePortStart, *mi.InstancePortEnd),
		})
	}
	r := mi.Restart
	errs = appendIntPtrRule(errs, "multiInstance.restart.maxRestarts", r.MaxRestarts)
	errs = appendIntPtrRule(errs, "multiInstance.restart.initBackoffSec", r.InitBackoffSec)
	errs = appendIntPtrRule(errs, "multiInstance.restart.maxBackoffSec", r.MaxBackoffSec)
	errs = appendIntPtrRule(errs, "multiInstance.restart.stableAfterSec", r.StableAfterSec)
	if r.InitBackoffSec != nil && r.MaxBackoffSec != nil && *r.InitBackoffSec > *r.MaxBackoffSec {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.restart.initBackoffSec/maxBackoffSec",
			Message: fmt.Sprintf("init backoff (%d) must be <= max backoff (%d)", *r.InitBackoffSec, *r.MaxBackoffSec),
		})
	}
	return errs
}

func validateInstanceDefaults(d InstanceDefaultsConfig) []error {
	var errs []error
	if d.Headless != nil && d.Mode != "" {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.headless",
			Message: fmt.Sprintf("both headless and mode are set; mode %q takes precedence", d.Mode),
		})
	}
	if d.Mode != "" && d.Mode != "headless" && d.Mode != "headed" {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.mode",
			Message: fmt.Sprintf("invalid value %q (must be headless or headed)", d.Mode),
		})
	}
	if d.StealthLevel != "" && !isValidStealthLevel(d.StealthLevel) {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.stealthLevel",
			Message: fmt.Sprintf("invalid value %q (must be light, medium, or full)", d.StealthLevel),
		})
	}
	if d.TabEvictionPolicy != "" && !isValidEvictionPolicy(d.TabEvictionPolicy) {
		errs = append(errs, ValidationError{
			Field:   "instanceDefaults.tabEvictionPolicy",
			Message: fmt.Sprintf("invalid value %q (must be reject, close_oldest, or close_lru)", d.TabEvictionPolicy),
		})
	}
	if tp := d.TabPolicy; tp != nil {
		if tp.Eviction != "" && !isValidEvictionPolicy(tp.Eviction) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabPolicy.eviction",
				Message: fmt.Sprintf("invalid value %q (must be reject, close_oldest, or close_lru)", tp.Eviction),
			})
		}
		if tp.Lifecycle != "" && !isValidLifecyclePolicy(tp.Lifecycle) {
			errs = append(errs, ValidationError{
				Field:   "instanceDefaults.tabPolicy.lifecycle",
				Message: fmt.Sprintf("invalid value %q (must be keep, close_idle, or freeze_idle)", tp.Lifecycle),
			})
		}
		errs = appendIntPtrRule(errs, "instanceDefaults.tabPolicy.closeDelaySec", tp.CloseDelaySec)
	}
	errs = appendIntPtrRule(errs, "instanceDefaults.maxTabs", d.MaxTabs)
	errs = appendIntPtrRule(errs, "instanceDefaults.maxParallelTabs", d.MaxParallelTabs)
	return errs
}

func validateMultiInstancePolicy(mi MultiInstanceConfig) []error {
	var errs []error
	if mi.Strategy != "" && !isValidStrategy(mi.Strategy) {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.strategy",
			Message: fmt.Sprintf("invalid value %q (must be simple, explicit, simple-autorestart, or always-on)", mi.Strategy),
		})
	}
	if mi.AllocationPolicy != "" && !isValidAllocationPolicy(mi.AllocationPolicy) {
		errs = append(errs, ValidationError{
			Field:   "multiInstance.allocationPolicy",
			Message: fmt.Sprintf("invalid value %q (must be fcfs, round_robin, or random)", mi.AllocationPolicy),
		})
	}
	return errs
}

func validateSecurity(sec SecurityConfig, browserExtraFlags string) []error {
	var errs []error
	for _, scheme := range sec.Attach.AllowSchemes {
		if !isValidAttachScheme(scheme) {
			errs = append(errs, ValidationError{
				Field:   "security.attach.allowSchemes",
				Message: fmt.Sprintf("invalid value %q (must be ws, wss, http, or https)", scheme),
			})
		}
	}
	if browserExtraFlags != "" {
		errs = append(errs, validateBrowserExtraFlags(browserExtraFlags)...)
	}
	errs = append(errs, validateIDPIConfig(sec.EffectiveIDPI(), effectiveSecurityAllowedDomains(sec))...)
	errs = append(errs, validateAllowedDomainList("security.downloadAllowedDomains", sec.DownloadAllowedDomains)...)
	errs = append(errs, validateTrustedCIDRList("security.trustedProxyCIDRs", sec.TrustedProxyCIDRs)...)
	errs = append(errs, validateTrustedCIDRList("security.trustedResolveCIDRs", sec.TrustedResolveCIDRs)...)
	errs = appendIntPtrRule(errs, "security.downloadMaxBytes", sec.DownloadMaxBytes)
	errs = appendIntPtrRule(errs, "security.memorySnapshotMaxBytes", sec.MemorySnapshotMaxBytes)
	errs = appendIntPtrRule(errs, "security.uploadMaxRequestBytes", sec.UploadMaxRequestBytes)
	errs = appendIntPtrRule(errs, "security.uploadMaxFiles", sec.UploadMaxFiles)
	errs = appendIntPtrRule(errs, "security.uploadMaxFileBytes", sec.UploadMaxFileBytes)
	errs = appendIntPtrRule(errs, "security.uploadMaxTotalBytes", sec.UploadMaxTotalBytes)
	if sec.UploadMaxFileBytes != nil && sec.UploadMaxTotalBytes != nil && *sec.UploadMaxFileBytes > *sec.UploadMaxTotalBytes {
		errs = append(errs, ValidationError{
			Field:   "security.uploadMaxFileBytes/uploadMaxTotalBytes",
			Message: fmt.Sprintf("uploadMaxFileBytes (%d) must be <= uploadMaxTotalBytes (%d)", *sec.UploadMaxFileBytes, *sec.UploadMaxTotalBytes),
		})
	}
	errs = appendIntRule(errs, "security.idpi.shieldThreshold", sec.EffectiveIDPI().ShieldThreshold)
	return errs
}

func validateTimeouts(t TimeoutsConfig) []error {
	var errs []error
	errs = appendIntRule(errs, "timeouts.actionSec", t.ActionSec)
	errs = appendIntRule(errs, "timeouts.navigateSec", t.NavigateSec)
	errs = appendIntRule(errs, "timeouts.shutdownSec", t.ShutdownSec)
	errs = appendIntRule(errs, "timeouts.waitNavMs", t.WaitNavMs)
	return errs
}

func validateAutoSolver(a AutoSolverFileConfig) []error {
	var errs []error
	errs = appendIntPtrRule(errs, "autoSolver.maxAttempts", a.MaxAttempts)
	errs = appendIntPtrRule(errs, "autoSolver.solverTimeoutSec", a.SolverTimeoutSec)
	errs = appendIntPtrRule(errs, "autoSolver.retryBaseDelayMs", a.RetryBaseDelayMs)
	errs = appendIntPtrRule(errs, "autoSolver.retryMaxDelayMs", a.RetryMaxDelayMs)
	if a.RetryBaseDelayMs != nil && a.RetryMaxDelayMs != nil && *a.RetryBaseDelayMs > *a.RetryMaxDelayMs {
		errs = append(errs, ValidationError{
			Field:   "autoSolver.retryBaseDelayMs/retryMaxDelayMs",
			Message: fmt.Sprintf("retry base delay (%d) must be <= retry max delay (%d)", *a.RetryBaseDelayMs, *a.RetryMaxDelayMs),
		})
	}
	for _, solverName := range a.Solvers {
		if strings.TrimSpace(solverName) == "" {
			errs = append(errs, ValidationError{
				Field:   "autoSolver.solvers",
				Message: "solver names must not be empty",
			})
			break
		}
	}
	// An unmatched name does not fail at run time, it changes which solvers run:
	// one typo alongside good names silently drops that solver, and a list of
	// only typos silently runs every solver instead.
	for i, solverName := range a.Solvers {
		name := strings.TrimSpace(solverName)
		if name == "" || catalog.IsKnown(name) {
			continue
		}
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("autoSolver.solvers[%d]", i),
			Message: fmt.Sprintf("unknown solver %q (known: %v)", name, catalog.Names()),
		})
	}
	return errs
}

func validateObservability(o ObservabilityFileConfig) []error {
	var errs []error
	errs = appendIntPtrRule(errs, "observability.activity.sessionIdleSec", o.Activity.SessionIdleSec)
	errs = appendIntPtrRule(errs, "observability.activity.retentionDays", o.Activity.RetentionDays)
	return errs
}

func validateSessions(s SessionsFileConfig) []error {
	errs := validateAgentSessionMode(s.Agent.Mode)
	d := s.Dashboard
	errs = appendIntPtrRule(errs, "sessions.dashboard.idleTimeoutSec", d.IdleTimeoutSec)
	errs = appendIntPtrRule(errs, "sessions.dashboard.maxLifetimeSec", d.MaxLifetimeSec)
	errs = appendIntPtrRule(errs, "sessions.dashboard.elevationWindowSec", d.ElevationWindowSec)
	if d.IdleTimeoutSec != nil && d.MaxLifetimeSec != nil && *d.IdleTimeoutSec > *d.MaxLifetimeSec {
		errs = append(errs, ValidationError{
			Field:   "sessions.dashboard.idleTimeoutSec/maxLifetimeSec",
			Message: fmt.Sprintf("idle timeout (%d) must be <= max lifetime (%d)", *d.IdleTimeoutSec, *d.MaxLifetimeSec),
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
	lifecyclePolicies  = []string{"keep", "close_idle", "freeze_idle"}
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

func IdleTabLifecycle(policy string) bool {
	return policy == "close_idle" || policy == "freeze_idle"
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

// FileConfigAdvisories reports what a config file says that has no effect. These are
// REPORTED, never gating: nothing here means the file is unsafe to load or the next write
// is unsafe to make, so blocking a save on one would refuse work for no reason.
//
// A diagnostic belongs here only when the value is inert — PinchTab reads something else,
// or nothing — and the reader has nothing to fix. Anything the user could get wrong, and
// anything PinchTab will act on, stays in ValidateFileConfig where it gates.
//
//	observability.activity.stateDir  the path is derived from server.stateDir; the loader
//	                                 never reads this key, and `config set` refuses it
func FileConfigAdvisories(fc *FileConfig) []string {
	if fc == nil {
		return nil
	}
	var advisories []string
	if strings.TrimSpace(fc.Observability.Activity.StateDir) != "" {
		advisories = append(advisories, ActivityStateDirAdvisory)
	}
	advisories = append(advisories, proxyServerAdvisories(fc)...)
	return advisories
}

// proxyServerAdvisories reports every proxy block that holds something but no server.
// Target proxies are walked too: their blocks were never dropped at save, so a
// serverless one has always been kept and silently unused — the same state this card's
// defect produced for browser.proxy, one level down.
func proxyServerAdvisories(fc *FileConfig) []string {
	var advisories []string
	if !fc.Browser.Proxy.IsZero() && fc.Browser.Proxy.HasNoServer() {
		advisories = append(advisories, ProxyServerRequiredAdvisory("browser.proxy"))
	}
	for _, name := range SortedBrowserTargetNames(fc.Browser.Targets) {
		target := fc.Browser.Targets[name]
		if !target.Proxy.IsZero() && target.Proxy.HasNoServer() {
			advisories = append(advisories, ProxyServerRequiredAdvisory(fmt.Sprintf("browser.targets.%s.proxy", name)))
		}
	}
	return advisories
}
