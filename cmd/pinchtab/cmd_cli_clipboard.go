package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

var clipboardCmd = &cobra.Command{
	Use:   "clipboard",
	Short: "Clipboard operations",
	Long:  "Read and write the shared server clipboard.",
}

var clipboardReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read clipboard text",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardRead(rt.Client, rt.Base, rt.Token)
		})
	},
}

var clipboardWriteCmd = &cobra.Command{
	Use:   "write <text>",
	Short: "Write text to clipboard",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardWrite(rt.Client, rt.Base, rt.Token, args)
		})
	},
}

var clipboardCopyCmd = &cobra.Command{
	Use:   "copy <text>",
	Short: "Copy text to clipboard (alias for write)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardWrite(rt.Client, rt.Base, rt.Token, args)
		})
	},
}

var clipboardPasteCmd = &cobra.Command{
	Use:   "paste",
	Short: "Paste clipboard text (alias for read)",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardRead(rt.Client, rt.Base, rt.Token)
		})
	},
}

func clipboardRead(client *http.Client, base, token string) {
	result := apiclient.DoGetRaw(client, base, token, "/clipboard/read", nil)
	if result == nil {
		fmt.Fprintln(os.Stderr, "Failed to read clipboard")
		os.Exit(1)
	}

	var resp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Text)
}

func clipboardWrite(client *http.Client, base, token string, args []string) {
	result := apiclient.DoPost(client, base, token, "/clipboard/write", map[string]any{
		"text": strings.Join(args, " "),
	})

	if result != nil {
		fmt.Println("Clipboard updated")
	}
}
