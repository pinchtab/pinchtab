package session

import (
	"slices"
	"strings"
	"testing"
)

// A mistyped grant that is dropped silently produces a session the caller believes
// is scoped and which is not — the failure mode this whole card exists to end, one
// layer up from the field that could not be written at all.
func TestValidateGrantsRefusesAnUnknownName(t *testing.T) {
	if _, err := ValidateGrants([]string{"browse", "brows"}); err == nil {
		t.Fatal("a mistyped grant was accepted; the session would look scoped and be unscoped")
	} else {
		for _, want := range []string{"brows", "browse", "activity"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %q does not carry %q — it must name the offender and the whole vocabulary", err, want)
			}
		}
	}
}

// Case and surrounding space are read the way every other vocabulary here is read,
// so a capitalised grant scopes the session rather than matching no route at all.
// The empty entry this list used to carry moved to the refusal test below: it was
// dropped silently here, which is the accept-and-discard shape this whole card
// exists to remove.
func TestValidateGrantsNormalizesAndDeduplicates(t *testing.T) {
	got, err := ValidateGrants([]string{" Browse ", "BROWSE", "network"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{GrantBrowse, GrantNetwork}) {
		t.Errorf("ValidateGrants = %v, want the normalized pair once each", got)
	}
}

// An entry that names nothing is the quiet half of the same defect: skipping it
// answered 201 with a session that reaches every non-admin route, for a caller who
// asked to be scoped. Saying nothing about grants is still allowed — that is the
// absent field and the empty list, both of which mean "do not narrow".
func TestValidateGrantsRefusesAnEntryThatNamesNothing(t *testing.T) {
	for _, entry := range []string{"", "   ", "\t"} {
		got, err := ValidateGrants([]string{entry})
		if err == nil {
			t.Errorf("ValidateGrants([%q]) = %v, nil; an entry naming no grant produced an UNSCOPED session with no refusal", entry, got)
			continue
		}
		if !strings.Contains(err.Error(), "browse") {
			t.Errorf("the refusal %q does not name the vocabulary", err)
		}
	}
	for _, entry := range []string{"", " "} {
		if _, err := ValidateGrants([]string{entry, GrantBrowse}); err == nil {
			t.Errorf("ValidateGrants([%q, browse]) was accepted; one good name must not excuse an empty one", entry)
		}
	}

	for name, grants := range map[string][]string{"absent": nil, "empty list": {}} {
		got, err := ValidateGrants(grants)
		if err != nil || len(got) != 0 {
			t.Errorf("%s grants = %v, %v; want the unscoped session, which is what saying nothing means", name, got, err)
		}
	}
}

// "*" is the explicit spelling of "no narrowing" and must stay settable: the
// middleware already reads it, so refusing it here would make the two disagree.
func TestValidateGrantsAcceptsTheAllWildcard(t *testing.T) {
	got, err := ValidateGrants([]string{GrantAll})
	if err != nil || !slices.Equal(got, []string{GrantAll}) {
		t.Fatalf("ValidateGrants(*) = %v, %v", got, err)
	}
}

// The copy is the point: a caller that mutated the returned slice would change
// what one process enforces without changing what it accepts.
func TestGrantNamesHandsOutACopy(t *testing.T) {
	names := GrantNames()
	if len(names) == 0 {
		t.Fatal("no grant names")
	}
	names[0] = "mutated"
	if GrantNames()[0] == "mutated" {
		t.Error("GrantNames returns the backing array; one caller could rewrite the vocabulary")
	}
}
