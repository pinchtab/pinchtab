package main

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestClickRefusesOutOfVocabularyFlagsLocallyWithTheSharedWording(t *testing.T) {
	_, modeErr := bridge.ParseClickMode("bogus")
	_, dialogErr := bridge.ParseDialogAction("dialogAction", "bogus")
	tests := []struct {
		flag       string
		valid      string
		normalized string
		want       string
	}{
		{flag: "mode", valid: " DOM ", normalized: bridge.ClickModeDOM, want: modeErr.Error()},
		{flag: "dialog-action", valid: "Dismiss", normalized: bridge.DialogActionDismiss, want: dialogErr.Error()},
	}
	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			t.Cleanup(func() { _ = clickCmd.Flags().Set(tc.flag, "") })

			if err := clickCmd.Flags().Set(tc.flag, "bogus"); err != nil {
				t.Fatal(err)
			}
			err := clickCmd.PreRunE(clickCmd, nil)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("click --%s bogus = %v, want %q", tc.flag, err, tc.want)
			}

			if err := clickCmd.Flags().Set(tc.flag, tc.valid); err != nil {
				t.Fatal(err)
			}
			if err := clickCmd.PreRunE(clickCmd, nil); err != nil {
				t.Fatalf("click --%s %q refused: %v", tc.flag, tc.valid, err)
			}
			if got, _ := clickCmd.Flags().GetString(tc.flag); got != tc.normalized {
				t.Errorf("--%s forwarded as %q, want %q", tc.flag, got, tc.normalized)
			}
		})
	}
}
