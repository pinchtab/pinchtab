package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

const DefaultBrowserTargetName = "default"

var browserTargetNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

func IsValidBrowserTargetName(name string) bool {
	return browserTargetNameRegex.MatchString(name)
}

func isRecognizedBrowserTarget(p string) bool {
	_, ok := browsers.Get(strings.ToLower(strings.TrimSpace(p)))
	return ok
}

func cloneBrowserTargetsConfig(in BrowserTargetsConfig) BrowserTargetsConfig {
	if len(in) == 0 {
		return nil
	}
	out := make(BrowserTargetsConfig, len(in))
	for name, target := range in {
		out[name] = cloneBrowserTarget(target)
	}
	return out
}

func cloneBrowserTarget(in BrowserTargetConfig) BrowserTargetConfig {
	out := in
	out.Cloak = cloneCloakBrowserConfig(in.Cloak)
	out.Proxy = cloneBrowserProxyConfig(in.Proxy)
	return out
}

func cloneCloakBrowserConfig(in CloakBrowserConfig) CloakBrowserConfig {
	out := in
	if in.StorageQuotaMB != nil {
		v := *in.StorageQuotaMB
		out.StorageQuotaMB = &v
	}
	if in.DisableDefaultStealthArgs != nil {
		v := *in.DisableDefaultStealthArgs
		out.DisableDefaultStealthArgs = &v
	}
	return out
}

func cloneBrowserProxyConfig(in BrowserProxyConfig) BrowserProxyConfig {
	out := in
	out.BypassList = append([]string(nil), in.BypassList...)
	if in.Geo != nil {
		geo := *in.Geo
		out.Geo = &geo
	}
	return out
}

// ValidateBrowserTargets returns nil when no targets are configured.
func ValidateBrowserTargets(bc BrowserConfig) []error {
	if len(bc.Targets) == 0 {
		return nil
	}

	var errs []error

	// Stable iteration order so error output is deterministic.
	names := make([]string, 0, len(bc.Targets))
	for name := range bc.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t := bc.Targets[name]
		if !IsValidBrowserTargetName(name) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.targets.%s", name),
				Message: fmt.Sprintf("invalid target name %q (must match ^[a-z][a-z0-9-]{0,31}$)", name),
			})
		}
		if strings.TrimSpace(t.Provider) == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.targets.%s.provider", name),
				Message: "provider is required",
			})
		} else if !isRecognizedBrowserTarget(t.Provider) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.targets.%s.provider", name),
				Message: fmt.Sprintf("unknown provider %q (known: %v)", t.Provider, browsers.IDs()),
			})
		} else {
			provID := NormalizeBrowser(t.Provider)
			if b, ok := browsers.Get(provID); ok {
				binary := strings.TrimSpace(t.Binary)
				if binary == "" {
					binary = strings.TrimSpace(bc.BrowserBinary)
				}
				tcfg := browsers.TargetConfig{
					Provider:   provID,
					Binary:     binary,
					ExtraFlags: t.ExtraFlags,
				}
				if err := b.ValidateTarget(tcfg); err != nil {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("browser.targets.%s.binary", name),
						Message: err.Error(),
					})
				}
			}
		}
		if strings.TrimSpace(t.ExtraFlags) != "" {
			errs = append(errs, validateTargetExtraFlags(name, t.ExtraFlags)...)
		}
		errs = append(errs, validateCloakBrowserConfigAt(fmt.Sprintf("browser.targets.%s.cloak", name), t.Cloak)...)
		errs = append(errs, ValidateBrowserProxy(fmt.Sprintf("browser.targets.%s.proxy", name), t.Proxy)...)
	}

	defaultTarget := strings.TrimSpace(bc.DefaultTarget)
	if dt := strings.TrimSpace(bc.DefaultTarget); dt != "" {
		if _, ok := bc.Targets[dt]; !ok {
			errs = append(errs, ValidationError{
				Field:   "browser.defaultTarget",
				Message: fmt.Sprintf("references unknown target %q", dt),
			})
		}
	} else if len(bc.Targets) > 1 {
		errs = append(errs, ValidationError{
			Field:   "browser.defaultTarget",
			Message: "must be set when multiple targets are configured",
		})
	}

	fallbackSeen := make(map[string]int, len(bc.FallbackOrder))
	for i, name := range bc.FallbackOrder {
		name = strings.TrimSpace(name)
		if name == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.fallbackOrder[%d]", i),
				Message: "target name must not be empty",
			})
			continue
		}
		if first, ok := fallbackSeen[name]; ok {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.fallbackOrder[%d]", i),
				Message: fmt.Sprintf("duplicates target %q already listed at browser.fallbackOrder[%d]", name, first),
			})
			continue
		}
		fallbackSeen[name] = i
		if defaultTarget != "" && name == defaultTarget {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.fallbackOrder[%d]", i),
				Message: fmt.Sprintf("must not include defaultTarget %q", name),
			})
		}
		if _, ok := bc.Targets[name]; !ok {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("browser.fallbackOrder[%d]", i),
				Message: fmt.Sprintf("references unknown target %q", name),
			})
		}
	}

	return errs
}

func validateTargetExtraFlags(targetName, raw string) []error {
	flagErrs := validateBrowserExtraFlags(raw)
	if len(flagErrs) == 0 {
		return nil
	}
	out := make([]error, 0, len(flagErrs))
	field := fmt.Sprintf("browser.targets.%s.extraFlags", targetName)
	for _, err := range flagErrs {
		if ve, ok := err.(ValidationError); ok {
			ve.Field = field
			out = append(out, ve)
			continue
		}
		out = append(out, ValidationError{Field: field, Message: err.Error()})
	}
	return out
}

// migrateLegacyBrowserConfig synthesizes targets["default"] from legacy fields when targets is empty.
// When both blocks are present, explicit targets win and conflict=true so the caller can warn.
func migrateLegacyBrowserConfig(bc *BrowserConfig, browsersDefault string) (synthesized bool, conflict bool) {
	if bc == nil {
		return false, false
	}

	hasLegacy := strings.TrimSpace(bc.Provider) != "" ||
		strings.TrimSpace(bc.BrowserBinary) != "" ||
		strings.TrimSpace(bc.BrowserExtraFlags) != "" ||
		hasCloakBrowserConfig(bc.Cloak) ||
		!bc.Proxy.IsZero()

	if len(bc.Targets) > 0 {
		if hasLegacy {
			conflict = true
		}
		return false, conflict
	}

	if !hasLegacy {
		return false, false
	}

	providerRaw := bc.Provider
	if providerRaw == "" {
		providerRaw = browsersDefault
	}
	provider := NormalizeBrowser(providerRaw)
	target := BrowserTargetConfig{
		Provider:   provider,
		Binary:     bc.BrowserBinary,
		ExtraFlags: bc.BrowserExtraFlags,
		Cloak:      cloneCloakBrowserConfig(bc.Cloak),
		Proxy:      cloneBrowserProxyConfig(bc.Proxy),
	}
	bc.Targets = BrowserTargetsConfig{DefaultBrowserTargetName: target}
	if strings.TrimSpace(bc.DefaultTarget) == "" {
		bc.DefaultTarget = DefaultBrowserTargetName
	}
	return true, false
}

type ResolvedBrowserTarget struct {
	Name     string
	Provider string
	Target   BrowserTargetConfig
	Config   *RuntimeConfig
	Legacy   bool
}

func CloneRuntimeConfig(in *RuntimeConfig) *RuntimeConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.CookieSecure = cloneBoolPtr(in.CookieSecure)
	out.AllowedDomains = cloneStringSlice(in.AllowedDomains)
	out.DownloadAllowedDomains = cloneStringSlice(in.DownloadAllowedDomains)
	out.TrustedProxyCIDRs = cloneStringSlice(in.TrustedProxyCIDRs)
	out.TrustedResolveCIDRs = cloneStringSlice(in.TrustedResolveCIDRs)
	out.ExtensionPaths = cloneStringSlice(in.ExtensionPaths)
	out.FallbackOrder = cloneStringSlice(in.FallbackOrder)
	out.AttachAllowHosts = cloneStringSlice(in.AttachAllowHosts)
	out.AttachAllowSchemes = cloneStringSlice(in.AttachAllowSchemes)
	out.IDPI.CustomPatterns = cloneStringSlice(in.IDPI.CustomPatterns)
	out.AutoSolver.Solvers = cloneStringSlice(in.AutoSolver.Solvers)
	out.Proxy = cloneBrowserProxyConfig(in.Proxy)
	out.Targets = cloneBrowserTargetsConfig(in.Targets)
	return &out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func TargetsForBrowser(cfg *RuntimeConfig, browser string) []string {
	if cfg == nil || len(cfg.Targets) == 0 {
		return nil
	}
	browser = NormalizeBrowser(browser)
	var names []string
	for name, t := range cfg.Targets {
		if NormalizeBrowser(t.Provider) == browser {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// AmbiguousBrowserError is returned when a resolved browser name matches
// multiple configured targets and no defaultTarget disambiguates.
type AmbiguousBrowserError struct {
	Browser string
	Targets []string
}

func (e *AmbiguousBrowserError) Error() string {
	return fmt.Sprintf(
		"browser %q matches multiple targets (%s); set browser.defaultTarget or specify a target explicitly",
		e.Browser, strings.Join(e.Targets, ", "))
}

func ResolveDefaultTarget(cfg *RuntimeConfig) string {
	if cfg == nil || len(cfg.Targets) == 0 {
		return ""
	}
	if dt := strings.TrimSpace(cfg.DefaultTarget); dt != "" {
		if _, ok := cfg.Targets[dt]; ok {
			return dt
		}
	}
	if len(cfg.Targets) == 1 {
		for name := range cfg.Targets {
			return name
		}
	}
	return ""
}

// MatchBrowserToTarget maps a provider/browser name to a single configured
// target name using the prefer-configured-default tie-break shared by the
// orchestrator's request resolution and the launch path: exactly one match
// wins; multiple matches resolve to the configured default target when it is
// among them. It returns ("", matches) when there is no unambiguous winner —
// zero matches (matches empty) or several with no default among them — so each
// caller applies its own policy (error vs. lenient fallthrough). matches is the
// full TargetsForBrowser list, for callers that need to build an ambiguity error.
func MatchBrowserToTarget(cfg *RuntimeConfig, browser string) (target string, matches []string) {
	if cfg == nil || len(cfg.Targets) == 0 {
		return "", nil
	}
	matches = TargetsForBrowser(cfg, browser)
	switch len(matches) {
	case 1:
		return matches[0], matches
	case 0:
		return "", matches
	default:
		dt := ResolveDefaultTarget(cfg)
		for _, m := range matches {
			if m == dt {
				return dt, matches
			}
		}
		return "", matches
	}
}

func ResolveDefaultBrowserTarget(cfg *RuntimeConfig) (*ResolvedBrowserTarget, error) {
	if cfg == nil || len(cfg.Targets) == 0 {
		return &ResolvedBrowserTarget{
			Provider: NormalizeBrowser(runtimeBrowser(cfg)),
			Config:   CloneRuntimeConfig(cfg),
			Legacy:   true,
		}, nil
	}

	name := ResolveDefaultTarget(cfg)
	if name == "" {
		return nil, fmt.Errorf("no default browser target configured and none requested")
	}
	return resolveBrowserTargetByName(cfg, name)
}

func ResolveExplicitBrowserTarget(cfg *RuntimeConfig, requested string) (*ResolvedBrowserTarget, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return nil, fmt.Errorf("browser target name is required")
	}
	if cfg == nil || len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("no browser targets configured; cannot resolve %q", requested)
	}
	if !IsValidBrowserTargetName(requested) {
		return nil, fmt.Errorf("invalid browser target name %q (must match ^[a-z][a-z0-9-]{0,31}$)", requested)
	}
	return resolveBrowserTargetByName(cfg, requested)
}

func resolveBrowserTargetByName(cfg *RuntimeConfig, name string) (*ResolvedBrowserTarget, error) {
	t, ok := cfg.Targets[name]
	if !ok {
		return nil, fmt.Errorf("browser target %q not found", name)
	}
	t = cloneBrowserTarget(t)
	effective := CloneRuntimeConfig(cfg)
	applyTargetToRuntime(effective, t)
	return &ResolvedBrowserTarget{
		Name:     name,
		Provider: effective.DefaultBrowser,
		Target:   t,
		Config:   effective,
	}, nil
}

func runtimeBrowser(cfg *RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.DefaultBrowser
}

func applyTargetToRuntime(out *RuntimeConfig, t BrowserTargetConfig) {
	if out == nil {
		return
	}
	out.DefaultBrowser = NormalizeBrowser(t.Provider)
	if strings.TrimSpace(t.Binary) != "" {
		out.BrowserBinary = t.Binary
	}
	if strings.TrimSpace(t.ExtraFlags) != "" {
		out.BrowserExtraFlags = t.ExtraFlags
	}
	out.Cloak = mergeCloakConfig(out.Cloak, t.Cloak)
	if !t.Proxy.IsZero() {
		out.Proxy = cloneBrowserProxyConfig(t.Proxy)
	}
}

// ResolveRequestedTarget validates a public target parameter. New call sites
// should use ResolveDefaultBrowserTarget or ResolveExplicitBrowserTarget so the
// empty-string semantics are explicit.
func ResolveRequestedTarget(cfg *RuntimeConfig, requested string) (targetName, provider string, err error) {
	var resolved *ResolvedBrowserTarget
	if strings.TrimSpace(requested) == "" {
		resolved, err = ResolveDefaultBrowserTarget(cfg)
	} else {
		resolved, err = ResolveExplicitBrowserTarget(cfg, requested)
	}
	if err != nil || resolved == nil {
		return "", "", err
	}
	if resolved.Legacy {
		return "", "", nil
	}
	return resolved.Name, resolved.Provider, nil
}

// ApplyTargetOverride is a legacy compatibility wrapper for older tests and
// callers. New code should use ResolveDefaultBrowserTarget or
// ResolveExplicitBrowserTarget and pass the returned effective Config through
// the launch path.
func ApplyTargetOverride(cfg *RuntimeConfig, targetName string) *RuntimeConfig {
	if cfg == nil {
		return cfg
	}
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		return cfg
	}
	resolved, err := ResolveExplicitBrowserTarget(cfg, targetName)
	if err != nil {
		return cfg
	}
	return resolved.Config
}

// mergeCloakConfig deep-merges target over global: target wins on non-zero/non-nil fields.
func mergeCloakConfig(global CloakBrowserRuntimeConfig, target CloakBrowserConfig) CloakBrowserRuntimeConfig {
	out := global
	if target.FingerprintSeed != "" {
		out.FingerprintSeed = target.FingerprintSeed
	}
	if target.Platform != "" {
		out.Platform = target.Platform
	}
	if target.Locale != "" {
		out.Locale = target.Locale
	}
	if target.Timezone != "" {
		out.Timezone = target.Timezone
	}
	if target.WebRTCIP != "" {
		out.WebRTCIP = target.WebRTCIP
	}
	if target.FontsDir != "" {
		out.FontsDir = target.FontsDir
	}
	if target.StorageQuotaMB != nil {
		out.StorageQuotaMB = *target.StorageQuotaMB
	}
	if target.DisableDefaultStealthArgs != nil {
		out.DisableDefaultStealthArgs = *target.DisableDefaultStealthArgs
	}
	return out
}
