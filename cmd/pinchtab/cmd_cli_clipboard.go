package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

var clipboardCmd = &cobra.Command{
	Use:   "clipboard",
	Short: "Clipboard operations",
	Long:  "Read and write browser clipboard content.",
}

var clipboardReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read clipboard text",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardRead(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var clipboardWriteCmd = &cobra.Command{
	Use:   "write <text>",
	Short: "Write text to clipboard",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardWrite(rt.client, rt.base, rt.token, cmd, strings.Join(args, " "))
		})
	},
}

var clipboardCopyCmd = &cobra.Command{
	Use:   "copy <text>",
	Short: "Copy text to clipboard (alias for write)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardWrite(rt.client, rt.base, rt.token, cmd, strings.Join(args, " "))
		})
	},
}

var clipboardPasteCmd = &cobra.Command{
	Use:   "paste",
	Short: "Paste clipboard text (alias for read)",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			clipboardRead(rt.client, rt.base, rt.token, cmd)
		})
	},
}

func clipboardRead(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	result := apiclient.DoGetRaw(client, base, token, "/clipboard/read", params)
	if result == nil {
		fmt.Fprintln(os.Stderr, "Failed to read clipboard")
		os.Exit(1)
	}

	var resp struct {
		TabID string `json:"tabId"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Text)
}

func clipboardWrite(client *http.Client, base, token string, cmd *cobra.Command, text string) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	path := "/clipboard/write"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	result := apiclient.DoPost(client, base, token, path, map[string]any{
		"text": text,
	})

	if result != nil {
		fmt.Println("Clipboard updated")
	}
}
