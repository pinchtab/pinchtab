package browsers

import "context"

// ---------------------------------------------------------------------------
// Provider-neutral config types used by the Browser interface.
// ---------------------------------------------------------------------------

// LaunchMode selects how a browser session is launched.
type LaunchMode string

// LaunchMode is internal — it controls how the bridge initializes the browser process.
// Not exposed in public config or API. Public selection uses browser provider names.
const (
	LaunchModeChrome LaunchMode = "chrome"
	LaunchModeLite   LaunchMode = "lite"
	LaunchModeAuto   LaunchMode = "auto"
)

// ResolveLaunchMode maps auto to chrome for launch purposes (the static browser
// is a separate runtime, not a Chromium variant). Unknown/empty values default
// to chrome.
func ResolveLaunchMode(m LaunchMode) LaunchMode {
	switch m {
	case LaunchModeChrome:
		return LaunchModeChrome
	case LaunchModeLite:
		return LaunchModeLite
	case LaunchModeAuto:
		return LaunchModeChrome
	default:
		return LaunchModeChrome
	}
}

// LaunchConfig carries provider-neutral parameters for launching a browser.
type LaunchConfig struct {
	Mode           LaunchMode
	Binary         string
	ProfileDir     string
	Proxy          ProxyConfig
	ExtraFlags     []string
	Headless       bool
	Timezone       string
	ExtensionPaths []string
	DebugPort      int
	NoRestore      bool
	UserAgent      string
	NoSandbox      bool
	Cloak          CloakFingerprint
}

// TargetConfig carries the resolved target a browser will be validated against.
type TargetConfig struct {
	Provider   string
	Binary     string
	ExtraFlags string
	Proxy      ProxyConfig
}

// ProxyConfig carries proxy settings for a browser launch.
type ProxyConfig struct {
	Server     string
	BypassList []string
	Username   string
	Password   string
	Geo        *GeoConfig
}

// GeoConfig carries geo-related settings for a launch.
type GeoConfig struct {
	Timezone   string
	Locale     string
	WebRTCIP   string
	CountryISO string

	OperatorTimezone string
	OperatorLocale   string
	OperatorWebRTCIP string
}

// BinaryDiscovery holds the result of searching for a browser binary.
type BinaryDiscovery struct {
	Found  string
	Probed []string
}

// GeoStrategy is the set of flags/env a browser derives from a GeoConfig.
type GeoStrategy struct {
	Flags        []string
	Env          []string
	OperatorWins bool // explicit user config overrides geo-derived values
}

// CloakFingerprint carries CloakBrowser-specific fingerprint settings.
type CloakFingerprint struct {
	FingerprintSeed string
	Platform        string
	Locale          string
	Timezone        string
	WebRTCIP        string
	FontsDir        string
	StorageQuotaMB  int
}

// DoctorStatus classifies the outcome of a provider health-check.
type DoctorStatus int

const (
	DoctorPass DoctorStatus = iota
	DoctorFail
	DoctorWarn
	DoctorSkip
)

// DoctorCheckResult is the outcome of a single provider health-check.
type DoctorCheckResult struct {
	Status DoctorStatus
	Detail string
	Err    error
}

// DoctorCheck is a single health-check a browser contributes to the doctor.
type DoctorCheck struct {
	ID          string
	Description string
	Fn          func(ctx context.Context, cfg interface{}) DoctorCheckResult
}

// DoctorEnv carries runtime configuration fields that provider doctor checks
// may need. It avoids browser sub-packages importing the config package.
type DoctorEnv struct {
	Binary string           // resolved browser binary path
	Cloak  CloakFingerprint // cloak fingerprint settings
}
