// Package catalog is the single owner of the names autoSolver.solvers accepts.
// Every name comes from the thing that answers to it — the solver types' own
// Name() methods, and the built-in stage's exported name — so adding a solver
// cannot leave config validation stale behind a second hand-written list.
package catalog

import (
	"sort"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/external"
	"github.com/pinchtab/pinchtab/internal/autosolver/solvers"
)

// registrable is one instance of each solver type that can enter the registry,
// built with zero config because only Name() is read. KeyGated members are here
// too: their name is always valid, and whether they register is a separate
// question answered by the API key.
func registrable() []autosolver.Solver {
	return buildAll(autosolver.Config{})
}

// buildAll constructs every solver type, handing each key-gated one the key cfg
// carries for its own name. This is the only place a solver type is constructed with
// its key, so the handler that used to do it per solver no longer knows any of them.
func buildAll(cfg autosolver.Config) []autosolver.Solver {
	return []autosolver.Solver{
		&solvers.Cloudflare{},
		&solvers.JSChallenge{},
		external.NewCapsolver(external.CapsolverConfig{APIKey: cfg.APIKey(autosolver.CapsolverSolverName)}),
		external.NewTwoCaptcha(external.TwoCaptchaConfig{APIKey: cfg.APIKey(autosolver.TwoCaptchaSolverName)}),
	}
}

// Registrable is every solver that may enter the registry for this config: the
// unconditional ones, plus each key-gated one that is not Planned and whose key is set. Registering a gated
// solver without its key is what this drops — the registry would accept it and every
// request to it would fail at the provider.
func Registrable(cfg autosolver.Config) []autosolver.Solver {
	out := make([]autosolver.Solver, 0, len(registrable()))
	for _, solver := range buildAll(cfg) {
		if gated, ok := autosolver.KeyGatedSolverNamed(solver.Name()); ok && (gated.Planned || cfg.APIKey(solver.Name()) == "") {
			continue
		}
		out = append(out, solver)
	}
	return out
}

// Available answers the question the API asks: which solver names can actually run
// under this config. It is the single owner of that rule — the unconditional set plus
// every non-Planned key-gated name whose key is set — so a caller never re-derives it from the
// key fields, which is how the same rule came to be spelled three times.
//
// Derived from KeyGatedSolvers rather than from the constructed solvers, so a gated
// solver added to that set is answered here without a registry instance existing yet.
func Available(cfg autosolver.Config) []string {
	names := AlwaysRegistered()
	for _, gated := range autosolver.KeyGatedSolvers() {
		if !gated.Planned && cfg.APIKey(gated.Name) != "" {
			names = append(names, gated.Name)
		}
	}
	sort.Strings(names)
	return names
}

// IsAvailable reports whether one name can run under this config.
func IsAvailable(name string, cfg autosolver.Config) bool {
	for _, available := range Available(cfg) {
		if available == name {
			return true
		}
	}
	return false
}

// KeyGated reports the solvers that only register when their API key is set.
// Naming one without its key is a missing-key mistake, not an unknown solver.
// The set itself is owned by autosolver.KeyGatedSolvers, which also carries the
// config key each one needs, so the runtime warning and this validation face of
// the same fact cannot drift.
func KeyGated() []string {
	gated := autosolver.KeyGatedSolvers()
	names := make([]string, 0, len(gated))
	for _, solver := range gated {
		names = append(names, solver.Name)
	}
	return names
}

// AlwaysRegistered reports the solvers that need no configuration to run.
func AlwaysRegistered() []string {
	gated := map[string]struct{}{}
	for _, name := range KeyGated() {
		gated[name] = struct{}{}
	}
	names := []string{autosolver.SemanticSolverName}
	for _, s := range registrable() {
		if _, ok := gated[s.Name()]; ok {
			continue
		}
		names = append(names, s.Name())
	}
	sort.Strings(names)
	return names
}

// Names is every value autoSolver.solvers may contain, sorted so error messages
// and tests read the same way every run.
func Names() []string {
	names := make([]string, 0, len(registrable())+1)
	for _, s := range registrable() {
		names = append(names, s.Name())
	}
	names = append(names, autosolver.SemanticSolverName)
	sort.Strings(names)
	return names
}

func IsKnown(name string) bool {
	for _, known := range Names() {
		if known == name {
			return true
		}
	}
	return false
}
