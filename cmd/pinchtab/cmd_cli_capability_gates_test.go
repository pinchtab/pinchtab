package main

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/spf13/cobra"
)

func TestEveryGatedCapabilityHasADeclaringCommandOrARecordedReason(t *testing.T) {
	if len(gatedCommands) == 0 {
		t.Fatal("no command declares a capability; the declaration table is empty and this census would pass over nothing")
	}
	if gatedCommands[storageCmd] != routes.CapStateExport {
		t.Fatalf("storage declares %q, want the state export capability it is gated by", gatedCommands[storageCmd])
	}
	declared := map[routes.Capability]bool{}
	for _, cap := range gatedCommands {
		declared[cap] = true
	}
	for _, cap := range routes.Capabilities() {
		reason, excluded := capabilitiesWithoutACommand[cap]
		switch {
		case declared[cap] && excluded:
			t.Errorf("capability %q is both declared by a command and recorded as having none: %q", cap, reason)
		case !declared[cap] && !excluded:
			t.Errorf("capability %q gates routes but no CLI command declares it; declare the command group in gatedCommands, or record why no command exists", cap)
		case excluded && strings.TrimSpace(reason) == "":
			t.Errorf("capability %q is recorded as having no command with no reason", cap)
		}
	}
}

func TestGatedCommandHelpNamesTheSettingFromTheRoutesTable(t *testing.T) {
	covered := map[*cobra.Command]string{}
	for cmd, cap := range gatedCommands {
		meta, _ := routes.Meta(cap)
		want := "Requires " + meta.Setting + "=true."
		for _, c := range append([]*cobra.Command{cmd}, cmd.Commands()...) {
			covered[c] = want
			if strings.Count(c.Long, want) != 1 || strings.Count(c.Long, "Requires security.") != 1 {
				t.Errorf("%s help does not name its gate exactly once; Long = %q, want one %q and no hand-written spelling beside it", c.CommandPath(), c.Long, want)
			}
		}
	}
	for _, name := range []string{"storage get", "storage delete", "record start", "cookies", "state save", "state list", "state"} {
		found := false
		for c := range covered {
			if c.CommandPath() == "pinchtab "+name {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is not covered by any declaration; the walk is not reaching it", name)
		}
	}

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if strings.Contains(c.Long, "Requires security.") {
			if _, ok := covered[c]; !ok {
				t.Errorf("%s hand-writes a capability sentence in its help: %q; declare the command in gatedCommands instead", c.CommandPath(), c.Long)
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)
}

func TestStorageKeyIsPositionalLikeSet(t *testing.T) {
	for _, cmd := range []*cobra.Command{storageGetCmd, storageDeleteCmd} {
		if err := cmd.ValidateArgs([]string{"k1"}); err != nil {
			t.Errorf("%s refuses a positional key: %v", cmd.CommandPath(), err)
		}
		if err := cmd.ValidateArgs([]string{"k1", "v1"}); err == nil {
			t.Errorf("%s accepts two positionals; only set takes a value", cmd.CommandPath())
		}
		if cmd.Flags().Lookup("key") == nil {
			t.Errorf("%s lost --key, which shipped scripts use", cmd.CommandPath())
		}
	}
	if storageClearCmd.Flags().Lookup("all") == nil {
		t.Error("clear lost --all")
	}
	if err := storageClearCmd.ValidateArgs([]string{"k1"}); err == nil {
		t.Error("clear accepts a positional key; it does not address a key and must not look as though it does")
	}
}
