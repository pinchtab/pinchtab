package cfchallenge

import "testing"

func TestIsChallengeTitle(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Just a moment...", true},
		{"JUST A MOMENT", true},
		{"Attention Required! | Cloudflare", true},
		{"Checking your browser before accessing", true},
		{"Example Domain", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsChallengeTitle(c.title); got != c.want {
			t.Errorf("IsChallengeTitle(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestSharedDataNonEmpty(t *testing.T) {
	if len(TitleIndicators) == 0 || len(CTypeTokens) == 0 {
		t.Fatal("shared CF substrate slices must not be empty")
	}
	if TurnstileBoxJS == "" || EmbeddedTurnstileScriptJS == "" || SpinnerText == "" {
		t.Fatal("shared CF substrate strings must not be empty")
	}
}
