package cli

import (
	"fmt"
	"io"
)

// CommandHint is one "<command>  <comment>" row in a CLI hint group.
type CommandHint struct {
	Command string
	Comment string
}

// WriteCommandHints renders a heading followed by aligned command/comment rows.
// When styled, the heading/command/comment use the cli styles (ANSI); otherwise
// they are emitted plain. width is the command-column pad (the %-44s/%-64s the
// call sites used inline); padding counts ANSI bytes when styled, matching the
// previous inline behavior exactly.
func WriteCommandHints(out io.Writer, heading string, hints []CommandHint, width int, styled bool) {
	if styled {
		_, _ = fmt.Fprintln(out, StyleStdout(HeadingStyle, heading))
	} else {
		_, _ = fmt.Fprintln(out, heading)
	}
	for _, h := range hints {
		cmd, comment := h.Command, h.Comment
		if styled {
			cmd = StyleStdout(CommandStyle, cmd)
			comment = StyleStdout(MutedStyle, comment)
		}
		_, _ = fmt.Fprintf(out, "  %-*s %s\n", width, cmd, comment)
	}
}

// NextStepsRunningHints is the "Next steps" group shown when the server is up;
// shared by the root banner and `pinchtab health` so the two stay in lockstep.
var NextStepsRunningHints = []CommandHint{
	{"export PINCHTAB_SESSION=$(pinchtab session create --agent-id <id>)", "# start a dedicated session"},
	{"pinchtab nav <url>", "# open a page in the current tab"},
	{"pinchtab snap", "# inspect interactive elements"},
}
