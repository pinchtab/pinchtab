package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

var snapCmd = &cobra.Command{
	Use:   "snap [selector]",
	Short: "Snapshot accessibility tree",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		selector := ""
		if len(args) > 0 && stringFlag(cmd, "selector") == "" {
			selector = args[0]
		}
		runCLI(func(rt cliRuntime) {
			browseractions.Snapshot(rt.client, rt.base, rt.token, cmd, selector)
		})
	},
}

var frameCmd = &cobra.Command{
	Use:   "frame [target|main]",
	Short: "Show or set the current frame scope",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Frame(rt.client, rt.base, rt.token, args, cmd)
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

var annotateCmd = &cobra.Command{
	Use:   "annotate",
	Short: "Inject a persistent, clickable annotation overlay on the live page (--clear to remove)",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Annotate(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var captureCmd = &cobra.Command{
	Use:     "capture",
	Aliases: []string{"cap"},
	Short:   "Paired screenshot + accessibility snapshot from the same DOM epoch",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Capture(rt.client, rt.base, rt.token, cmd)
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
	Use:   "text [selector]",
	Short: "Extract page text",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Text(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var titleCmd = &cobra.Command{
	Use:   "title",
	Short: "Get the current tab title",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Title(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var urlCmd = &cobra.Command{
	Use:   "url",
	Short: "Get the current tab URL",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.URL(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var htmlCmd = &cobra.Command{
	Use:   "html [selector]",
	Short: "Get document or element HTML",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.HTML(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var stylesCmd = &cobra.Command{
	Use:   "styles [selector]",
	Short: "Get computed styles for the root element or a matched element",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Styles(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var valueCmd = &cobra.Command{
	Use:   "value <ref>",
	Short: "Get the current value of a form element by ref",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Value(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var attrCmd = &cobra.Command{
	Use:   "attr <ref> <name>",
	Short: "Get the value of an HTML attribute by ref",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Attr(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var boxCmd = &cobra.Command{
	Use:   "box <ref>",
	Short: "Get the bounding box of an element by ref",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Box(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var visibleCmd = &cobra.Command{
	Use:   "visible <ref>",
	Short: "Check if an element is visible by ref",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Visible(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var enabledCmd = &cobra.Command{
	Use:   "enabled <ref>",
	Short: "Check if an element is enabled by ref",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Enabled(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var checkedCmd = &cobra.Command{
	Use:   "checked <ref>",
	Short: "Check if an element is checked by ref",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Checked(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var countCmd = &cobra.Command{
	Use:   "count <selector>",
	Short: "Count elements matching a CSS selector",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Count(rt.client, rt.base, rt.token, cmd, args)
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
