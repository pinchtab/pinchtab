package navguard

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/security"
	"github.com/pinchtab/pinchtab/internal/urls"
)

// authoritySpelling is one way of writing the same destination. WHATWG's
// special-authority-ignore-slashes state skips ANY run of "/" and "\" after a
// special scheme's colon, so the set of spellings is a property of the state
// machine rather than a list somebody remembered — which is why they are generated.
type authoritySpelling struct {
	raw  string
	note string
}

// authoritySpellings generates every run of "/" and "\" up to length three, after a
// special scheme's colon and in the scheme-less (protocol-relative) position, plus
// the backslash-in-path forms a browser reads as slashes.
func authoritySpellings(hostAndPath string) []authoritySpelling {
	var runs []string
	for length := 0; length <= 3; length++ {
		for mask := 0; mask < 1<<length; mask++ {
			var run strings.Builder
			for bit := 0; bit < length; bit++ {
				if mask&(1<<bit) == 0 {
					run.WriteByte('/')
				} else {
					run.WriteByte('\\')
				}
			}
			runs = append(runs, run.String())
		}
	}

	backslashPath := strings.ReplaceAll(hostAndPath, "/", `\`)
	var out []authoritySpelling
	for _, run := range runs {
		out = append(out,
			authoritySpelling{raw: "https:" + run + hostAndPath, note: fmt.Sprintf("scheme-ful, run %q", run)},
			authoritySpelling{raw: "https:" + run + backslashPath, note: fmt.Sprintf("scheme-ful with backslash path, run %q", run)},
		)
		if run != "" {
			out = append(out,
				authoritySpelling{raw: run + hostAndPath, note: fmt.Sprintf("scheme-less, run %q", run)},
				authoritySpelling{raw: run + backslashPath, note: fmt.Sprintf("scheme-less with backslash path, run %q", run)},
			)
		}
	}
	return out
}

// The whole defect: seven spellings of one private address reached it past the SSRF
// guard the correctly-spelled one is refused by, because Go read them as hostless and
// a hostless target skipped resolution entirely. Driven through the production
// sequence — urls.EnsureScheme then ValidateTarget — not against the extractor alone.
func TestEverySpellingOfAPrivateHostIsRefused(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	canonical := "https://10.0.0.5/x"
	if _, err := ValidateTarget(context.Background(), urls.EnsureScheme(canonical), false, nil); err == nil {
		t.Fatal("the correctly-spelled private address was allowed; this table compares against it")
	}

	for _, spelling := range authoritySpellings("10.0.0.5/x") {
		t.Run(spelling.raw, func(t *testing.T) {
			normalized := urls.EnsureScheme(spelling.raw)
			if host := security.ExtractHost(spelling.raw); host != "10.0.0.5" {
				t.Errorf("host = %q, want 10.0.0.5 (%s): normalized to %q — the guard reads a different host than the browser", host, spelling.note, normalized)
			}
			if _, err := ValidateTarget(context.Background(), normalized, false, nil); err == nil {
				t.Errorf("ALLOWED (%s): normalized to %q — this names a private address the canonical spelling is refused for", spelling.note, normalized)
			}
		})
	}
}

// The regression named in the card. Before urls.EnsureScheme existed, a
// protocol-relative target was REFUSED; the normalization prepended a scheme in front
// of the leading "//" and produced "https:////10.0.0.5/x", which net/url reads as
// hostless — so the most ordinary spelling of the set became the bypass. It is
// refused again, and stripping the leading run before prepending is what does it.
func TestTheProtocolRelativeRegressionIsRefusedAgain(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	const raw = "//10.0.0.5/x"
	if got, want := urls.EnsureScheme(raw), "https://10.0.0.5/x"; got != want {
		t.Fatalf("EnsureScheme(%q) = %q, want %q; prepending a scheme destroyed the authority the input already had", raw, got, want)
	}
	if _, err := ValidateTarget(context.Background(), urls.EnsureScheme(raw), false, nil); err == nil {
		t.Fatal("a protocol-relative private address was allowed")
	}
}

// The other half, and the reason the fix normalises rather than blanket-rejects
// unusual punctuation: a public host written in any of the same spellings still
// navigates. A guard that refused all of them would be fail-closed and useless.
func TestEverySpellingOfAPublicHostStillNavigates(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	for _, spelling := range authoritySpellings("example.com/x") {
		t.Run(spelling.raw, func(t *testing.T) {
			if _, err := ValidateTarget(context.Background(), urls.EnsureScheme(spelling.raw), false, nil); err != nil {
				t.Errorf("REFUSED (%s): %v — a legitimate target stopped navigating", spelling.note, err)
			}
		})
	}
}

// "Cannot check" and "checked and safe" must not share an outcome. This is what
// keeps the class closed rather than resting on how exactly EnsureScheme mirrors the
// WHATWG state machine: a target left with no readable host is refused whether or not
// the normaliser anticipated its spelling.
func TestATargetWithNoReadableHostIsRefusedRatherThanAllowedUnresolved(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		t.Error("a target with no readable host was resolved, which cannot happen")
		return nil, nil
	})

	for _, raw := range []string{"https://", "https://?q=1", "https://#frag"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ValidateTarget(context.Background(), raw, false, nil); err == nil {
				t.Error("a target the guard cannot read a host from was allowed unresolved")
			}
		})
	}
}

// EnsureScheme must never destroy an authority the input already had: whenever a host
// is readable from the input, one is readable from its output, and it is the same
// host. That is the property the protocol-relative regression violated.
func TestEnsureSchemeNeverDestroysAnAuthority(t *testing.T) {
	for _, hostAndPath := range []string{"10.0.0.5/x", "example.com/x", "example.com:8443/x"} {
		for _, spelling := range authoritySpellings(hostAndPath) {
			t.Run(spelling.raw, func(t *testing.T) {
				normalized := urls.EnsureScheme(spelling.raw)
				if security.ExtractHost(normalized) == "" {
					t.Errorf("EnsureScheme(%q) = %q, from which no host can be read (%s)", spelling.raw, normalized, spelling.note)
				}
			})
		}
	}
}
