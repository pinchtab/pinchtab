package stealth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/config"
)

func majorOf(version string) string {
	return strings.SplitN(version, ".", 2)[0]
}

func secCHUAMajor(md *emulation.UserAgentMetadata) string {
	for _, b := range md.Brands {
		if b.Brand == BrandChrome {
			return b.Version
		}
	}
	return ""
}

func versionLineProbe(byBinary map[string]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, binary string) (string, error) {
		v, ok := byBinary[binary]
		if !ok {
			return "", errors.New("unknown binary")
		}
		return "Google Chrome " + v, nil
	}
}

func personaMajor(p BrowserPersona) string {
	for _, b := range p.UserAgentData.Brands {
		if b.Brand == BrandChrome {
			return b.Version
		}
	}
	return ""
}

// AC2: an explicit browser.version wins and the binary is not probed at all — a
// deliberate pin must not depend on, or pay for, spawning the binary.
func TestResolveBrowserVersionPrefersExplicitConfigWithoutProbing(t *testing.T) {
	cache := browserprobe.NewVersionCache(func(context.Context, string) (string, error) {
		t.Fatal("the binary was probed even though browser.version was set explicitly")
		return "", nil
	})
	got := resolveBrowserVersion(cache, &config.RuntimeConfig{
		BrowserVersion: "199.0.1234.5",
		BrowserBinary:  "/fake/chrome",
	})
	if got != "199.0.1234.5" {
		t.Errorf("resolved %q, want the explicit config version", got)
	}
}

// AC1/AC8: with browser.version unset the resolver returns the probed binary
// version, and that version reaches both the persona brands and the Sec-CH-UA
// metadata. Bumping the fake binary's version changes both and nothing else.
func TestProbedVersionReachesPersonaAndSecCHUA(t *testing.T) {
	for _, full := range []string{"150.0.7871.187", "151.0.8000.10"} {
		cache := browserprobe.NewVersionCache(versionLineProbe(map[string]string{"/fake/chrome": full}))
		resolved := resolveBrowserVersion(cache, &config.RuntimeConfig{BrowserBinary: "/fake/chrome"})
		if resolved != full {
			t.Fatalf("resolved %q, want the probed %q", resolved, full)
		}

		persona := BuildPersona("", resolved)
		wantMajor := majorOf(full)
		if got := personaMajor(persona); got != wantMajor {
			t.Errorf("persona brand major = %q, want %q", got, wantMajor)
		}
		if got := fullVersionMajorList(persona); got != full {
			t.Errorf("persona fullVersionList = %q, want the full probed build %q", got, full)
		}

		override := BuildUserAgentOverride("", resolved)
		if override == nil || override.UserAgentMetadata == nil {
			t.Fatal("no UA-CH metadata built; Sec-CH-UA would carry no version")
		}
		if got := secCHUAMajor(override.UserAgentMetadata); got != wantMajor {
			t.Errorf("Sec-CH-UA brand major = %q, want %q", got, wantMajor)
		}
	}
}

// AC3: a binary is probed at most once per process no matter how many persona
// builds ask for its version — the cache stands between the request path and the
// process spawn.
func TestBinaryIsProbedAtMostOncePerProcess(t *testing.T) {
	cache := browserprobe.NewVersionCache(versionLineProbe(map[string]string{"/fake/chrome": "150.0.7871.187"}))
	cfg := &config.RuntimeConfig{BrowserBinary: "/fake/chrome"}
	for i := range 5 {
		if v := resolveBrowserVersion(cache, cfg); v != "150.0.7871.187" {
			t.Fatalf("resolve %d returned %q", i, v)
		}
	}
	if cache.Probes() != 1 {
		t.Errorf("probes = %d, want 1 — RunVersion spawns a process, so repeats must be served from the cache", cache.Probes())
	}
}

// AC6: the cache is keyed by resolved binary path, so two binaries reporting
// different versions produce two personas — cloak and ghost-chrome are separate
// binaries and are covered by construction, with no provider-specific code.
func TestTwoBinariesYieldTwoPersonas(t *testing.T) {
	cache := browserprobe.NewVersionCache(versionLineProbe(map[string]string{
		"/bin/chrome": "150.0.7871.187",
		"/bin/cloak":  "151.0.8000.10",
	}))
	va := resolveBrowserVersion(cache, &config.RuntimeConfig{BrowserBinary: "/bin/chrome"})
	vb := resolveBrowserVersion(cache, &config.RuntimeConfig{BrowserBinary: "/bin/cloak"})

	pa := personaMajor(BuildPersona("", va))
	pb := personaMajor(BuildPersona("", vb))
	if pa == pb {
		t.Fatalf("both binaries produced persona major %q; a path-keyed probe must distinguish them", pa)
	}
	if pa != "150" || pb != "151" {
		t.Errorf("persona majors = %q, %q; want 150, 151", pa, pb)
	}
}

// AC4: a probe that fails resolves to the empty string, which the persona builders
// reduce to the fallback, so a missing or unrunnable binary never blocks the build.
func TestFailedProbeFallsBackWithoutError(t *testing.T) {
	cache := browserprobe.NewVersionCache(func(context.Context, string) (string, error) {
		return "", errors.New("binary missing")
	})
	resolved := resolveBrowserVersion(cache, &config.RuntimeConfig{BrowserBinary: "/does/not/exist"})
	if resolved != "" {
		t.Fatalf("resolved %q, want empty so the persona falls back", resolved)
	}
	persona := BuildPersona("", resolved)
	wantMajor := majorOf(browserprobe.FallbackChromeVersion)
	if got := personaMajor(persona); got != wantMajor {
		t.Errorf("fallback persona major = %q, want %q", got, wantMajor)
	}
}

func fullVersionMajorList(p BrowserPersona) string {
	for _, b := range p.UserAgentData.FullVersionList {
		if b.Brand == BrandChrome {
			return b.Version
		}
	}
	return ""
}

func fakeChromeBinary(t *testing.T, version string) string {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("fake binary uses a shell script")
	}
	path := filepath.Join(t.TempDir(), "fake-chrome")
	script := "#!/bin/sh\necho 'Google Chrome " + version + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

// Wiring proof (native, since the Docker e2e that checks the wire cannot run
// here): with browser.version unset, NewBundle's persona JSON — the script that
// sets navigator.userAgent and userAgentData on every page — carries the version
// the launched binary actually reports, not the frozen fallback. This is the
// bundle.go half of AC1 that no BuildPersona-level test reaches.
func TestNewBundlePersonaAdvertisesTheProbedBinaryVersion(t *testing.T) {
	binary := fakeChromeBinary(t, "150.0.7871.187")
	bundle := NewBundle(&config.RuntimeConfig{BrowserBinary: binary}, 0)

	if !strings.Contains(bundle.Script, "Chrome/150.0.0.0") {
		t.Errorf("persona script does not advertise the probed major 150; it still carries a frozen version:\n%s", firstPersonaLine(bundle.Script))
	}
	if strings.Contains(bundle.Script, "Chrome/144.0.0.0") {
		t.Errorf("persona script still advertises the fallback 144 though the binary reports 150")
	}
}

func firstPersonaLine(script string) string {
	for line := range strings.SplitSeq(script, "\n") {
		if strings.Contains(line, "userAgent") {
			return line
		}
	}
	return script
}
