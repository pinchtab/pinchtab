package cli

import (
	"strings"
	"testing"
)

// The hint is state-blind by design — the CLI cannot know whether the server has agent
// sessions enabled without a new health field or a round trip per nav — so the wording
// has to be TRUE in both states rather than correct in one. These assertions are on the
// properties that make that so, not on the sentence, because the sentence will be
// reworded again and a full-string compare would only pin today's phrasing.
func TestNoSessionHintHoldsWhetherOrNotSessionsAreEnabled(t *testing.T) {
	hint := NoSessionHint

	// The enabled server is the state the previous wording got wrong: it asserted the
	// prerequisite as an unmet requirement, so a correctly configured user was told to
	// change config that was already right and restart a server that needed no restart.
	for _, imperative := range []string{
		"must be enabled",
		"Agent sessions must be",
		"then restart)",
	} {
		if strings.Contains(hint, imperative) {
			t.Errorf("hint asserts %q as an unmet requirement, which is false on a server that already has sessions enabled: %q", imperative, hint)
		}
	}

	// The disabled server keeps the no-dead-end property: the prerequisite is still
	// named, so a reader following the hint top to bottom does not land in the command's
	// own "agent sessions are not enabled on this server" with nowhere to go.
	for _, needed := range []string{"sessions.agent.enabled = true", "restart", SessionCreateCommand} {
		if !strings.Contains(hint, needed) {
			t.Errorf("hint no longer carries %q; a reader on default config dead-ends without it: %q", needed, hint)
		}
	}

	// Order carries the meaning, and it is the CONDITION clauses that carry it: the
	// half that applies to a working server is read first, and the config change is
	// reachable only through the fallback. This used to measure the position of the
	// command literal instead, which said "clause order" but forbade any layout that
	// moved the command elsewhere — including the copy-safe one below.
	applicable := strings.Index(hint, "If agent sessions are enabled")
	fallback := strings.Index(hint, "otherwise")
	if applicable < 0 {
		t.Errorf("the create half is not conditional, so it reads as available on every server: %q", hint)
	}
	if fallback < 0 {
		t.Errorf("the enable-and-restart clause is not conditional, so it reads as an instruction to users who need nothing: %q", hint)
	}
	if applicable >= 0 && fallback >= 0 && applicable > fallback {
		t.Errorf("the enable-and-restart fallback is read before the half that applies to a working server: %q", hint)
	}
}

// The hint is the ONLY place the create command is published — it appears in no
// doc and no skill — so its readers lift it from this line, and an agent lifts it
// to end of line. Anything after it is therefore appended to what gets run.
//
// Ending the hint with the command is the form that holds, not merely "no shell
// separator after it": this command carries a $(...) substitution and an
// assignment, so a trailing clause corrupts it however it is punctuated. A ";"
// runs the prose as a command ("otherwise: command not found"); a space makes the
// following words extra export operands; even a full stop is absorbed into the
// assigned value silently. The separator check is kept as its own assertion
// because it names the sharpest failure.
func TestNoSessionHintEndsWithTheCommandSoItCanBeLifted(t *testing.T) {
	_, after, ok := strings.Cut(NoSessionHint, SessionCreateCommand)
	if !ok {
		t.Fatalf("hint no longer carries the create command: %q", NoSessionHint)
	}
	trailing := after

	for _, separator := range []string{";", "&&", "||", "|", "&"} {
		if strings.HasPrefix(trailing, separator) {
			t.Errorf("the create command is followed by %q, so lifting the line to end-of-line runs the prose after it as a command: %q",
				separator, NoSessionHint)
		}
	}
	if trailing != "" {
		t.Errorf("the create command is not last (followed by %q); it carries $(...) and an assignment, so anything after it lands inside what gets run: %q",
			trailing, NoSessionHint)
	}
}

// One constant, rendered by both call sites. The temptation once a hint has two halves is
// to specialise it per caller, which is how the two states drift apart again.
func TestNoSessionHintIsOneLineSoBothCallSitesRenderItWhole(t *testing.T) {
	if strings.Contains(NoSessionHint, "\n") {
		t.Errorf("hint spans lines; output.Hint prefixes a single HINT: line, so a newline breaks both call sites' formatting: %q", NoSessionHint)
	}
	if strings.TrimSpace(NoSessionHint) != NoSessionHint {
		t.Errorf("hint has surrounding whitespace, which shows up after HINT:: %q", NoSessionHint)
	}
}
