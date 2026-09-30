package autosolver

const (
	CapsolverSolverName  = "capsolver"
	TwoCaptchaSolverName = "twocaptcha"
)

// KeyGatedSolver is a solver that only registers once its API key is set, paired
// with the config key that sets it. Naming one without its key is legal config,
// so the runtime says which key is missing rather than the config refusing to
// load. This is the single owner of both facts: catalog.KeyGated() and the
// registration sites read it instead of restating names.
type KeyGatedSolver struct {
	Name      string
	ConfigKey string
	// Planned marks a provider whose API client does not exist yet: its name and
	// key stay valid config, but it never registers, key or not.
	Planned bool
}

var keyGatedSolvers = []KeyGatedSolver{
	{Name: CapsolverSolverName, ConfigKey: "autoSolver.external.capsolverKey", Planned: true},
	{Name: TwoCaptchaSolverName, ConfigKey: "autoSolver.external.twoCaptchaKey", Planned: true},
}

// KeyGatedSolvers returns a copy, so a caller iterating it cannot edit the set.
func KeyGatedSolvers() []KeyGatedSolver {
	return append([]KeyGatedSolver(nil), keyGatedSolvers...)
}

// SetKeyGatedSolversForTest replaces the set and returns a restore function. It is
// exported production code on purpose: the rule under test is "adding a gated solver
// changes availability without anyone editing the availability code", and that claim
// cannot be made from a fixture — only by adding one to the real set the real callers
// read. Every caller reads it through KeyGatedSolvers, so the substitution reaches all
// of them.
func SetKeyGatedSolversForTest(gated []KeyGatedSolver) (restore func()) {
	previous := keyGatedSolvers
	keyGatedSolvers = append([]KeyGatedSolver(nil), gated...)
	return func() { keyGatedSolvers = previous }
}

// KeyGatedSolverNamed reports the key-gated solver answering to this name, so a
// caller rejecting it can name the key that would enable it.
func KeyGatedSolverNamed(name string) (KeyGatedSolver, bool) {
	for _, gated := range KeyGatedSolvers() {
		if gated.Name == name {
			return gated, true
		}
	}
	return KeyGatedSolver{}, false
}
