package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
)

// printAgentHints renders the bare-landing banner for `pinchtab` with no
// arguments. It intentionally does NOT probe localhost: running the bare
// command just to read help/next-step output must not block on a stopped or
// firewalled local server. The banner reflects the on-disk config as the
// "stopped" state; use printAgentHintsWithHealth when live server status is
// required.
func printAgentHints(cfg *config.RuntimeConfig) {
	renderAgentHints(os.Stdout, projectAgentStatus(cfg, nil, healthSnapshotStopped))
}

// printAgentHintsWithHealth probes localhost and renders the banner with live
// server status. Used by status/health-style paths that genuinely need the
// probe.
func printAgentHintsWithHealth(cfg *config.RuntimeConfig) {
	snap, state := fetchHealthSnapshot(cfg.Port)
	renderAgentHints(os.Stdout, projectAgentStatus(cfg, snap, state))
}

func renderAgentHints(out *os.File, st agentStatus) {
	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "PinchTab")+" "+cli.StyleStdout(cli.MutedStyle, version))
	_, _ = fmt.Fprintln(out)

	if st.running {
		serverStatus := "running"
		serverStyle := cli.SuccessStyle
		if st.guardsDown {
			serverStatus = "running (YOLO — guards down for this run)"
			serverStyle = cli.WarningStyle
		}
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "server", cli.StyleStdout(serverStyle, serverStatus))
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "listen", cli.StyleStdout(cli.ValueStyle, st.listenAddr))
		if len(st.sensitive) > 0 {
			_, _ = fmt.Fprintf(out, "  %-20s %s\n", "sensitive", cli.StyleStdout(cli.WarningStyle, strings.Join(st.sensitive, ", ")))
		}
	} else {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "server", cli.StyleStdout(cli.WarningStyle, string(st.state)))
	}

	formatted := formatAllowedDomains(st.allowedDomains)
	domStyle := cli.ValueStyle
	if formatted == "all" {
		domStyle = cli.WarningStyle
	}
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "allowedDomains", cli.StyleStdout(domStyle, formatted))

	idpiStatus := "disabled"
	idpiStyle := cli.WarningStyle
	if st.idpiEnabled {
		idpiStatus = "enabled"
		idpiStyle = cli.SuccessStyle
	}
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "idpi", cli.StyleStdout(idpiStyle, idpiStatus))
	_, _ = fmt.Fprintln(out)

	cli.WriteCommandHints(out, "Next steps:", st.nextSteps, st.nextStepsWidth, true)
	_, _ = fmt.Fprintln(out)

	cli.WriteCommandHints(out, "Configure:", []cli.CommandHint{
		{Command: "pinchtab config show", Comment: "# view current config"},
		{Command: "pinchtab security", Comment: "# review security posture"},
		{Command: "pinchtab --help", Comment: "# full command list"},
	}, 44, true)
}
