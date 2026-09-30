package catalog

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

func TestKeyGatedMirrorsTheOwningSet(t *testing.T) {
	want := make([]string, 0)
	for _, gated := range autosolver.KeyGatedSolvers() {
		want = append(want, gated.Name)
		if gated.ConfigKey == "" {
			t.Errorf("key-gated solver %q has no config key, so the runtime warning cannot say what to set", gated.Name)
		}
	}
	if len(want) == 0 {
		t.Fatal("autosolver.KeyGatedSolvers is empty, so nothing pins this behaviour")
	}
	if got := KeyGated(); !reflect.DeepEqual(got, want) {
		t.Fatalf("KeyGated() = %v, want the owning set %v", got, want)
	}
}

func TestKeyGatedNamesAreValidConfigValues(t *testing.T) {
	for _, name := range KeyGated() {
		if !IsKnown(name) {
			t.Errorf("key-gated solver %q is rejected by config validation (known: %v)", name, Names())
		}
	}
}

func TestKeyGatedSolversAreExcludedFromAlwaysRegistered(t *testing.T) {
	gated := map[string]struct{}{}
	for _, name := range KeyGated() {
		gated[name] = struct{}{}
	}
	for _, name := range AlwaysRegistered() {
		if _, ok := gated[name]; ok {
			t.Errorf("%q is key-gated yet reported as always registered", name)
		}
	}
}

func TestRegistrableSolversAreAllKnownNames(t *testing.T) {
	names := Names()
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(names, sorted) {
		t.Fatalf("Names() = %v, want sorted output so messages read the same every run", names)
	}
	for _, s := range registrable() {
		if !IsKnown(s.Name()) {
			t.Errorf("registrable solver %q is not a known config value (known: %v)", s.Name(), names)
		}
	}
}

// The rule this package now owns: availability follows the key-gated SET, so adding a
// gated solver is answered here with nobody editing an availability check. Proven by
// adding one to the real set the real callers read — a fixture could not make that
// claim, since the point is that production reads this exact list.
func TestAddingAKeyGatedSolverChangesAvailabilityWithNoOtherEdit(t *testing.T) {
	const fake = "fakesolver"

	restore := autosolver.SetKeyGatedSolversForTest(append(autosolver.KeyGatedSolvers(),
		autosolver.KeyGatedSolver{Name: fake, ConfigKey: "autoSolver.external.fakeKey"}))
	defer restore()

	keyless := autosolver.Config{}
	if IsAvailable(fake, keyless) {
		t.Errorf("%q is available with no key set; a key-gated solver without its key cannot run", fake)
	}
	if slices.Contains(Available(keyless), fake) {
		t.Errorf("Available(%v) lists %q with no key set", keyless.APIKeys, fake)
	}

	keyed := autosolver.Config{APIKeys: map[string]string{fake: "k"}}
	if !IsAvailable(fake, keyed) {
		t.Errorf("%q is unavailable with its key set", fake)
	}
	if !slices.Contains(Available(keyed), fake) {
		t.Errorf("Available with a key set = %v, want it to list %q", Available(keyed), fake)
	}

	// A blank-but-present key is the same as absent, or "configured it to empty" would
	// register a solver whose every request fails at the provider.
	blank := autosolver.Config{APIKeys: map[string]string{fake: "   "}}
	if IsAvailable(fake, blank) {
		t.Errorf("%q is available with a blank key", fake)
	}
}

// The unconditional solvers are unaffected by any of this: they answer available with
// no config at all, which is what stops a keyless config from disabling the whole set.
func TestUnconditionalSolversAreAvailableWithNoConfig(t *testing.T) {
	available := Available(autosolver.Config{})
	for _, name := range AlwaysRegistered() {
		if !slices.Contains(available, name) {
			t.Errorf("%q is always registered but Available(empty config) = %v", name, available)
		}
	}
	for _, gated := range autosolver.KeyGatedSolvers() {
		if slices.Contains(available, gated.Name) {
			t.Errorf("key-gated %q is available with no key configured", gated.Name)
		}
	}
}

// Registration follows the same rule as availability, from the same config: a gated
// solver enters the registry exactly when its key is set. These were two separate
// re-implementations of one rule before, in a package that could not see this one.
func TestRegistrableFollowsTheSameKeyRuleAsAvailable(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cfg            autosolver.Config
		wantRegistered map[string]bool
	}{
		{"planned solvers stay out even with keys", autosolver.Config{APIKeys: map[string]string{
			autosolver.CapsolverSolverName:  "k",
			autosolver.TwoCaptchaSolverName: "k2",
		}}, map[string]bool{}},
		{"no keys", autosolver.Config{}, map[string]bool{
			autosolver.CapsolverSolverName:  false,
			autosolver.TwoCaptchaSolverName: false,
		}},
		{"one key", autosolver.Config{APIKeys: map[string]string{autosolver.CapsolverSolverName: "k"}}, map[string]bool{
			autosolver.CapsolverSolverName:  false,
			autosolver.TwoCaptchaSolverName: false,
		}},
		{"blank key", autosolver.Config{APIKeys: map[string]string{autosolver.CapsolverSolverName: " "}}, map[string]bool{
			autosolver.CapsolverSolverName:  false,
			autosolver.TwoCaptchaSolverName: false,
		}},
		{"both keys", autosolver.Config{APIKeys: map[string]string{
			autosolver.CapsolverSolverName:  "k",
			autosolver.TwoCaptchaSolverName: "k2",
		}}, map[string]bool{
			autosolver.CapsolverSolverName:  false,
			autosolver.TwoCaptchaSolverName: false,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registered := []string{}
			for _, solver := range Registrable(tc.cfg) {
				registered = append(registered, solver.Name())
			}
			for _, gated := range autosolver.KeyGatedSolvers() {
				wantRegistered := tc.wantRegistered[gated.Name]
				if got := slices.Contains(registered, gated.Name); got != wantRegistered {
					t.Errorf("%q registered = %v, want %v (available = %v); the literal expectation is what pins the key rule, a whitespace-only key counts as unset", gated.Name, got, wantRegistered, IsAvailable(gated.Name, tc.cfg))
				}
				if slices.Contains(registered, gated.Name) != IsAvailable(gated.Name, tc.cfg) {
					t.Errorf("%q: registration and availability disagree under %v; this consistency check catches the two implementations diverging, NOT a key-rule change — a trim removed from the shared accessor moves both sides together, only the literal wantRegistered reds on that", gated.Name, tc.cfg.APIKeys)
				}
			}
		})
	}
}
