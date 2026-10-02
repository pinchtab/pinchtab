package main

import (
	"strings"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/spf13/cobra"
)

// addTabFlag wires a --tab flag onto the given commands. Anonymous CLI calls
// default its value from the state file written by `nav`, which lets local
// single-agent workflows avoid threading `--tab "$TAB"` through every command:
//
//	pinchtab nav http://example.com   # writes tab ID to state file
//	pinchtab snap -i -c               # auto-reads from state file
//
// Explicit --tab still wins (cobra flag precedence). Identified callers
// (PINCHTAB_SESSION, --agent-id, or PINCHTAB_AGENT_ID) leave --tab unset so the
// server-side scoped current-tab store is authoritative. If no state file is
// set, the server picks the active tab as before.
func addTabFlag(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.Flags().String("tab", "", "Tab ID")
		tabFlagCommands = append(tabFlagCommands, cmd)
		existingPreRun := cmd.PreRun
		existingPreRunE := cmd.PreRunE
		cmd.PreRun = nil
		cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
			defaultTabFlagFromState(cmd)
			if existingPreRun != nil {
				existingPreRun(cmd, args)
			}
			if existingPreRunE != nil {
				return existingPreRunE(cmd, args)
			}
			return nil
		}
	}
}

var tabFlagCommands []*cobra.Command

func addJSONFlag(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.Flags().Bool("json", false, "Output full JSON response instead of terse status")
	}
}

func addPointFlags(cmd *cobra.Command, action string) {
	cmd.Flags().Float64("x", 0, "X coordinate for "+action)
	cmd.Flags().Float64("y", 0, "Y coordinate for "+action)
}

// addPointerActionFlags adds the css-selector, point-coordinate, and humanize
// flags shared by pointer actions. Callers add any action-specific flags after.
func addPointerActionFlags(cmd *cobra.Command, action string) {
	cmd.Flags().String("css", "", "CSS selector instead of ref")
	addPointFlags(cmd, action)
	cmd.Flags().Bool("humanize", false, "Use humanized bezier+jitter input path (overrides instance config)")
}

// addPostActionFlags registers the standard post-action output flags (snap,
// snap-diff, and optionally text) with descriptions interpolated from verb, so
// the bundle is defined once instead of repeated across browser commands.
func addPostActionFlags(cmd *cobra.Command, verb string, withText bool) {
	cmd.Flags().Bool("snap", false, "Output interactive snapshot after "+verb)
	cmd.Flags().Bool("snap-diff", false, "Output snapshot diff after "+verb+" ("+observe.CompactDiffSummary+")")
	if withText {
		cmd.Flags().Bool("text", false, "Output page text after "+verb+" (for verification)")
	}
}

func addVocabRefusal(cmd *cobra.Command, flag string, parse func(string) (string, error)) {
	previous := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if raw, _ := c.Flags().GetString(flag); raw != "" {
			normalized, err := parse(raw)
			if err != nil {
				return err
			}
			if err := c.Flags().Set(flag, normalized); err != nil {
				return err
			}
		}
		if previous != nil {
			return previous(c, args)
		}
		return nil
	}
}

// addMouseButtonFlag registers --button and the local refusal together, so the help text,
// the default and the accepted set all come from the one vocabulary owner rather than being
// spelled out per command. The refusal is a fast path for a typo, NOT the guard: the HTTP
// body is validated server-side because the CLI is not the only client.
func addMouseButtonFlag(cmd *cobra.Command) {
	cmd.Flags().String("button", bridgecdpops.DefaultMouseButton,
		"Mouse button: "+strings.Join(bridgecdpops.MouseButtons(), ", "))
	previous := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		button, _ := c.Flags().GetString("button")
		if err := bridgecdpops.ValidateMouseButton(button); err != nil {
			return err
		}
		if previous != nil {
			return previous(c, args)
		}
		return nil
	}
}
