package idpi

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/security"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// Every implementation behind Guard.DomainAllowed must answer the same question, and
// the answer that matters is what an EMPTY allowlist means. ShieldGuard used to say
// "allowed" for every URL under an empty list, while the free function and noopGuard
// said "not explicitly allowed" — two answers to one question, and the consumers feed
// it straight into navguard's private-IP override.
func TestEveryDomainAllowedImplementationAgreesOnAnEmptyAllowlist(t *testing.T) {
	const target = "https://10.0.0.5/x"
	enabled := config.IDPIConfig{Enabled: true, StrictMode: true}

	guards := map[string]Guard{
		"noop":   noopGuard{},
		"shield": NewShieldGuard(enabled, nil),
	}
	for name, guard := range guards {
		if guard.DomainAllowed(target) {
			t.Errorf("%s.DomainAllowed = true under an empty allowlist; this boolean is allowExplicitInternal, and an absent list is no restriction, never an explicit allowance", name)
		}
	}
	if DomainAllowed(target, enabled, nil) {
		t.Error("the free DomainAllowed granted an explicit allowance from an empty list")
	}

	// The override the fix must preserve: a positive match against a NON-empty list
	// still grants it, or the correction has removed the capability instead of the bug.
	if !NewShieldGuard(enabled, []string{"10.0.0.5"}).DomainAllowed(target) {
		t.Error("an explicitly listed internal host no longer grants the override; the fix has over-corrected")
	}
}

// security.HostAllowed answers a DIFFERENT question — "is this host permitted by the
// allowlist" — where an empty list imposes no restriction and the answer is true. It is
// asserted here, in the package whose fix could most easily have been swept into it,
// because a change that "made them consistent" would break the allowlist itself.
func TestTheAllowlistPredicateKeepsItsOwnEmptyListMeaning(t *testing.T) {
	if !security.HostAllowed("https://10.0.0.5/x", nil) {
		t.Error("security.HostAllowed refuses under an empty allowlist; an empty list imposes no restriction, and refusing here would block every navigation")
	}
	if security.HostAllowed("https://evil.com/x", []string{"example.com"}) {
		t.Error("security.HostAllowed permitted an unlisted host")
	}
}

// One owner, pinned by measurement rather than by reading: for every combination of
// config, allowlist and target, the guard's method and the free function give the same
// answer. A re-divergence reds here however it is written.
func TestTheGuardAndTheFreeFunctionAgreeOnEveryCombination(t *testing.T) {
	configs := map[string]config.IDPIConfig{
		"enabled strict": {Enabled: true, StrictMode: true},
		"enabled":        {Enabled: true},
		"disabled":       {},
	}
	allowlists := map[string][]string{
		"empty":              nil,
		"unrelated":          {"example.com"},
		"the target host":    {"10.0.0.5"},
		"wildcard":           {"*"},
		"wildcard subdomain": {"*.corp.example.com"},
	}
	targets := []string{
		"https://10.0.0.5/x",
		"https://example.com/x",
		"https://api.corp.example.com/x",
		"//10.0.0.5/x",
		"about:blank",
		"",
	}

	compared := 0
	for cfgName, cfg := range configs {
		for listName, allowed := range allowlists {
			guard := NewShieldGuard(cfg, allowed)
			for _, target := range targets {
				compared++
				if got, want := guard.DomainAllowed(target), DomainAllowed(target, cfg, allowed); got != want {
					t.Errorf("cfg=%s allowlist=%s target=%q: guard says %v, the owner says %v — two answers to one question is the defect, not a detail of it",
						cfgName, listName, target, got, want)
				}
			}
		}
	}
	if compared < len(configs)*len(allowlists)*len(targets) {
		t.Fatalf("compared %d combinations; the matrix is not being walked", compared)
	}
}

// The specific re-divergence to keep out: answering the explicit-allowance question
// from the shield's own threat scan. Every CheckDomain call inside this package must
// sit in a function named CheckDomain — the one that asks that question — so a
// DomainAllowed method reaching for the scan again reds by file and line.
//
// The census matches the bare name because that is the spelling the AST produces for a
// call through a field receiver (g.shield.CheckDomain): the callee's X is itself a
// selector, so the receiver does not survive into the callee name. Calls(t, ...) fails
// on zero matches, which is the floor — a rename cannot silently disarm this.
func TestTheShieldScanIsCalledOnlyWhereItAnswersItsOwnQuestion(t *testing.T) {
	pkg := srccensus.Load(t, ".", 5)

	for _, site := range pkg.Calls(t, "CheckDomain") {
		if site.Func != "CheckDomain" {
			t.Errorf("%s calls the domain scan from %s; answering any question but 'is there anything wrong with this host' from a threat score is how 'nothing looked wrong' came to mean 'the operator allowed this'",
				site, site.Func)
		}
	}
}
