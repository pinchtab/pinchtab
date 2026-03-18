package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

var snapCmd = &cobra.Command{
	Use:   "snap",
	Short: "Snapshot accessibility tree",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Snapshot(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var screenshotCmd = &cobra.Command{
	Use:   "screenshot",
	Short: "Take a screenshot",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Screenshot(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var evalCmd = &cobra.Command{
	Use:   "eval <expression>",
	Short: "Evaluate JavaScript",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Evaluate(rt.client, rt.base, rt.token, args, cmd)
		})
	},
}

var pdfCmd = &cobra.Command{
	Use:   "pdf",
	Short: "Export the current page as PDF",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.PDF(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var textCmd = &cobra.Command{
	Use:   "text",
	Short: "Extract page text",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Text(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var downloadCmd = &cobra.Command{
	Use:   "download <url>",
	Short: "Download a file via browser session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = urls.Normalize(args[0])
		runCLI(func(rt cliRuntime) {
			browseractions.Download(rt.client, rt.base, rt.token, args, stringFlag(cmd, "output"))
		})
	},
}

var uploadCmd = &cobra.Command{
	Use:   "upload <file-path>",
	Short: "Upload a file to a file input element",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Upload(rt.client, rt.base, rt.token, args, stringFlag(cmd, "selector"))
		})
	},
}

var findCmd = &cobra.Command{
	Use:   "find <query>",
	Short: "Find elements by natural language query",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Find(rt.client, rt.base, rt.token, args[0], cmd)
		})
	},
}

var waitCmd = &cobra.Command{
	Use:   "wait [selector|ms]",
	Short: "Wait for element, text, URL, network idle, JS expression, or fixed duration",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Wait(rt.client, rt.base, rt.token, args, cmd)
		})
	},
}

var networkCmd = &cobra.Command{
	Use:   "network [requestId]",
	Short: "List or inspect network requests",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Network(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}
