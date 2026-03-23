package stealth

import (
	goruntime "runtime"
	"strings"
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

func ResolveUserAgent(userAgent, chromeVersion string) string {
	if userAgent != "" {
		return userAgent
	}
	if chromeVersion == "" {
		return ""
	}

	switch goruntime.GOOS {
	case "darwin":
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	case "windows":
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	default:
		return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
	}
}

func BuildPersona(userAgent, chromeVersion string) BrowserPersona {
	ua := ResolveUserAgent(userAgent, chromeVersion)
	language := "en-US"
	languages := []string{"en-US", "en"}
	acceptLanguage := "en-US,en"
	if ua == "" {
		return BrowserPersona{
			Language:       language,
			Languages:      languages,
			AcceptLanguage: acceptLanguage,
		}
	}

	major := chromeVersion
	if i := strings.Index(chromeVersion, "."); i > 0 {
		major = chromeVersion[:i]
	}
	if major == "" {
		major = "144"
	}

	navigatorPlatform := "Linux x86_64"
	uaDataPlatform := "Linux"
	platformVersion := "6.5.0"
	switch {
	case strings.Contains(ua, "Windows"):
		navigatorPlatform = "Win32"
		uaDataPlatform = "Windows"
		platformVersion = "15.0.0"
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		navigatorPlatform = "MacIntel"
		uaDataPlatform = "macOS"
		platformVersion = "14.0.0"
	}

	architecture := "x86"
	switch {
	case strings.Contains(ua, "arm64"), strings.Contains(ua, "aarch64"), strings.Contains(ua, "ARM"):
		architecture = "arm"
	case goruntime.GOARCH == "arm64":
		architecture = "arm"
	}

	brands := []BrandVersion{
		{Brand: "Not(A:Brand", Version: "99"},
		{Brand: "Google Chrome", Version: major},
		{Brand: "Chromium", Version: major},
	}
	fullVersionList := []BrandVersion{
		{Brand: "Not(A:Brand", Version: "99.0.0.0"},
		{Brand: "Google Chrome", Version: chromeVersionOrFallback(chromeVersion)},
		{Brand: "Chromium", Version: chromeVersionOrFallback(chromeVersion)},
	}

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

func chromeVersionOrFallback(chromeVersion string) string {
	if chromeVersion != "" {
		return chromeVersion
	}
	return "144.0.0.0"
}
