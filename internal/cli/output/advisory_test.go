package output

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = writer

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, reader)
		close(done)
	}()

	fn()

	_ = writer.Close()
	os.Stderr = old
	<-done
	_ = reader.Close()
	return buf.String()
}

// isolatedInstall points the CLI state directory at a temp dir, so a test never
// reads or writes the markers of the machine it runs on.
func isolatedInstall(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(HintsEnv, "")
	ResetAdvisories()
}

// nextInvocation is the whole point of this card in one helper: a `pinchtab`
// command is its own process, so the next run starts with an empty in-process map
// and the SAME state directory. Only the marker can suppress anything here — the
// in-memory guard is gone the moment the process exits.
func nextInvocation() {
	advisoryMu.Lock()
	advisoryShown = map[string]bool{}
	silencerShown = false
	advisoryMu.Unlock()
}

const testAdvisory = "this tab is shared — no agent session is set: export PINCHTAB_SESSION=$(pinchtab session create --agent-id <id>)"

// The filed defect: every invocation of the most-run command reprinted the same
// advice about a steady state the caller had already chosen, because the guard
// lived only in the process that printed it.
func TestAnAdvisoryPrintedOnceIsNotRepeatedByALaterRun(t *testing.T) {
	isolatedInstall(t)

	first := captureStderr(t, func() { Advisory(testAdvisory) })
	if !strings.Contains(first, testAdvisory) {
		t.Fatalf("the first run printed no advisory: %q", first)
	}

	nextInvocation()
	second := captureStderr(t, func() { Advisory(testAdvisory) })
	if second != "" {
		t.Errorf("a later run reprinted the advisory: %q", second)
	}

	nextInvocation()
	third := captureStderr(t, func() { Advisory(testAdvisory) })
	if third != "" {
		t.Errorf("a third run reprinted the advisory: %q", third)
	}
}

// The switch has to stay discoverable from the output it silences, and it has to
// come first: both lines end with a command, and an agent reading the block lifts
// the last one — which must be the advisory's own remedy, not the way to mute it.
func TestTheFirstRunPublishesTheSwitchBeforeTheAdvisory(t *testing.T) {
	isolatedInstall(t)

	out := captureStderr(t, func() { Advisory(testAdvisory) })

	switchAt := strings.Index(out, SilenceAdvisoryHint)
	adviceAt := strings.Index(out, testAdvisory)
	if switchAt < 0 || adviceAt < 0 {
		t.Fatalf("the first run must carry both lines, got %q", out)
	}
	if switchAt > adviceAt {
		t.Errorf("the silencer is printed after the advisory, so the last command in the block mutes the hint instead of following it:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(SilenceAdvisoryHint), HintsEnv+"="+HintsOff) {
		t.Errorf("the switch must end its own line so it can be lifted verbatim: %q", SilenceAdvisoryHint)
	}
}

// A marker is a suppression, never a dependency: if it cannot be written the
// caller hears the advice again, which is the behaviour this replaces, and the
// command itself is untouched.
func TestAnUnwritableStateDirectoryFallsBackToPrinting(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through the permission this test relies on")
	}
	isolatedInstall(t)

	readOnly := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", readOnly)

	first := captureStderr(t, func() { Advisory(testAdvisory) })
	if !strings.Contains(first, testAdvisory) {
		t.Fatalf("nothing printed under an unwritable state directory: %q", first)
	}

	nextInvocation()
	second := captureStderr(t, func() { Advisory(testAdvisory) })
	if !strings.Contains(second, testAdvisory) {
		t.Errorf("the advisory was suppressed although no marker could be written: %q", second)
	}
}

// Suppression is keyed per text, exactly as the in-process map is, so shipping a
// second advisory does not arrive pre-silenced by the first.
func TestASecondAdvisoryIsNotSuppressedByTheFirst(t *testing.T) {
	isolatedInstall(t)

	const other = "a different steady state: export PINCHTAB_SOMETHING=1"
	captureStderr(t, func() { Advisory(testAdvisory) })

	nextInvocation()
	out := captureStderr(t, func() { Advisory(other) })
	if !strings.Contains(out, other) {
		t.Errorf("an advisory that has never been shown was suppressed: %q", out)
	}
}

// The run where the two suppression states MEET, which is the ordinary path once a
// caller has run any command that emits its own advisory: an already-shown one is
// suppressed and a brand-new one prints in the same process. The new advisory is
// the caller's first sight of this class, so it must carry the switch — an
// advisory that prints nothing must not consume the run's silencer on its way out.
func TestASuppressedAdvisoryDoesNotConsumeTheSilencerOfANewOne(t *testing.T) {
	isolatedInstall(t)

	const other = "a different steady state: export PINCHTAB_SOMETHING=1"
	if first := captureStderr(t, func() { Advisory(testAdvisory) }); !strings.Contains(first, SilenceAdvisoryHint) {
		t.Fatalf("the first run printed no silencer (%q), so this test never reached the state it is about", first)
	}

	nextInvocation()
	out := captureStderr(t, func() {
		Advisory(testAdvisory)
		Advisory(other)
	})

	if strings.Contains(out, testAdvisory) {
		t.Errorf("the already-shown advisory printed again: %q", out)
	}
	if !strings.Contains(out, other) {
		t.Errorf("the new advisory was suppressed by an unrelated one: %q", out)
	}
	if !strings.Contains(out, SilenceAdvisoryHint) {
		t.Errorf("the new advisory arrived with no way to learn about the switch: %q", out)
	}
}

// ResetAdvisories has to reach the marker too, or a test asking for a fresh run
// gets a machine that has already heard everything.
func TestResetAdvisoriesClearsThePersistedMarker(t *testing.T) {
	isolatedInstall(t)

	captureStderr(t, func() { Advisory(testAdvisory) })
	if _, err := os.Stat(advisoryMarkerPath(testAdvisory)); err != nil {
		t.Fatalf("the first run wrote no marker (%v), so the reset below would prove nothing", err)
	}

	ResetAdvisories()
	if _, err := os.Stat(advisoryMarkerPath(testAdvisory)); err == nil {
		t.Error("the marker survived ResetAdvisories")
	}
	out := captureStderr(t, func() { Advisory(testAdvisory) })
	if !strings.Contains(out, testAdvisory) {
		t.Errorf("after a reset the advisory stayed silent: %q", out)
	}
}

// The switch still wins over everything: a caller who has already opted out hears
// nothing on the very first run, and no marker is written to suppress a hint they
// may want back once they unset it.
func TestSilencedAdvisoriesPrintNothingAndRecordNothing(t *testing.T) {
	isolatedInstall(t)
	t.Setenv(HintsEnv, HintsOff)

	if out := captureStderr(t, func() { Advisory(testAdvisory) }); out != "" {
		t.Fatalf("hints are silenced but something printed: %q", out)
	}
	if _, err := os.Stat(advisoryMarkerPath(testAdvisory)); err == nil {
		t.Error("a silenced advisory recorded itself as shown, so unsetting the switch would still say nothing")
	}
}
