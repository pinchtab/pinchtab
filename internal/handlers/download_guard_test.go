package handlers

import (
	"context"
	"strings"
	"testing"
)

// The download dialler asks the guard whether the operator named this host, and a
// bare "*" names none. It used to derive that grant from the RESTRICTION predicate
// — "nothing blocked it" read as "the operator permitted it" — which is why a
// wildcard, whose whole purpose is to block nothing, dialled every private address.
func TestABareWildcardGrantsNoPrivateIPOverrideOnTheDownloadDialler(t *testing.T) {
	const privateHost = "10.0.0.5"

	for _, tc := range []struct {
		name    string
		allowed []string
		want    bool
	}{
		{"a bare wildcard", []string{"*"}, false},
		{"a wildcard beside an unrelated host", []string{"*", "example.com"}, false},
		{"a wildcard beside the internal host", []string{"*", privateHost}, true},
		{"the internal host alone", []string{privateHost}, true},
		{"a wildcard subdomain", []string{"*.corp.example.com"}, false},
		{"no list at all", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard := newDownloadURLGuard(tc.allowed)
			allowInternal := guard.explicitlyAllowsHost(privateHost)
			if allowInternal != tc.want {
				t.Fatalf("explicitlyAllowsHost(%v) = %v, want %v", tc.allowed, allowInternal, tc.want)
			}

			// The predicate matters only through the dialler that consumes it: a
			// refused host must stop at the private-IP check, not at a connection.
			_, err := resolveDownloadDialIPs(context.Background(), privateHost, allowInternal)
			if tc.want {
				if err != nil {
					t.Errorf("the dialler refused a host the operator named: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "blocked remote IP") {
				t.Errorf("the dialler answered %v, want the private-IP refusal", err)
			}
		})
	}
}

// The withdrawal is scoped to the override: "*" still lifts the domain restriction
// for every host, which is what an operator writes it for. Without this, refusing
// the wildcard outright would satisfy every row above.
func TestABareWildcardStillLiftsTheDownloadDomainRestriction(t *testing.T) {
	const unlisted = "https://unlisted.example/file.bin"

	if err := newDownloadURLGuard([]string{"*"}).Validate(unlisted); err != nil {
		t.Errorf("a wildcard allowlist refused an unlisted public host: %v", err)
	}
	err := newDownloadURLGuard([]string{"listed.example"}).Validate(unlisted)
	if err == nil || !strings.Contains(err.Error(), "downloadAllowedDomains") {
		t.Errorf("a named allowlist admitted an unlisted host (%v); the row above would then prove nothing", err)
	}
}
