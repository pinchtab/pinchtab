package stealth

import (
	goruntime "runtime"
	"sort"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/host"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
)

type BrandVersion struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

type UserAgentDataProfile struct {
	Brands          []BrandVersion `json:"brands"`
	FullVersionList []BrandVersion `json:"fullVersionList"`
	Mobile          bool           `json:"mobile"`
	Platform        string         `json:"platform"`
	PlatformVersion string         `json:"platformVersion"`
	Architecture    string         `json:"architecture"`
	Bitness         string         `json:"bitness"`
	Model           string         `json:"model"`
	Wow64           bool           `json:"wow64"`
}

type BrowserPersona struct {
	UserAgent         string               `json:"userAgent"`
	Language          string               `json:"language"`
	Languages         []string             `json:"languages"`
	AcceptLanguage    string               `json:"acceptLanguage"`
	NavigatorPlatform string               `json:"navigatorPlatform"`
	UserAgentData     UserAgentDataProfile `json:"userAgentData"`
}

// ReducedBrowserVersion returns the UA-reduced (v100+) "<major>.0.0.0" form
// of a Chrome version string. navigator.userAgent in real Chrome never carries
// the full build — exposing it is a fingerprint tell. The full build still
// lives in the high-entropy UA Client Hints. Falls back to the default major
// when chromeVersion is empty so all callers agree on a single value.
func ReducedBrowserVersion(chromeVersion string) string {
	if chromeVersion == "" {
		chromeVersion = browserprobe.FallbackChromeVersion
	}
	major := chromeVersion
	if i := strings.Index(chromeVersion, "."); i > 0 {
		major = chromeVersion[:i]
	}
	return major + ".0.0.0"
}

// The Sec-CH-UA brand vocabulary. A Chromium build sends its own vendor brand next to
// "Chromium" and a GREASE placeholder, and the vendor brand is what a detector
// cross-checks against the UA string: Edge sends "Microsoft Edge" where Chrome sends
// "Google Chrome", so an Edg/ UA shipping Chrome brands describes a browser that cannot
// exist.
const (
	BrandChrome        = "Google Chrome"
	BrandEdge          = "Microsoft Edge"
	brandChromium      = "Chromium"
	brandGrease        = "Not(A:Brand"
	greaseMajorVersion = "99"
	greaseFullVersion  = "99.0.0.0"
)

// brandList is the brand vocabulary a UA claims, read back out of the string the same way
// the platform is. A UA naming no Chromium build gets no brands at all: real Safari sends
// no Sec-CH-UA header, so brands there are the tell rather than the cover, and the callers
// that build CDP metadata omit it entirely rather than describing a Chrome that is not
// there.
func brandList(ua, version, greaseVersion string) []BrandVersion {
	vendor, ok := chromiumVendorBrand(ua)
	if !ok {
		return nil
	}
	return []BrandVersion{
		{Brand: brandGrease, Version: greaseVersion},
		{Brand: vendor, Version: version},
		{Brand: brandChromium, Version: version},
	}
}

func chromiumVendorBrand(ua string) (string, bool) {
	switch {
	case strings.Contains(ua, "Edg/"):
		return BrandEdge, true
	case strings.Contains(ua, "Chrome/"):
		return BrandChrome, true
	}
	return "", false
}

// The platform vocabulary of the persona: the same values UserAgentDataProfile
// reports and PlatformVersionFor is keyed on, so a caller names a platform once
// and every UA fact follows from it.
const (
	PlatformWindows = "Windows"
	PlatformMacOS   = "macOS"
	PlatformLinux   = "Linux"
)

// ChromeUserAgent is the only place the Chrome UA string is spelled out. It is
// keyed on a platform rather than on GOOS because its two callers select the
// platform differently — the launch persona from the host, the fingerprint
// endpoint from the request — and that selection is theirs, not the template's.
// The frozen OS tokens (Mac OS X 10_15_7, Windows NT 10.0) are what Chrome's UA
// reduction pins; they have changed before, and a second copy of them is a drift
// waiting to happen. Callers pass a version already reduced by
// ReducedBrowserVersion; handing it a full build emits a UA real Chrome never
// sends.
func ChromeUserAgent(platform, chromeVersion string) string {
	switch platform {
	case PlatformMacOS:
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	case PlatformWindows:
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	default:
		return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	}
}

// EdgeUserAgent decorates a Chrome UA with Edge's own token, which is what Edge
// actually sends: the Chrome string plus Edg/<version>. Building it as a fourth
// template would put the OS tokens in one more place.
func EdgeUserAgent(chromeUserAgent, chromeVersion string) string {
	return chromeUserAgent + " Edg/" + chromeVersion
}

// HostPlatform is the persona platform of the machine this process runs on.
func HostPlatform() string {
	switch goruntime.GOOS {
	case "darwin":
		return PlatformMacOS
	case "windows":
		return PlatformWindows
	default:
		return PlatformLinux
	}
}

// The /fingerprint endpoint keys its matrix by these lowercase os names while the
// persona vocabulary above spells the same three platforms "Windows", "macOS" and
// "Linux". Two spellings of one thing drift, so the endpoint's names are DERIVED
// from the persona's here rather than written a second time: fingerprintOSKeys is
// the only place the two meet, and the endpoint keys its matrix by these constants.
const (
	FingerprintOSWindows = "windows"
	FingerprintOSMac     = "mac"
	FingerprintOSLinux   = "linux"
)

var fingerprintOSKeys = map[string]string{
	PlatformWindows: FingerprintOSWindows,
	PlatformMacOS:   FingerprintOSMac,
	PlatformLinux:   FingerprintOSLinux,
}

// FingerprintOSKey is the endpoint's os key for a persona platform.
func FingerprintOSKey(platform string) (string, bool) {
	key, ok := fingerprintOSKeys[platform]
	return key, ok
}

// FingerprintPlatforms is every persona platform the endpoint's os keys are derived
// from, sorted. A census over the vocabulary reads this rather than restating the
// platforms, so a row added to fingerprintOSKeys is visited without an edit.
func FingerprintPlatforms() []string {
	platforms := make([]string, 0, len(fingerprintOSKeys))
	for platform := range fingerprintOSKeys {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	return platforms
}

// HostFingerprintOS is the endpoint's os key for the machine this process runs on,
// which is what a fingerprint request naming no os means: the caller delegated the
// choice, and the honest answer is this host. A Linux host does advertise a rarer
// desktop population than Windows would, and coherence still wins — a Linux browser
// claiming Windows contradicts everything else it leaks (fonts, WebGL, UA-CH
// platform), which is louder than a rare but consistent platform. The launch persona
// already resolves from the host through GOOS, so defaulting the endpoint to the
// host is what makes the two agree. This is not the same question as os: "random",
// which means "surprise me within the usual desktop population" and deliberately
// draws over the weighted rows only.
func HostFingerprintOS() string {
	key, _ := FingerprintOSKey(HostPlatform())
	return key
}

// ResolveFingerprintOS translates a requested os into the endpoint's key, accepting
// either vocabulary in any case — "macOS" and "mac" and "MACOS" all resolve. It is
// an alias translation rather than case folding because "macOS" to "mac" is not a
// case difference, and "macOS" is the spelling a caller who has read PlatformMacOS
// will send. It reports false for anything else, so a genuinely unknown os stays a
// refusal that names what the caller sent.
func ResolveFingerprintOS(requested string) (string, bool) {
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" {
		return "", false
	}
	for platform, key := range fingerprintOSKeys {
		if strings.EqualFold(trimmed, platform) || strings.EqualFold(trimmed, key) {
			return key, true
		}
	}
	return "", false
}

// resolveUserAgent honours an explicit custom UA verbatim and otherwise builds the
// host's. It stays unexported: the only correct entry point is BuildPersona, which
// reduces the version first — an exported form invites a caller to pass
// cfg.BrowserVersion straight through and reintroduce the version drift the
// fingerprint endpoint already had to be resynced for by hand.
//
// A blank-but-present custom UA counts as absent. Returning it verbatim made the
// persona's UserAgent empty, and the launch path applies that field to
// Emulation.setUserAgentOverride directly — so a configured "  " blanked
// navigator.userAgent on every page, which is the one thing the whole persona exists
// to avoid. It also makes the guarantee the rotate path's guard relies on true
// without exception: persona.UserAgent is never empty.
func resolveUserAgent(userAgent, chromeVersion string) string {
	if strings.TrimSpace(userAgent) != "" {
		return userAgent
	}
	return ChromeUserAgent(HostPlatform(), chromeVersion)
}

func BuildPersona(userAgent, chromeVersion string) BrowserPersona {
	reduced := ReducedBrowserVersion(chromeVersion)
	major := reduced
	if i := strings.Index(reduced, "."); i > 0 {
		major = reduced[:i]
	}
	// Freeze the UA build to <major>.0.0.0. Real Chrome (UA reduction, v100+) never
	// exposes the full build in navigator.userAgent — a UA like "Chrome/146.0.7680.80"
	// is a fingerprint tell. The full build still lives in the high-entropy UA-CH
	// (uaFullVersion / fullVersionList). An explicit custom userAgent is respected
	// verbatim by resolveUserAgent.
	ua := resolveUserAgent(userAgent, reduced)
	language := "en-US"
	languages := []string{"en-US", "en"}
	acceptLanguage := "en-US,en"

	// The platform is read back out of the UA rather than carried down from the
	// caller, and that is not a round trip: on the custom-UA path no platform was
	// ever selected, so the string is the only thing that knows. A custom Windows UA
	// on a mac host must report Win32, which is why this sniff cannot be replaced by
	// passing the generated platform through.
	navigatorPlatform := "Linux x86_64"
	uaDataPlatform := "Linux"
	switch {
	case strings.Contains(ua, "Windows"):
		navigatorPlatform = "Win32"
		uaDataPlatform = "Windows"
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		navigatorPlatform = "MacIntel"
		uaDataPlatform = "macOS"
	}
	platformVersion := PlatformVersionFor(uaDataPlatform)

	architecture := architectureFor(ua, uaDataPlatform)

	brands := brandList(ua, major, greaseMajorVersion)
	fullVersionList := brandList(ua, chromeVersionOrFallback(chromeVersion), greaseFullVersion)

	return BrowserPersona{
		UserAgent:         ua,
		Language:          language,
		Languages:         languages,
		AcceptLanguage:    acceptLanguage,
		NavigatorPlatform: navigatorPlatform,
		UserAgentData: UserAgentDataProfile{
			Brands:          brands,
			FullVersionList: fullVersionList,
			Mobile:          false,
			Platform:        uaDataPlatform,
			PlatformVersion: platformVersion,
			Architecture:    architecture,
			Bitness:         "64",
			Model:           "",
			Wow64:           false,
		},
	}
}

var defaultPlatformVersions = map[string]string{
	"Windows": "15.0.0",
	"macOS":   "14.0.0",
	"Linux":   "6.5.0",
}

var hostProductVersion = sync.OnceValue(func() string {
	_, _, version, err := host.PlatformInformation()
	if err != nil {
		return ""
	}
	return version
})

var hostKernelVersion = sync.OnceValue(func() string {
	version, err := host.KernelVersion()
	if err != nil {
		return ""
	}
	return version
})

// PlatformVersionFor returns the UA-CH platformVersion to advertise for a
// userAgentData platform. Real Chrome reports the running host's OS version, so
// a persona describing this host reads it from the host — a frozen value
// contradicts the Sec-CH-UA-Platform-Version header Chrome itself sends. A
// persona describing a different platform keeps a plausible constant.
func PlatformVersionFor(uaDataPlatform string) string {
	if version := normalizePlatformVersion(hostPlatformVersion(uaDataPlatform)); version != "" {
		return version
	}
	return defaultPlatformVersions[uaDataPlatform]
}

// architectureFor derives the UA-CH architecture the persona advertises. The host is
// read only when the persona describes it — the gate hostPlatformVersion applies to
// the OS version — because real Chrome on Apple Silicon ships the "Intel Mac OS X" UA
// WITH architecture arm, so macOS is the one platform whose host value agrees with its
// UA. The UA-token branch is unreachable rather than dead: no shipped matrix UA carries
// an ARM token, so Windows and Linux report x86 today, and a future ARM matrix entry
// (which must also set the hardcoded bitness) is the supported way to add one.
func architectureFor(ua, uaDataPlatform string) string {
	return architectureForHost(ua, uaDataPlatform, goruntime.GOOS, goruntime.GOARCH)
}

func architectureForHost(ua, uaDataPlatform, hostOS, hostArch string) string {
	switch {
	case strings.Contains(ua, "arm64"), strings.Contains(ua, "aarch64"), strings.Contains(ua, "ARM"):
		return "arm"
	case uaDataPlatform == "macOS" && hostOS == "darwin" && hostArch == "arm64":
		return "arm"
	default:
		return "x86"
	}
}

func hostPlatformVersion(uaDataPlatform string) string {
	switch {
	case uaDataPlatform == "macOS" && goruntime.GOOS == "darwin":
		return hostProductVersion()
	case uaDataPlatform == "Linux" && goruntime.GOOS == "linux":
		return hostKernelVersion()
	default:
		return ""
	}
}

func normalizePlatformVersion(raw string) string {
	parts := make([]string, 0, 3)
	for field := range strings.SplitSeq(raw, ".") {
		digits := leadingDigits(field)
		if digits == "" {
			break
		}
		parts = append(parts, digits)
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".")
}

func leadingDigits(field string) string {
	for i := 0; i < len(field); i++ {
		if field[i] < '0' || field[i] > '9' {
			return field[:i]
		}
	}
	return field
}

func chromeVersionOrFallback(chromeVersion string) string {
	if chromeVersion != "" {
		return chromeVersion
	}
	return ReducedBrowserVersion("")
}
