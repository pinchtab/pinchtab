package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/server"
)

func printConfigOverview(cfg *config.RuntimeConfig) {
	_, configPath, err := config.LoadFileConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("Error loading config path: %v", err)))
		os.Exit(1)
	}

	dashPort := cfg.Port
	dashboardURL := fmt.Sprintf("http://localhost:%s", dashPort)
	running := server.CheckPinchTabRunning(dashPort, cfg.Token)

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "Config"))
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "strategy", cli.StyleStdout(cli.ValueStyle, cfg.Strategy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "allocation policy", cli.StyleStdout(cli.ValueStyle, cfg.AllocationPolicy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "stealth level", cli.StyleStdout(cli.ValueStyle, cfg.StealthLevel))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "tab eviction", cli.StyleStdout(cli.ValueStyle, cfg.TabEvictionPolicy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "tab lifecycle", cli.StyleStdout(cli.ValueStyle, formatTabLifecycle(cfg)))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "file", cli.StyleStdout(cli.ValueStyle, configPath))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "token", cli.StyleStdout(cli.ValueStyle, config.MaskToken(cfg.Token)))
	if running {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "dashboard", cli.StyleStdout(cli.ValueStyle, dashboardURL))
	} else {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "dashboard", cli.StyleStdout(cli.MutedStyle, "not running"))
	}
	_, _ = fmt.Fprintln(out)

	cli.WriteCommandHints(out, "Change config:", []cli.CommandHint{
		{Command: "pinchtab config get <path>", Comment: "# read a value (e.g. server.port)"},
		{Command: "pinchtab config set <path> <value>", Comment: "# update a value"},
		{Command: "pinchtab config show", Comment: "# print labelled config summary"},
		{Command: "pinchtab config token", Comment: "# copy API token to clipboard"},
		{Command: "pinchtab security", Comment: "# review security posture"},
	}, 44, true)
	_, _ = fmt.Fprintf(out, "  %s %s\n", cli.StyleStdout(cli.MutedStyle, "Or edit the file directly:"), cli.StyleStdout(cli.ValueStyle, configPath))
}

func formatTabLifecycle(cfg *config.RuntimeConfig) string {
	policy := cfg.TabLifecyclePolicy
	if policy == "" {
		policy = "keep"
	}
	if policy == "close_idle" && cfg.TabCloseDelay > 0 {
		return fmt.Sprintf("%s (%s)", policy, cfg.TabCloseDelay)
	}
	return policy
}
