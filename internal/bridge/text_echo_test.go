package bridge

import "testing"

func TestRedactedEchoHasNoPlaintext(t *testing.T) {
	secret := "hunter2-hunter2"
	out := redactedEcho(echoKeyTyped, secret, map[string]any{"human": true})
	if _, ok := out[echoKeyTyped]; ok {
		t.Fatalf("redactedEcho leaked plaintext key: %v", out)
	}
	if out["typed_len"] != len([]rune(secret)) {
		t.Fatalf("typed_len = %v, want %d", out["typed_len"], len([]rune(secret)))
	}
	if out["redacted"] != true {
		t.Fatalf("redacted flag missing: %v", out)
	}
	if out["human"] != true {
		t.Fatalf("extra keys dropped: %v", out)
	}
	for _, v := range out {
		if s, ok := v.(string); ok && s == secret {
			t.Fatalf("secret present in echo: %v", out)
		}
	}
}

func TestRedactedTextEchoAlwaysRedacts(t *testing.T) {
	out := RedactedTextEcho("s3cret")
	if _, ok := out["typed"]; ok {
		t.Fatalf("lite echo leaked plaintext: %v", out)
	}
	if out["redacted"] != true || out["typed_len"] != 6 {
		t.Fatalf("unexpected lite echo: %v", out)
	}
}

func TestPlainEchoKeepsLegacyShape(t *testing.T) {
	out := plainEcho(echoKeyFilled, "cape town", nil)
	if out[echoKeyFilled] != "cape town" {
		t.Fatalf("plainEcho changed shape: %v", out)
	}
	if _, ok := out["redacted"]; ok {
		t.Fatalf("plainEcho should not set redacted: %v", out)
	}
}
