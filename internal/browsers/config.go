package browsers

import "context"

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

type TargetConfig struct {
	Provider   string
	Binary     string
	ExtraFlags string
	Proxy      ProxyConfig
}

type ProxyConfig struct {
	Server     string
	BypassList []string
	Username   string
	Password   string
	Geo        *GeoConfig
}

type GeoConfig struct {
	Timezone   string
	Locale     string
	WebRTCIP   string
	CountryISO string

	OperatorTimezone string
	OperatorLocale   string
	OperatorWebRTCIP string
}

type BinaryDiscovery struct {
	Found  string
	Probed []string
}

type GeoStrategy struct {
	Flags        []string
	Env          []string
	OperatorWins bool // explicit user config overrides geo-derived values
}

type CloakFingerprint struct {
	FingerprintSeed string
	Platform        string
	Locale          string
	Timezone        string
	WebRTCIP        string
	FontsDir        string
	StorageQuotaMB  int
}

type DoctorStatus int

const (
	DoctorPass DoctorStatus = iota
	DoctorFail
	DoctorWarn
	DoctorSkip
)

type DoctorCheckResult struct {
	Status DoctorStatus
	Detail string
	Err    error
}

type DoctorCheck struct {
	ID          string
	Description string
	Fn          func(ctx context.Context, cfg interface{}) DoctorCheckResult
}

// DoctorEnv avoids browser sub-packages importing the config package.
type DoctorEnv struct {
	Binary string
	Cloak  CloakFingerprint
}
