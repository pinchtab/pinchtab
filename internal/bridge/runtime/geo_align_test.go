package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/geo"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestApplyGeoAlignment_ZeroInfo(t *testing.T) {
	flags, env := applyGeoAlignment(config.BrowserProviderChrome, geo.Info{}, config.CloakBrowserRuntimeConfig{})
	if len(flags) != 0 || len(env) != 0 {
		t.Fatalf("expected no flags/env for zero info, got flags=%v env=%v", flags, env)
	}
}

func TestResolveLaunchGeoAlignmentSkipsLookupWithoutProxyServer(t *testing.T) {
	old := geoProviderForConfigFunc
	t.Cleanup(func() { geoProviderForConfigFunc = old })
	geoProviderForConfigFunc = func(*config.RuntimeConfig) geo.Provider {
		t.Fatal("geo provider should not be constructed without a proxy server")
		return geo.Noop{}
	}

	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderChrome,
		Proxy: config.BrowserProxyConfig{
			Geo: &config.BrowserProxyGeoConfig{Timezone: "Europe/London"},
		},
	}
	got, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveLaunchGeoAlignment returned error: %v", err)
	}
	if !got.info.IsZero() || len(got.flags) != 0 || len(got.env) != 0 {
		t.Fatalf("expected empty alignment without proxy server, got %+v", got)
	}
}

func TestResolveLaunchGeoAlignmentPropagatesLookupError(t *testing.T) {
	old := geoProviderForConfigFunc
	t.Cleanup(func() { geoProviderForConfigFunc = old })

	sentinel := errors.New("geo boom")
	calls := 0
	geoProviderForConfigFunc = func(*config.RuntimeConfig) geo.Provider {
		return fakeGeoProvider{lookup: func(context.Context, string) (geo.Info, error) {
			calls++
			return geo.Info{}, sentinel
		}}
	}

	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderCloak,
		Proxy: config.BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &config.BrowserProxyGeoConfig{Timezone: "Europe/London"},
		},
	}
	_, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
	if calls != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls)
	}
}

func TestBuildChromeArgsWithBundleUsesProvidedGeoAlignment(t *testing.T) {
	old := geoProviderForConfigFunc
	t.Cleanup(func() { geoProviderForConfigFunc = old })
	geoProviderForConfigFunc = func(*config.RuntimeConfig) geo.Provider {
		t.Fatal("buildChromeArgsWithBundle should use cached alignment")
		return geo.Noop{}
	}

	args := buildChromeArgsWithBundle(&config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderCloak,
	}, nil, 9222, launchGeoAlignment{
		flags: []string{"--fingerprint-locale=en-GB"},
	})
	if !stealth.HasLaunchArg(args, "--fingerprint-locale=en-GB") {
		t.Fatalf("cached geo flag missing from args: %v", args)
	}
}

type fakeGeoProvider struct {
	lookup func(context.Context, string) (geo.Info, error)
}

func (f fakeGeoProvider) Lookup(ctx context.Context, ip string) (geo.Info, error) {
	return f.lookup(ctx, ip)
}

func TestResolveLaunchGeoAlignmentSkipsLookupForChromeProvider(t *testing.T) {
	old := geoProviderForConfigFunc
	t.Cleanup(func() { geoProviderForConfigFunc = old })
	geoProviderForConfigFunc = func(*config.RuntimeConfig) geo.Provider {
		t.Fatal("geo provider should not be constructed for the chrome provider")
		return geo.Noop{}
	}

	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderChrome,
		Proxy: config.BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &config.BrowserProxyGeoConfig{Timezone: "Europe/London", Locale: "en-GB"},
		},
	}
	got, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveLaunchGeoAlignment returned error: %v", err)
	}
	if !got.info.IsZero() || len(got.flags) != 0 || len(got.env) != 0 {
		t.Fatalf("expected empty alignment for chrome provider, got %+v", got)
	}
}

func TestApplyGeoAlignment_ChromeNoop(t *testing.T) {
	info := geo.Info{
		Timezone:   "Europe/London",
		Locale:     "en-GB",
		CountryISO: "GB",
	}
	flags, env := applyGeoAlignment(config.BrowserProviderChrome, info, config.CloakBrowserRuntimeConfig{})
	if len(flags) != 0 || len(env) != 0 {
		t.Fatalf("chrome provider should not receive proxy-derived geo flags/env, got flags=%v env=%v", flags, env)
	}
}

func TestApplyGeoAlignment_ChromeLocaleOnlyNoop(t *testing.T) {
	info := geo.Info{Locale: "en-GB"}
	flags, env := applyGeoAlignment(config.BrowserProviderChrome, info, config.CloakBrowserRuntimeConfig{})
	if len(flags) != 0 || len(env) != 0 {
		t.Fatalf("chrome provider should not receive proxy-derived geo flags/env, got flags=%v env=%v", flags, env)
	}
}

func TestApplyGeoAlignment_ChromeCountryOnly_NoFlags(t *testing.T) {
	// CountryISO is metadata only — it must not trigger flag emission.
	info := geo.Info{CountryISO: "GB"}
	flags, env := applyGeoAlignment(config.BrowserProviderChrome, info, config.CloakBrowserRuntimeConfig{})
	if len(flags) != 0 || len(env) != 0 {
		t.Fatalf("country-only info should produce no flags/env, got flags=%v env=%v", flags, env)
	}
}

func TestApplyGeoAlignment_Cloak(t *testing.T) {
	info := geo.Info{
		Timezone: "Europe/London",
		Locale:   "en-GB",
		WebRTCIP: "203.0.113.7",
	}
	flags, env := applyGeoAlignment(config.BrowserProviderCloak, info, config.CloakBrowserRuntimeConfig{})

	wantFlags := []string{
		"--fingerprint-timezone=Europe/London",
		"--fingerprint-locale=en-GB",
		"--fingerprint-webrtc-ip=203.0.113.7",
	}
	if !reflect.DeepEqual(flags, wantFlags) {
		t.Errorf("cloak flags = %v, want %v", flags, wantFlags)
	}
	if len(env) != 0 {
		t.Errorf("cloak env should be empty, got %v", env)
	}
}

func TestApplyGeoAlignment_CloakExplicitWinsOverDerivedGeo(t *testing.T) {
	info := geo.Info{
		Timezone: "Europe/London",
		Locale:   "en-GB",
		WebRTCIP: "203.0.113.7",
	}
	cloak := config.CloakBrowserRuntimeConfig{
		Timezone: "America/New_York", // explicit per-target override
	}
	flags, _ := applyGeoAlignment(config.BrowserProviderCloak, info, cloak)

	// Explicit Cloak timezone wins → no derived flag.
	for _, f := range flags {
		if f == "--fingerprint-timezone=Europe/London" {
			t.Errorf("derived timezone leaked despite explicit Cloak override: %v", flags)
		}
	}
	// Other derived fields still emitted (no explicit override).
	if !stealth.HasLaunchArg(flags, "--fingerprint-locale=en-GB") {
		t.Errorf("expected derived locale, got %v", flags)
	}
	if !stealth.HasLaunchArg(flags, "--fingerprint-webrtc-ip=203.0.113.7") {
		t.Errorf("expected derived webrtc-ip, got %v", flags)
	}
}

func TestApplyGeoAlignment_UnknownProviderNoop(t *testing.T) {
	info := geo.Info{Timezone: "Europe/London", Locale: "en-GB"}
	flags, env := applyGeoAlignment("future-provider", info, config.CloakBrowserRuntimeConfig{})
	if len(flags) != 0 || len(env) != 0 {
		t.Fatalf("unknown provider should produce no flags/env, got %v / %v", flags, env)
	}
}

func TestBuildChromeArgs_ChromeDoesNotApplyProxyGeoFlagsByDefault(t *testing.T) {
	old := geoProviderForConfigFunc
	t.Cleanup(func() { geoProviderForConfigFunc = old })
	geoProviderForConfigFunc = func(*config.RuntimeConfig) geo.Provider {
		t.Fatal("BuildChromeArgs should not perform geo lookup for the chrome provider")
		return geo.Noop{}
	}

	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderChrome,
		ChromeVersion:   "144.0.0.0",
		Proxy: config.BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo: &config.BrowserProxyGeoConfig{
				Timezone: "Europe/London",
				Locale:   "en-GB",
			},
		},
	}
	args := BuildChromeArgs(cfg, 9222)
	for _, blocked := range []string{
		"--lang=en-GB",
		"--webrtc-ip-handling-policy=disable_non_proxied_udp",
	} {
		if stealth.HasLaunchArg(args, blocked) {
			t.Errorf("BuildChromeArgs() included proxy-derived geo flag %q in %v", blocked, args)
		}
	}
}

func TestBuildChromeArgs_CloakAppliesGeoFlagsAndRespectsExplicit(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			Timezone: "America/New_York", // explicit wins
		},
		Proxy: config.BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo: &config.BrowserProxyGeoConfig{
				Timezone: "Europe/London",
				Locale:   "en-GB",
			},
		},
	}
	args := BuildChromeArgs(cfg, 9222)
	// Locale derived (no explicit cloak locale)
	if !stealth.HasLaunchArg(args, "--fingerprint-locale=en-GB") {
		t.Errorf("expected --fingerprint-locale=en-GB in %v", args)
	}
	// Explicit cloak timezone preserved
	if !stealth.HasLaunchArg(args, "--fingerprint-timezone=America/New_York") {
		t.Errorf("expected explicit cloak timezone in %v", args)
	}
	// Derived timezone NOT emitted
	if stealth.HasLaunchArg(args, "--fingerprint-timezone=Europe/London") {
		t.Errorf("derived timezone leaked despite explicit cloak override in %v", args)
	}
}

func TestMergeGeoEnv_OverridesByKey(t *testing.T) {
	base := []string{"PATH=/bin", "TZ=UTC", "FOO=bar"}
	add := []string{"TZ=Europe/London"}
	got := mergeGeoEnv(base, add)
	want := []string{"PATH=/bin", "FOO=bar", "TZ=Europe/London"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeGeoEnv = %v, want %v", got, want)
	}
}

func TestMergeGeoEnv_NoAdditionsReturnsBase(t *testing.T) {
	base := []string{"PATH=/bin"}
	got := mergeGeoEnv(base, nil)
	if !reflect.DeepEqual(got, base) {
		t.Errorf("mergeGeoEnv(_, nil) = %v, want %v", got, base)
	}
}
