package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/spf13/cobra"
)

func Text(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	params := url.Values{}
	// --full is the preferred, discoverable name; --raw is kept as a
	// backward-compatible alias. Both switch the server off its default
	// Readability extraction onto a plain document.body.innerText pull, so
	// navigation / repeated headlines / short text nodes that Readability
	// considers chrome are retained.
	raw, _ := cmd.Flags().GetBool("raw")
	full, _ := cmd.Flags().GetBool("full")
	markdown, _ := cmd.Flags().GetBool("markdown")
	switch {
	case markdown:
		params.Set("mode", "markdown")
	case raw || full:
		params.Set("mode", "raw")
		params.Set("format", "text")
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	// --frame lets a one-shot call target an iframe without having to go
	// through the stateful /frame scope. If omitted, the handler falls back
	// to the currently-scoped frame for the tab (set via `pinchtab frame`).
	if v, _ := cmd.Flags().GetString("frame"); v != "" {
		params.Set("frameId", v)
	}

	selectorStr := ""
	if len(args) > 0 {
		selectorStr = args[0]
	} else if v, _ := cmd.Flags().GetString("selector"); v != "" {
		selectorStr = v
	}
	if selectorStr != "" {
		sel := selector.Parse(selectorStr)
		if sel.Kind == selector.KindRef {
			params.Set("ref", sel.Value)
		} else {
			params.Set("selector", selectorStr)
		}
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get("/text", params)
		return
	}

	outFile, _ := cmd.Flags().GetString("output")

	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/text", params)
	var result struct {
		Text       string `json:"text"`
		Extraction string `json:"extraction"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// A plain-text response (--full/--raw set format=text) is not an envelope;
		// the whole body is the content.
		writeTextOrPrint(outFile, string(body))
		return
	}
	if result.Extraction == extractionReadabilityFallback {
		output.Hint("readability extracted only a fragment of this page; returned the full page text instead (pass --full to request it directly)")
	}
	writeTextOrPrint(outFile, result.Text)
}

// writeTextOrPrint writes the body to --output when set, printing a one-line
// confirmation so a long page never floods the agent's context, and otherwise
// prints the body. An explicit path is written as typed, overwrite included.
func writeTextOrPrint(outFile, text string) {
	if outFile == "" {
		output.Value(text)
		return
	}
	saved, err := writeOutputFile(outFile, false, []byte(text))
	if err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	printSaved(saved, len(text))
}

// extractionReadabilityFallback mirrors the /text envelope value the server
// echoes when readability collapsed and it returned the raw document instead.
const extractionReadabilityFallback = "readability_fallback"
