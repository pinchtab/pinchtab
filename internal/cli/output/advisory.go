package output

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/cli/clistate"
)

// HintsEnv silences advisory hints when it is set to HintsOff. It is the only
// switch: a caller who has decided not to use sessions, or not to move a setting
// this build keeps mentioning, turns the whole class off in one place.
const (
	HintsEnv = "PINCHTAB_HINTS"
	HintsOff = "off"
)

// SilenceAdvisoryHint is printed once beside the first advisory of a run, so the
// switch is discoverable from the output it silences. It is printed BEFORE the
// advisory: both lines end with a command, and their readers lift the last one on
// the block, which must be the advisory's own remedy rather than the switch.
const SilenceAdvisoryHint = "silence advisory hints like this one: export " + HintsEnv + "=" + HintsOff

// advisoryMarkerDir holds one empty file per advisory already shown on this
// machine, named by the digest of its text.
const advisoryMarkerDir = "advisories"

var (
	advisoryMu    sync.Mutex
	advisoryShown = map[string]bool{}
	silencerShown bool
)

// Advisory prints a hint about a steady state the caller may have chosen — one
// whose wording is identical on every invocation and whose remedy is a decision,
// not a reaction. It prints once per install, not once per run: a CLI invocation
// is its own process, so an in-memory guard alone would repeat the same advice on
// every command forever. Nothing prints when the caller has silenced hints.
//
// A hint that reports what just happened is not advisory: it belongs on Hint,
// which prints every time it fires, because the second occurrence is news too.
func Advisory(text string) {
	// Both suppression checks come FIRST, because a suppressed advisory must change
	// no state. Taking the silencer decision before consulting the marker spent the
	// run's one silencer line on an advisory that then printed nothing, so the next
	// advisory — the one the caller is seeing for the first time — arrived with no
	// opt-out beside it. Two different advisories reach one process on the ordinary
	// path (the redundant --server notice and the no-session hint), so that is a
	// real caller's first sight of the switch, not a test-only shape.
	if AdvisoriesSilenced() || advisoryAlreadyShown(text) {
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
	if withSilencer {
		Hint(SilenceAdvisoryHint)
	}
	Hint(text)
	recordAdvisoryShown(text)
}

// AdvisoriesSilenced reports whether the caller has switched advisory hints off.
func AdvisoriesSilenced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(HintsEnv)), HintsOff)
}

// ResetAdvisories forgets what has already been said, in this process and on
// disk, so a test — or an operator who wants the advice back — starts from a
// genuinely fresh install.
func ResetAdvisories() {
	advisoryMu.Lock()
	advisoryShown = map[string]bool{}
	silencerShown = false
	advisoryMu.Unlock()
	_ = os.RemoveAll(filepath.Join(clistate.Dir(), advisoryMarkerDir))
}

// advisoryAlreadyShown reports whether this text has been printed on an earlier
// run. Marker I/O never decides anything but suppression: an unreadable state
// directory answers false, which prints, and printing twice is the behaviour this
// mechanism improves on rather than a failure of the command.
func advisoryAlreadyShown(text string) bool {
	_, err := os.Stat(advisoryMarkerPath(text))
	return err == nil
}

// recordAdvisoryShown is best-effort for the same reason: a caller who cannot
// write the marker keeps hearing the advice, which is the old behaviour, and
// never sees an error about a hint.
func recordAdvisoryShown(text string) {
	path := advisoryMarkerPath(text)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_ = file.Close()
	}
}

// advisoryMarkerPath keys the marker by the digest of the text, exactly as the
// in-process map is keyed by the text itself: a new advisory is never
// pre-suppressed by an unrelated one that already fired, and rewording an
// advisory makes it new, which is what a changed message deserves.
func advisoryMarkerPath(text string) string {
	digest := sha256.Sum256([]byte(text))
	return filepath.Join(clistate.Dir(), advisoryMarkerDir, hex.EncodeToString(digest[:])+".shown")
}
