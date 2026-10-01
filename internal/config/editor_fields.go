package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/safelog"
)

type fieldKind int

const (
	fieldString fieldKind = iota
	fieldInt
	fieldBool
	fieldList
	fieldReadOnly
	fieldRetired
)

type fieldSpec[T any] struct {
	key  string
	kind fieldKind
	get  func(*T) string
	set  func(t *T, path, value string) error
}

type nestedSpec[T any] struct {
	prefix string
	set    func(t *T, at, field, value string) error
	get    func(t *T, at, field string) (string, error)
	leaves func(at string) []fieldLeaf
}

type fieldTable[T any] struct {
	fields []fieldSpec[T]
	nested []nestedSpec[T]
}

type fieldLeaf struct {
	path string
	kind fieldKind
}

func (tbl fieldTable[T]) nestedFor(field string) (nestedSpec[T], string, bool) {
	for _, n := range tbl.nested {
		if rest, ok := strings.CutPrefix(field, n.prefix+"."); ok {
			return n, rest, true
		}
	}
	return nestedSpec[T]{}, "", false
}

func (tbl fieldTable[T]) lookup(at, field string) (fieldSpec[T], string, error) {
	path := at + "." + field
	for _, spec := range tbl.fields {
		if spec.key == field {
			return spec, path, nil
		}
	}
	return fieldSpec[T]{}, path, fmt.Errorf("unknown field %s", path)
}

func (tbl fieldTable[T]) set(t *T, at, field, value string) error {
	if n, rest, ok := tbl.nestedFor(field); ok {
		return n.set(t, at+"."+n.prefix, rest, value)
	}
	spec, path, err := tbl.lookup(at, field)
	if err != nil {
		return err
	}
	return spec.set(t, path, value)
}

func (tbl fieldTable[T]) get(t *T, at, field string) (string, error) {
	if n, rest, ok := tbl.nestedFor(field); ok {
		return n.get(t, at+"."+n.prefix, rest)
	}
	spec, path, err := tbl.lookup(at, field)
	if err != nil {
		return "", err
	}
	if spec.kind == fieldRetired {
		return "", spec.set(t, path, "")
	}
	return spec.get(t), nil
}

func (tbl fieldTable[T]) leaves(at string) []fieldLeaf {
	var out []fieldLeaf
	for _, spec := range tbl.fields {
		out = append(out, fieldLeaf{path: at + "." + spec.key, kind: spec.kind})
	}
	for _, n := range tbl.nested {
		out = append(out, n.leaves(at+"."+n.prefix)...)
	}
	return out
}

func nested[T, S any](prefix string, sub *fieldTable[S], slot func(*T) *S) nestedSpec[T] {
	return nestedSpec[T]{
		prefix: prefix,
		set:    func(t *T, at, field, value string) error { return sub.set(slot(t), at, field, value) },
		get:    func(t *T, at, field string) (string, error) { return sub.get(slot(t), at, field) },
		leaves: func(at string) []fieldLeaf { return sub.leaves(at) },
	}
}

func nestedPtr[T, S any](prefix string, sub *fieldTable[S], slot func(*T) **S) nestedSpec[T] {
	return nestedSpec[T]{
		prefix: prefix,
		set: func(t *T, at, field, value string) error {
			p := slot(t)
			if *p != nil {
				return sub.set(*p, at, field, value)
			}
			var fresh S
			if err := sub.set(&fresh, at, field, value); err != nil {
				return err
			}
			*p = &fresh
			return nil
		},
		get: func(t *T, at, field string) (string, error) {
			if p := *slot(t); p != nil {
				return sub.get(p, at, field)
			}
			var zero S
			return sub.get(&zero, at, field)
		},
		leaves: func(at string) []fieldLeaf { return sub.leaves(at) },
	}
}

func stringField[T any](key string, slot func(*T) *string) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldString,
		get:  func(t *T) string { return *slot(t) },
		set: func(t *T, _, value string) error {
			*slot(t) = value
			return nil
		},
	}
}

func validatedStringField[T any](key string, slot func(*T) *string, normalize func(path, value string) (string, error)) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldString,
		get:  func(t *T) string { return *slot(t) },
		set: func(t *T, path, value string) error {
			v, err := normalize(path, value)
			if err != nil {
				return err
			}
			*slot(t) = v
			return nil
		},
	}
}

func stringPtrField[T any](key string, slot func(*T) **string) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldString,
		get:  func(t *T) string { return formatStringPtr(*slot(t)) },
		set: func(t *T, _, value string) error {
			*slot(t) = &value
			return nil
		},
	}
}

func intField[T any](key string, slot func(*T) *int) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldInt,
		get:  func(t *T) string { return strconv.Itoa(*slot(t)) },
		set: func(t *T, path, value string) error {
			n, err := parseIntValue(path, value)
			if err != nil {
				return err
			}
			*slot(t) = n
			return nil
		},
	}
}

func intPtrField[T any](key string, slot func(*T) **int) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldInt,
		get:  func(t *T) string { return formatIntPtr(*slot(t)) },
		set: func(t *T, path, value string) error {
			n, err := parseIntValue(path, value)
			if err != nil {
				return err
			}
			*slot(t) = &n
			return nil
		},
	}
}

func boolField[T any](key string, slot func(*T) *bool) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldBool,
		get:  func(t *T) string { return strconv.FormatBool(*slot(t)) },
		set: func(t *T, path, value string) error {
			b, err := parseBoolValue(path, value)
			if err != nil {
				return err
			}
			*slot(t) = b
			return nil
		},
	}
}

func boolPtrField[T any](key string, slot func(*T) **bool) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldBool,
		get:  func(t *T) string { return formatBoolPtr(*slot(t)) },
		set: func(t *T, path, value string) error {
			b, err := parseBoolValue(path, value)
			if err != nil {
				return err
			}
			*slot(t) = &b
			return nil
		},
	}
}

func listField[T any](key string, slot func(*T) *[]string, validate func([]string) error) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldList,
		get:  func(t *T) string { return strings.Join(*slot(t), ",") },
		set: func(t *T, _, value string) error {
			items := parseCSVList(value)
			if validate != nil {
				if err := validate(items); err != nil {
					return err
				}
			}
			*slot(t) = items
			return nil
		},
	}
}

func readOnlyField[T any](key string, slot func(*T) *string, refusal error) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldReadOnly,
		get:  func(t *T) string { return *slot(t) },
		set:  func(*T, string, string) error { return refusal },
	}
}

func retiredField[T any](key string, refusal error) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldRetired,
		set:  func(*T, string, string) error { return refusal },
	}
}

func parseIntValue(path, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", path, err)
	}
	if err := checkIntRule(path, n); err != nil {
		return 0, err
	}
	return n, nil
}

func parseBoolValue(path, value string) (bool, error) {
	b, err := parseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", path, err)
	}
	return b, nil
}

func knownBrowserID(_, value string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(value))
	if _, ok := browsers.Get(id); !ok {
		return "", fmt.Errorf("unknown browser %q (known: %v)", value, browsers.IDs())
	}
	return id, nil
}

var serverFields = fieldTable[ServerConfig]{
	fields: []fieldSpec[ServerConfig]{
		stringField("port", func(s *ServerConfig) *string { return &s.Port }),
		stringField("bind", func(s *ServerConfig) *string { return &s.Bind }),
		stringField("token", func(s *ServerConfig) *string { return &s.Token }),
		stringField("stateDir", func(s *ServerConfig) *string { return &s.StateDir }),
		validatedStringField("logLevel", func(s *ServerConfig) *string { return &s.LogLevel }, func(path, value string) (string, error) {
			if _, err := safelog.ParseLevel(value); err != nil {
				return "", fmt.Errorf("%s: %w", path, err)
			}
			return strings.ToLower(strings.TrimSpace(value)), nil
		}),
		intPtrField("networkBufferSize", func(s *ServerConfig) **int { return &s.NetworkBufferSize }),
		boolPtrField("retainNetworkBodies", func(s *ServerConfig) **bool { return &s.RetainNetworkBodies }),
		intPtrField("retainNetworkBodyMaxBytes", func(s *ServerConfig) **int { return &s.RetainNetworkBodyMaxBytes }),
		boolPtrField("trustProxyHeaders", func(s *ServerConfig) **bool { return &s.TrustProxyHeaders }),
		{
			key:  "cookieSecure",
			kind: fieldBool,
			get:  func(s *ServerConfig) string { return formatBoolPtr(s.CookieSecure) },
			set: func(s *ServerConfig, path, value string) error {
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "", "auto", "null":
					s.CookieSecure = nil
					return nil
				}
				b, err := parseBool(value)
				if err != nil {
					return fmt.Errorf("%s must be true or false (or empty/auto/null to unset): %w", path, err)
				}
				s.CookieSecure = &b
				return nil
			},
		},
	},
}

var attachFields = fieldTable[AttachConfig]{
	fields: []fieldSpec[AttachConfig]{
		boolPtrField("enabled", func(a *AttachConfig) **bool { return &a.Enabled }),
		listField("allowHosts", func(a *AttachConfig) *[]string { return &a.AllowHosts }, nil),
		listField("allowSchemes", func(a *AttachConfig) *[]string { return &a.AllowSchemes }, nil),
		boolPtrField("forwardProxyAuth", func(a *AttachConfig) **bool { return &a.ForwardProxyAuth }),
	},
}

var idpiFields = fieldTable[IDPIConfig]{
	fields: []fieldSpec[IDPIConfig]{
		boolField("enabled", func(i *IDPIConfig) *bool { return &i.Enabled }),
		boolField("strictMode", func(i *IDPIConfig) *bool { return &i.StrictMode }),
		boolField("scanContent", func(i *IDPIConfig) *bool { return &i.ScanContent }),
		boolField("wrapContent", func(i *IDPIConfig) *bool { return &i.WrapContent }),
		listField("customPatterns", func(i *IDPIConfig) *[]string { return &i.CustomPatterns }, nil),
		intField("scanTimeoutSec", func(i *IDPIConfig) *int { return &i.ScanTimeoutSec }),
		intField("shieldThreshold", func(i *IDPIConfig) *int { return &i.ShieldThreshold }),
	},
}

var securityFields = fieldTable[SecurityConfig]{
	fields: []fieldSpec[SecurityConfig]{
		boolPtrField("allowEvaluate", func(s *SecurityConfig) **bool { return &s.AllowEvaluate }),
		boolPtrField("allowClipboard", func(s *SecurityConfig) **bool { return &s.AllowClipboard }),
		boolPtrField("allowMacro", func(s *SecurityConfig) **bool { return &s.AllowMacro }),
		boolPtrField("allowScreencast", func(s *SecurityConfig) **bool { return &s.AllowScreencast }),
		boolPtrField("allowDownload", func(s *SecurityConfig) **bool { return &s.AllowDownload }),
		boolPtrField("allowCookies", func(s *SecurityConfig) **bool { return &s.AllowCookies }),
		boolPtrField("allowStateExport", func(s *SecurityConfig) **bool { return &s.AllowStateExport }),
		boolPtrField("allowUpload", func(s *SecurityConfig) **bool { return &s.AllowUpload }),
		boolPtrField("allowNetworkIntercept", func(s *SecurityConfig) **bool { return &s.AllowNetworkIntercept }),
		boolPtrField("allowMemory", func(s *SecurityConfig) **bool { return &s.AllowMemory }),
		boolPtrField("allowFileScheme", func(s *SecurityConfig) **bool { return &s.AllowFileScheme }),
		boolPtrField("trustLoopbackProxy", func(s *SecurityConfig) **bool { return &s.TrustLoopbackProxy }),
		stringPtrField("stateEncryptionKey", func(s *SecurityConfig) **string { return &s.StateEncryptionKey }),
		listField("allowedDomains", func(s *SecurityConfig) *[]string { return &s.AllowedDomains }, validateAllowlistEntries),
		listField("downloadAllowedDomains", func(s *SecurityConfig) *[]string { return &s.DownloadAllowedDomains }, nil),
		listField("trustedProxyCIDRs", func(s *SecurityConfig) *[]string { return &s.TrustedProxyCIDRs }, nil),
		listField("trustedResolveCIDRs", func(s *SecurityConfig) *[]string { return &s.TrustedResolveCIDRs }, nil),
		intPtrField("downloadMaxBytes", func(s *SecurityConfig) **int { return &s.DownloadMaxBytes }),
		intPtrField("memorySnapshotMaxBytes", func(s *SecurityConfig) **int { return &s.MemorySnapshotMaxBytes }),
		intPtrField("uploadMaxRequestBytes", func(s *SecurityConfig) **int { return &s.UploadMaxRequestBytes }),
		intPtrField("uploadMaxFiles", func(s *SecurityConfig) **int { return &s.UploadMaxFiles }),
		intPtrField("uploadMaxFileBytes", func(s *SecurityConfig) **int { return &s.UploadMaxFileBytes }),
		intPtrField("uploadMaxTotalBytes", func(s *SecurityConfig) **int { return &s.UploadMaxTotalBytes }),
		intPtrField("maxRedirects", func(s *SecurityConfig) **int { return &s.MaxRedirects }),
	},
	nested: []nestedSpec[SecurityConfig]{
		nested("attach", &attachFields, func(s *SecurityConfig) *AttachConfig { return &s.Attach }),
		nestedPtr("idpi", &idpiFields, func(s *SecurityConfig) **IDPIConfig { return &s.IDPI }),
	},
}

var cloakBrowserFields = fieldTable[CloakBrowserConfig]{
	fields: []fieldSpec[CloakBrowserConfig]{
		stringField("fingerprintSeed", func(c *CloakBrowserConfig) *string { return &c.FingerprintSeed }),
		stringField("platform", func(c *CloakBrowserConfig) *string { return &c.Platform }),
		stringField("locale", func(c *CloakBrowserConfig) *string { return &c.Locale }),
		stringField("timezone", func(c *CloakBrowserConfig) *string { return &c.Timezone }),
		stringField("webrtcIP", func(c *CloakBrowserConfig) *string { return &c.WebRTCIP }),
		stringField("fontsDir", func(c *CloakBrowserConfig) *string { return &c.FontsDir }),
		intPtrField("storageQuotaMB", func(c *CloakBrowserConfig) **int { return &c.StorageQuotaMB }),
		boolPtrField("disableDefaultStealthArgs", func(c *CloakBrowserConfig) **bool { return &c.DisableDefaultStealthArgs }),
	},
}

var browserProxyGeoFields = fieldTable[BrowserProxyGeoConfig]{
	fields: []fieldSpec[BrowserProxyGeoConfig]{
		stringField("timezone", func(g *BrowserProxyGeoConfig) *string { return &g.Timezone }),
		stringField("locale", func(g *BrowserProxyGeoConfig) *string { return &g.Locale }),
		stringField("webrtcIP", func(g *BrowserProxyGeoConfig) *string { return &g.WebRTCIP }),
		stringField("countryISO", func(g *BrowserProxyGeoConfig) *string { return &g.CountryISO }),
	},
}

var browserProxyFields = fieldTable[BrowserProxyConfig]{
	fields: []fieldSpec[BrowserProxyConfig]{
		stringField("server", func(p *BrowserProxyConfig) *string { return &p.Server }),
		listField("bypassList", func(p *BrowserProxyConfig) *[]string { return &p.BypassList }, nil),
		stringField("username", func(p *BrowserProxyConfig) *string { return &p.Username }),
		stringField("password", func(p *BrowserProxyConfig) *string { return &p.Password }),
	},
	nested: []nestedSpec[BrowserProxyConfig]{
		nestedPtr("geo", &browserProxyGeoFields, func(p *BrowserProxyConfig) **BrowserProxyGeoConfig { return &p.Geo }),
	},
}

var browserTargetFields = fieldTable[BrowserTargetConfig]{
	fields: []fieldSpec[BrowserTargetConfig]{
		validatedStringField("provider", func(t *BrowserTargetConfig) *string { return &t.Provider }, knownBrowserID),
		stringField("binary", func(t *BrowserTargetConfig) *string { return &t.Binary }),
		stringField("extraFlags", func(t *BrowserTargetConfig) *string { return &t.ExtraFlags }),
	},
	nested: []nestedSpec[BrowserTargetConfig]{
		nested("cloak", &cloakBrowserFields, func(t *BrowserTargetConfig) *CloakBrowserConfig { return &t.Cloak }),
		nested("proxy", &browserProxyFields, func(t *BrowserTargetConfig) *BrowserProxyConfig { return &t.Proxy }),
	},
}

var browserFields = fieldTable[BrowserConfig]{
	fields: []fieldSpec[BrowserConfig]{
		retiredField[BrowserConfig]("provider", errors.New("browser.provider is no longer supported; use browsers.default")),
		stringField("version", func(b *BrowserConfig) *string { return &b.BrowserVersion }),
		stringField("binary", func(b *BrowserConfig) *string { return &b.BrowserBinary }),
		stringField("extraFlags", func(b *BrowserConfig) *string { return &b.BrowserExtraFlags }),
		intPtrField("remoteDebuggingPort", func(b *BrowserConfig) **int { return &b.BrowserDebugPort }),
		listField("extensionPaths", func(b *BrowserConfig) *[]string { return &b.ExtensionPaths }, nil),
		stringField("defaultTarget", func(b *BrowserConfig) *string { return &b.DefaultTarget }),
		listField("fallbackOrder", func(b *BrowserConfig) *[]string { return &b.FallbackOrder }, nil),
	},
	nested: []nestedSpec[BrowserConfig]{
		nested("cloak", &cloakBrowserFields, func(b *BrowserConfig) *CloakBrowserConfig { return &b.Cloak }),
		nested("proxy", &browserProxyFields, func(b *BrowserConfig) *BrowserProxyConfig { return &b.Proxy }),
		{
			prefix: "targets",
			set: func(b *BrowserConfig, at, field, value string) error {
				return setBrowserTargetField(&b.Targets, at, field, value)
			},
			get: func(b *BrowserConfig, at, field string) (string, error) {
				return getBrowserTargetField(b.Targets, at, field)
			},
			leaves: func(string) []fieldLeaf { return nil },
		},
	},
}

var browsersFields = fieldTable[BrowsersConfig]{
	fields: []fieldSpec[BrowsersConfig]{
		validatedStringField("default", func(b *BrowsersConfig) *string { return &b.Default }, knownBrowserID),
		listField("available", func(b *BrowsersConfig) *[]string { return &b.Available }, nil),
	},
}

var tabPolicyFields = fieldTable[TabPolicyDefaults]{
	fields: []fieldSpec[TabPolicyDefaults]{
		stringField("eviction", func(tp *TabPolicyDefaults) *string { return &tp.Eviction }),
		stringField("lifecycle", func(tp *TabPolicyDefaults) *string { return &tp.Lifecycle }),
		intPtrField("closeDelaySec", func(tp *TabPolicyDefaults) **int { return &tp.CloseDelaySec }),
		boolPtrField("restore", func(tp *TabPolicyDefaults) **bool { return &tp.Restore }),
	},
}

var instanceDefaultsFields = fieldTable[InstanceDefaultsConfig]{
	fields: []fieldSpec[InstanceDefaultsConfig]{
		stringField("mode", func(c *InstanceDefaultsConfig) *string { return &c.Mode }),
		boolPtrField("noRestore", func(c *InstanceDefaultsConfig) **bool { return &c.NoRestore }),
		stringField("timezone", func(c *InstanceDefaultsConfig) *string { return &c.Timezone }),
		boolPtrField("blockImages", func(c *InstanceDefaultsConfig) **bool { return &c.BlockImages }),
		boolPtrField("blockMedia", func(c *InstanceDefaultsConfig) **bool { return &c.BlockMedia }),
		boolPtrField("blockAds", func(c *InstanceDefaultsConfig) **bool { return &c.BlockAds }),
		intPtrField("maxTabs", func(c *InstanceDefaultsConfig) **int { return &c.MaxTabs }),
		intPtrField("maxParallelTabs", func(c *InstanceDefaultsConfig) **int { return &c.MaxParallelTabs }),
		stringField("userAgent", func(c *InstanceDefaultsConfig) *string { return &c.UserAgent }),
		boolPtrField("noAnimations", func(c *InstanceDefaultsConfig) **bool { return &c.NoAnimations }),
		boolPtrField("captureAllowActivation", func(c *InstanceDefaultsConfig) **bool { return &c.CaptureAllowActivation }),
		boolPtrField("humanize", func(c *InstanceDefaultsConfig) **bool { return &c.Humanize }),
		stringField("stealthLevel", func(c *InstanceDefaultsConfig) *string { return &c.StealthLevel }),
		stringField("tabEvictionPolicy", func(c *InstanceDefaultsConfig) *string { return &c.TabEvictionPolicy }),
		boolPtrField("dialogAutoAccept", func(c *InstanceDefaultsConfig) **bool { return &c.DialogAutoAccept }),
	},
	nested: []nestedSpec[InstanceDefaultsConfig]{
		nestedPtr("tabPolicy", &tabPolicyFields, func(c *InstanceDefaultsConfig) **TabPolicyDefaults { return &c.TabPolicy }),
	},
}

var profilesFields = fieldTable[ProfilesConfig]{
	fields: []fieldSpec[ProfilesConfig]{
		stringField("baseDir", func(p *ProfilesConfig) *string { return &p.BaseDir }),
		stringField("defaultProfile", func(p *ProfilesConfig) *string { return &p.DefaultProfile }),
		intPtrField("quarantineKeep", func(p *ProfilesConfig) **int { return &p.QuarantineKeep }),
	},
}

var multiInstanceRestartFields = fieldTable[MultiInstanceRestartConfig]{
	fields: []fieldSpec[MultiInstanceRestartConfig]{
		intPtrField("maxRestarts", func(r *MultiInstanceRestartConfig) **int { return &r.MaxRestarts }),
		intPtrField("initBackoffSec", func(r *MultiInstanceRestartConfig) **int { return &r.InitBackoffSec }),
		intPtrField("maxBackoffSec", func(r *MultiInstanceRestartConfig) **int { return &r.MaxBackoffSec }),
		intPtrField("stableAfterSec", func(r *MultiInstanceRestartConfig) **int { return &r.StableAfterSec }),
	},
}

var multiInstanceFields = fieldTable[MultiInstanceConfig]{
	fields: []fieldSpec[MultiInstanceConfig]{
		stringField("strategy", func(o *MultiInstanceConfig) *string { return &o.Strategy }),
		intPtrField("instancePortStart", func(o *MultiInstanceConfig) **int { return &o.InstancePortStart }),
		intPtrField("instancePortEnd", func(o *MultiInstanceConfig) **int { return &o.InstancePortEnd }),
	},
	nested: []nestedSpec[MultiInstanceConfig]{
		nested("restart", &multiInstanceRestartFields, func(o *MultiInstanceConfig) *MultiInstanceRestartConfig { return &o.Restart }),
	},
}

var timeoutsFields = fieldTable[TimeoutsConfig]{
	fields: []fieldSpec[TimeoutsConfig]{
		intField("actionSec", func(t *TimeoutsConfig) *int { return &t.ActionSec }),
		intField("navigateSec", func(t *TimeoutsConfig) *int { return &t.NavigateSec }),
		intField("shutdownSec", func(t *TimeoutsConfig) *int { return &t.ShutdownSec }),
		intField("waitNavMs", func(t *TimeoutsConfig) *int { return &t.WaitNavMs }),
	},
}

var schedulerFields = fieldTable[SchedulerFileConfig]{
	fields: []fieldSpec[SchedulerFileConfig]{
		boolPtrField("enabled", func(s *SchedulerFileConfig) **bool { return &s.Enabled }),
		stringField("strategy", func(s *SchedulerFileConfig) *string { return &s.Strategy }),
		intPtrField("maxQueueSize", func(s *SchedulerFileConfig) **int { return &s.MaxQueueSize }),
		intPtrField("maxPerAgent", func(s *SchedulerFileConfig) **int { return &s.MaxPerAgent }),
		intPtrField("maxInflight", func(s *SchedulerFileConfig) **int { return &s.MaxInflight }),
		intPtrField("maxPerAgentInflight", func(s *SchedulerFileConfig) **int { return &s.MaxPerAgentFlight }),
		intPtrField("resultTTLSec", func(s *SchedulerFileConfig) **int { return &s.ResultTTLSec }),
		intPtrField("workerCount", func(s *SchedulerFileConfig) **int { return &s.WorkerCount }),
		intPtrField("maxBatchSize", func(s *SchedulerFileConfig) **int { return &s.MaxBatchSize }),
	},
}

var activityEventFields = fieldTable[ActivityEventsFileConfig]{
	fields: []fieldSpec[ActivityEventsFileConfig]{
		boolPtrField("dashboard", func(e *ActivityEventsFileConfig) **bool { return &e.Dashboard }),
		boolPtrField("server", func(e *ActivityEventsFileConfig) **bool { return &e.Server }),
		boolPtrField("bridge", func(e *ActivityEventsFileConfig) **bool { return &e.Bridge }),
		boolPtrField("orchestrator", func(e *ActivityEventsFileConfig) **bool { return &e.Orchestrator }),
		boolPtrField("scheduler", func(e *ActivityEventsFileConfig) **bool { return &e.Scheduler }),
		boolPtrField("mcp", func(e *ActivityEventsFileConfig) **bool { return &e.MCP }),
		boolPtrField("other", func(e *ActivityEventsFileConfig) **bool { return &e.Other }),
	},
}

var activityFields = fieldTable[ActivityFileConfig]{
	fields: []fieldSpec[ActivityFileConfig]{
		boolPtrField("enabled", func(a *ActivityFileConfig) **bool { return &a.Enabled }),
		intPtrField("sessionIdleSec", func(a *ActivityFileConfig) **int { return &a.SessionIdleSec }),
		intPtrField("retentionDays", func(a *ActivityFileConfig) **int { return &a.RetentionDays }),
		readOnlyField("stateDir", func(a *ActivityFileConfig) *string { return &a.StateDir }, errors.New(ActivityStateDirRefusal)),
	},
	nested: []nestedSpec[ActivityFileConfig]{
		nested("events", &activityEventFields, func(a *ActivityFileConfig) *ActivityEventsFileConfig { return &a.Events }),
	},
}

var observabilityFields = fieldTable[ObservabilityFileConfig]{
	nested: []nestedSpec[ObservabilityFileConfig]{
		nested("activity", &activityFields, func(o *ObservabilityFileConfig) *ActivityFileConfig { return &o.Activity }),
	},
}

var agentSessionFields = fieldTable[AgentSessionFileConfig]{
	fields: []fieldSpec[AgentSessionFileConfig]{
		boolPtrField("enabled", func(s *AgentSessionFileConfig) **bool { return &s.Enabled }),
		stringField("mode", func(s *AgentSessionFileConfig) *string { return &s.Mode }),
		intPtrField("idleTimeoutSec", func(s *AgentSessionFileConfig) **int { return &s.IdleTimeoutSec }),
		intPtrField("maxLifetimeSec", func(s *AgentSessionFileConfig) **int { return &s.MaxLifetimeSec }),
	},
}

var dashboardSessionFields = fieldTable[DashboardSessionFileConfig]{
	fields: []fieldSpec[DashboardSessionFileConfig]{
		boolPtrField("persist", func(s *DashboardSessionFileConfig) **bool { return &s.Persist }),
		intPtrField("idleTimeoutSec", func(s *DashboardSessionFileConfig) **int { return &s.IdleTimeoutSec }),
		intPtrField("maxLifetimeSec", func(s *DashboardSessionFileConfig) **int { return &s.MaxLifetimeSec }),
		intPtrField("elevationWindowSec", func(s *DashboardSessionFileConfig) **int { return &s.ElevationWindowSec }),
		boolPtrField("persistElevationAcrossRestart", func(s *DashboardSessionFileConfig) **bool { return &s.PersistElevationAcrossRestart }),
		boolPtrField("requireElevation", func(s *DashboardSessionFileConfig) **bool { return &s.RequireElevation }),
	},
}

var sessionsFields = fieldTable[SessionsFileConfig]{
	nested: []nestedSpec[SessionsFileConfig]{
		nested("dashboard", &dashboardSessionFields, func(s *SessionsFileConfig) *DashboardSessionFileConfig { return &s.Dashboard }),
		nested("agent", &agentSessionFields, func(s *SessionsFileConfig) *AgentSessionFileConfig { return &s.Agent }),
	},
}

var autoSolverExternalFields = fieldTable[AutoSolverExtConf]{
	fields: []fieldSpec[AutoSolverExtConf]{
		stringField("capsolverKey", func(e *AutoSolverExtConf) *string { return &e.CapsolverKey }),
		stringField("twoCaptchaKey", func(e *AutoSolverExtConf) *string { return &e.TwoCaptchaKey }),
	},
}

var autoSolverCredentialsFields = fieldTable[AutoSolverCredentialsConf]{
	fields: []fieldSpec[AutoSolverCredentialsConf]{
		stringField("login.user", func(c *AutoSolverCredentialsConf) *string { return &c.Login.User }),
		stringField("login.password", func(c *AutoSolverCredentialsConf) *string { return &c.Login.Password }),
		stringField("signup.name", func(c *AutoSolverCredentialsConf) *string { return &c.Signup.Name }),
		stringField("signup.email", func(c *AutoSolverCredentialsConf) *string { return &c.Signup.Email }),
		stringField("signup.password", func(c *AutoSolverCredentialsConf) *string { return &c.Signup.Password }),
		stringField("form.field1", func(c *AutoSolverCredentialsConf) *string { return &c.Form.Field1 }),
		stringField("form.field2", func(c *AutoSolverCredentialsConf) *string { return &c.Form.Field2 }),
		stringField("form.email", func(c *AutoSolverCredentialsConf) *string { return &c.Form.Email }),
	},
}

var autoSolverFields = fieldTable[AutoSolverFileConfig]{
	fields: []fieldSpec[AutoSolverFileConfig]{
		boolPtrField("enabled", func(a *AutoSolverFileConfig) **bool { return &a.Enabled }),
		boolPtrField("autoTrigger", func(a *AutoSolverFileConfig) **bool { return &a.AutoTrigger }),
		boolPtrField("triggerOnNavigate", func(a *AutoSolverFileConfig) **bool { return &a.TriggerOnNavigate }),
		boolPtrField("triggerOnAction", func(a *AutoSolverFileConfig) **bool { return &a.TriggerOnAction }),
		boolPtrField("llmFallback", func(a *AutoSolverFileConfig) **bool { return &a.LLMFallback }),
		intPtrField("maxAttempts", func(a *AutoSolverFileConfig) **int { return &a.MaxAttempts }),
		intPtrField("solverTimeoutSec", func(a *AutoSolverFileConfig) **int { return &a.SolverTimeoutSec }),
		intPtrField("retryBaseDelayMs", func(a *AutoSolverFileConfig) **int { return &a.RetryBaseDelayMs }),
		intPtrField("retryMaxDelayMs", func(a *AutoSolverFileConfig) **int { return &a.RetryMaxDelayMs }),
		stringField("llmProvider", func(a *AutoSolverFileConfig) *string { return &a.LLMProvider }),
		listField("solvers", func(a *AutoSolverFileConfig) *[]string { return &a.Solvers }, nil),
	},
	nested: []nestedSpec[AutoSolverFileConfig]{
		nested("external", &autoSolverExternalFields, func(a *AutoSolverFileConfig) *AutoSolverExtConf { return &a.External }),
		nested("credentials", &autoSolverCredentialsFields, func(a *AutoSolverFileConfig) *AutoSolverCredentialsConf { return &a.Credentials }),
	},
}
