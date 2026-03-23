package stealth

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/emulation"
	"github.com/pinchtab/pinchtab/internal/config"
)

// BuildUserAgentOverride creates a SetUserAgentOverride action with persona-backed
// metadata. chromeVersion should be the full version (for example
// "144.0.7559.133"). If chromeVersion is empty, returns nil.
func BuildUserAgentOverride(userAgent, chromeVersion string) *emulation.SetUserAgentOverrideParams {
	if chromeVersion == "" {
		return nil
	}

	persona := BuildPersona(userAgent, chromeVersion)
	if persona.UserAgent == "" {
		return nil
	}

	brands := make([]*emulation.UserAgentBrandVersion, 0, len(persona.UserAgentData.Brands))
	for _, brand := range persona.UserAgentData.Brands {
		brands = append(brands, &emulation.UserAgentBrandVersion{
			Brand:   brand.Brand,
			Version: brand.Version,
		})
	}
	fullVersionList := make([]*emulation.UserAgentBrandVersion, 0, len(persona.UserAgentData.FullVersionList))
	for _, brand := range persona.UserAgentData.FullVersionList {
		fullVersionList = append(fullVersionList, &emulation.UserAgentBrandVersion{
			Brand:   brand.Brand,
			Version: brand.Version,
		})
	}

	return emulation.SetUserAgentOverride(persona.UserAgent).
		WithAcceptLanguage(persona.AcceptLanguage).
		WithPlatform(persona.NavigatorPlatform).
		WithUserAgentMetadata(&emulation.UserAgentMetadata{
			Platform:        persona.UserAgentData.Platform,
			PlatformVersion: persona.UserAgentData.PlatformVersion,
			Architecture:    persona.UserAgentData.Architecture,
			Bitness:         persona.UserAgentData.Bitness,
			Mobile:          persona.UserAgentData.Mobile,
			Brands:          brands,
			FullVersionList: fullVersionList,
		})
}

func BuildLocaleOverride(userAgent, chromeVersion string) *emulation.SetLocaleOverrideParams {
	persona := BuildPersona(userAgent, chromeVersion)
	if persona.Language == "" {
		return nil
	}
	return emulation.SetLocaleOverride().WithLocale(persona.Language)
}

// ApplyTargetEmulation applies the launch persona to a page/worker target so
// navigator.*, client hints, Accept-Language, and Intl locale stay coherent.
func ApplyTargetEmulation(ctx context.Context, cfg *config.RuntimeConfig, userAgent string) error {
	if cfg == nil {
		return nil
	}

	if err := emulation.SetAutomationOverride(false).Do(ctx); err != nil {
		return fmt.Errorf("automation override: %w", err)
	}

	if localeOverride := BuildLocaleOverride(userAgent, cfg.ChromeVersion); localeOverride != nil {
		if err := localeOverride.Do(ctx); err != nil {
			return fmt.Errorf("locale override: %w", err)
		}
	}

	if uaOverride := BuildUserAgentOverride(userAgent, cfg.ChromeVersion); uaOverride != nil {
		if err := uaOverride.Do(ctx); err != nil {
			return fmt.Errorf("user agent override: %w", err)
		}
	}

	return nil
}
