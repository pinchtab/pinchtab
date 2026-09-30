package bridge

import (
	"strings"
	"testing"
)

func TestPopupGuardInitScriptNeutralizesOpener(t *testing.T) {
	for _, want := range []string{"window.open", "noopener", "noreferrer", "window.opener"} {
		if !strings.Contains(popupGuardInitScript, want) {
			t.Fatalf("expected popup guard script to contain %q", want)
		}
	}
}
