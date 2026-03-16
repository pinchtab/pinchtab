package actions

import (
	"testing"

	"github.com/spf13/cobra"
)

func newActivityCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Int("age-sec", 0, "")
	return cmd
}

func TestActivityParams(t *testing.T) {
	cmd := newActivityCmd()
	if err := cmd.Flags().Set("limit", "50"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("age-sec", "300"); err != nil {
		t.Fatal(err)
	}

	got := activityParams(cmd, "").Encode()
	if got != "ageSec=300&limit=50" {
		t.Fatalf("expected ageSec and limit query, got %q", got)
	}
}

func TestActivityTabParams(t *testing.T) {
	cmd := newActivityCmd()
	if err := cmd.Flags().Set("limit", "25"); err != nil {
		t.Fatal(err)
	}

	got := activityParams(cmd, "tab_123").Encode()
	if got != "limit=25&tabId=tab_123" {
		t.Fatalf("expected limit and tabId query, got %q", got)
	}
}
