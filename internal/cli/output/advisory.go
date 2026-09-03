package output

import (
	"os"
	"strings"
	"sync"
)

// HintsEnv silences advisory hints when it is set to HintsOff. It is the only
// switch: a caller who has decided not to use sessions, or not to move a setting
// this build keeps mentioning, turns the whole class off in one place.
const (
	HintsEnv = "PINCHTAB_HINTS"
	HintsOff = "off"
)

// SilenceAdvisoryHint is printed once beside the first advisory of a run, so the
// switch is discoverable from the output it silences. It ends with the command,
// because its readers lift the last words of a hint verbatim.
const SilenceAdvisoryHint = "silence advisory hints like this one: export " + HintsEnv + "=" + HintsOff

var (
	advisoryMu    sync.Mutex
	advisoryShown = map[string]bool{}
	silencerShown bool
)

// Advisory prints a hint about a steady state the caller may have chosen — one
// whose wording is identical on every invocation and whose remedy is a decision,
// not a reaction. It prints at most once per process and not at all when the
// caller has silenced hints.
//
// A hint that reports what just happened is not advisory: it belongs on Hint,
// which prints every time it fires, because the second occurrence is news too.
func Advisory(text string) {
	if AdvisoriesSilenced() {
		return
	}

	advisoryMu.Lock()
	first := !advisoryShown[text]
	advisoryShown[text] = true
	withSilencer := first && !silencerShown
	silencerShown = silencerShown || first
	advisoryMu.Unlock()

	if !first {
		return
	}
	Hint(text)
	if withSilencer {
		Hint(SilenceAdvisoryHint)
	}
}

// AdvisoriesSilenced reports whether the caller has switched advisory hints off.
func AdvisoriesSilenced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(HintsEnv)), HintsOff)
}

// ResetAdvisories forgets what this process has already said, for a test that
// needs a fresh run and for any host that reuses one process across sessions.
func ResetAdvisories() {
	advisoryMu.Lock()
	advisoryShown = map[string]bool{}
	silencerShown = false
	advisoryMu.Unlock()
}
