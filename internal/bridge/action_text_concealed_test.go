package bridge

import (
	"strings"
	"testing"
)

// Tests for concealedReply — the F4 redaction helper that prevents the
// fill/type action handlers from echoing plaintext credentials in their
// response bodies. See valkyriweb/pinchtab smilerite/v0.8.6-hardening
// branch (origin: SMI-81 retro Finding 4).

func TestConcealedReply_NotConcealed_EchoesPlaintext(t *testing.T) {
	req := ActionRequest{Text: "hello", Concealed: false}
	out := concealedReply("filled", req, nil)

	if got, want := out["filled"], "hello"; got != want {
		t.Fatalf("expected echoed plaintext %q, got %v", want, got)
	}
	if _, has := out["concealed"]; has {
		t.Errorf("did not expect concealed flag on non-concealed reply, got %v", out)
	}
	if _, has := out["len"]; has {
		t.Errorf("did not expect len field on non-concealed reply, got %v", out)
	}
}

func TestConcealedReply_Concealed_Redacts(t *testing.T) {
	req := ActionRequest{Text: "hunter2", Concealed: true}
	out := concealedReply("filled", req, nil)

	if got, want := out["filled"], "[REDACTED]"; got != want {
		t.Fatalf("expected redacted marker %q, got %v", want, got)
	}
	if got, want := out["len"], 7; got != want {
		t.Errorf("expected len=%d, got %v", want, got)
	}
	if got, want := out["concealed"], true; got != want {
		t.Errorf("expected concealed=true, got %v", got)
	}

	// Critical correctness: the plaintext value MUST NOT appear anywhere
	// in the reply. Walk every value (string-typed) and fail if it
	// matches the original text. This catches future regressions like
	// "extras shadow concealed" or "verb mistyped".
	for k, v := range out {
		if s, ok := v.(string); ok && strings.Contains(s, req.Text) {
			t.Fatalf("plaintext %q leaked into reply key %q (value %q)", req.Text, k, s)
		}
	}
}

func TestConcealedReply_Concealed_PreservesExtras(t *testing.T) {
	// actionHumanType returns {"typed": req.Text, "human": true} — the
	// "human" flag must survive the redaction path.
	req := ActionRequest{Text: "secret", Concealed: true}
	out := concealedReply("typed", req, map[string]any{"human": true})

	if got, want := out["human"], true; got != want {
		t.Errorf("expected extras.human=true to survive, got %v", got)
	}
	if got, want := out["typed"], "[REDACTED]"; got != want {
		t.Errorf("expected typed redacted, got %v", got)
	}
}

func TestConcealedReply_Concealed_EmptyText(t *testing.T) {
	// Edge: caller passes Concealed:true with empty Text. Still redact
	// (don't leak the fact that the field was empty by echoing "" vs
	// "[REDACTED]" — uniform shape).
	req := ActionRequest{Text: "", Concealed: true}
	out := concealedReply("filled", req, nil)

	if got, want := out["filled"], "[REDACTED]"; got != want {
		t.Errorf("expected redacted for empty concealed text, got %v", got)
	}
	if got, want := out["len"], 0; got != want {
		t.Errorf("expected len=0, got %v", got)
	}
	if got, want := out["concealed"], true; got != want {
		t.Errorf("expected concealed=true, got %v", got)
	}
}

func TestConcealedReply_Concealed_UnicodeLen(t *testing.T) {
	// len() returns byte length, not rune count. That's intentional:
	// "did pinchtab type the whole thing?" is a byte-level question
	// once chromedp hands the string to chromium. Document the choice
	// via a test so a future "switch to utf8.RuneCountInString" doesn't
	// land without re-thinking the contract.
	req := ActionRequest{Text: "héllo", Concealed: true} // 6 bytes, 5 runes
	out := concealedReply("filled", req, nil)

	if got, want := out["len"], len(req.Text); got != want {
		t.Errorf("expected len=%d bytes, got %v", want, got)
	}
}

func TestActionRequest_JSON_ConcealedOmitemptyDefault(t *testing.T) {
	// Concealed is an opt-in flag. The struct tag is `json:"concealed,omitempty"`
	// so callers that don't know about it (e.g. older Smiles instructions)
	// continue to work unchanged — zero value = false = echo behavior.
	req := ActionRequest{Text: "hello"}
	if req.Concealed {
		t.Fatalf("default Concealed must be false (opt-in)")
	}
}
