package actions

import (
	"fmt"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

func Wait(conn apiclient.Conn, args []string, cmd *cobra.Command) {
	body := map[string]any{}

	for flag, field := range map[string]string{
		"text":     "text",
		"not-text": "notText",
		"url":      "url",
		"load":     "load",
		"fn":       "fn",
	} {
		if value, _ := cmd.Flags().GetString(flag); value != "" {
			body[field] = value
		}
	}
	if len(args) > 0 {
		// Bare arg is overloaded: a number means a ms wait, anything else a selector.
		if ms, err := strconv.Atoi(args[0]); err == nil {
			body["ms"] = ms
		} else {
			body["selector"] = args[0]
		}
	}
	if len(body) == 0 {
		fmt.Println("Usage: pinchtab wait <selector|ms> [--text|--not-text|--url|--load|--fn] [--state visible|hidden|attached|detached] [--timeout-ms ms] [--tab id]")
		return
	}
	if state, _ := cmd.Flags().GetString("state"); state != "" {
		body["state"] = state
	}

	tabID, _ := cmd.Flags().GetString("tab")

	// The server's /wait timeout is milliseconds. --timeout-ms is the canonical
	// flag; --timeout is the deprecated millisecond alias (cobra prints its
	// deprecation note on use). --timeout-ms wins when both are given.
	timeoutMs, _ := cmd.Flags().GetInt("timeout-ms")
	if timeoutMs == 0 {
		timeoutMs, _ = cmd.Flags().GetInt("timeout")
	}
	if timeoutMs > 0 {
		body["timeout"] = timeoutMs
	}

	path := apiclient.TabPath(tabID, "wait")

	result := conn.PostQuiet(path, body)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		printIndented(result)
	}

	if waited, ok := result["waited"].(bool); ok && !waited {
		output.Error("wait", "timeout", output.ExitTimeout)
		return
	}
	if !jsonOutput {
		output.Success()
	}
}
