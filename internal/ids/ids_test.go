package ids

import (
	"regexp"
	"testing"
)

func TestProfileIDIsStableAndPrefixed(t *testing.T) {
	got := ProfileID("work")
	if !regexp.MustCompile(`^prof_[0-9a-f]{8}$`).MatchString(got) {
		t.Fatalf("ProfileID = %q, want prof_ + 8 hex chars", got)
	}
	if again := ProfileID("work"); again != got {
		t.Fatalf("ProfileID not stable: %q then %q", got, again)
	}
	if other := ProfileID("home"); other == got {
		t.Fatalf("distinct names share ProfileID %q", got)
	}
}

func TestInstanceIDIsPrefixed(t *testing.T) {
	got := InstanceID(ProfileID("work"), "work")
	if !regexp.MustCompile(`^inst_[0-9a-f]{8}$`).MatchString(got) {
		t.Fatalf("InstanceID = %q, want inst_ + 8 hex chars", got)
	}
}
