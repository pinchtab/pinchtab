package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/urlutil"
	"github.com/spf13/cobra"
)

var quickCmd = &cobra.Command{
	Use:   "quick <url>",
	Short: "Navigate + analyze page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = urlutil.Normalize(args[0])
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Quick(client, base, token, args)
		})
	},
}

var navCmd = &cobra.Command{
	Use:     "nav <url>",
	Aliases: []string{"goto", "navigate", "open"},
	Short:   "Navigate to URL",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := urlutil.Normalize(args[0])
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Navigate(client, base, token, url, cmd)
		})
	},
}

var backCmd = &cobra.Command{
	Use:   "back",
	Short: "Go back in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Back(client, base, token, cmd)
		})
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "Go forward in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Forward(client, base, token, cmd)
		})
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload current page",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Reload(client, base, token, cmd)
		})
	},
}

var snapCmd = &cobra.Command{
	Use:   "snap",
	Short: "Snapshot accessibility tree",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Snapshot(client, base, token, cmd)
		})
	},
}

var clickCmd = &cobra.Command{
	Use:   "click <ref>",
	Short: "Click element",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		}
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Action(client, base, token, "click", ref, cmd)
		})
	},
}

var dblclickCmd = &cobra.Command{
	Use:   "dblclick <ref>",
	Short: "Double-click element",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		}
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Action(client, base, token, "dblclick", ref, cmd)
		})
	},
}

var typeCmd = &cobra.Command{
	Use:   "type <ref> <text>",
	Short: "Type into element",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.ActionSimple(client, base, token, "type", args, cmd)
		})
	},
}

var screenshotCmd = &cobra.Command{
	Use:   "screenshot",
	Short: "Take a screenshot",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Screenshot(client, base, token, cmd)
		})
	},
}

var tabsCmd = &cobra.Command{
	Use:     "tab [id]",
	Aliases: []string{"tabs"},
	Short:   "List tabs, or focus a tab by ID",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			if len(args) == 0 {
				browseractions.TabList(client, base, token)
			} else {
				browseractions.TabFocus(client, base, token, args[0])
			}
		})
	},
}

var instancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "List or manage instances",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Instances(client, base, token)
		})
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check server health",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Health(client, base, token)
		})
	},
}

var pressCmd = &cobra.Command{
	Use:   "press <key>",
	Short: "Press key (Enter, Tab, Escape...)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.ActionSimple(client, base, token, "press", args, cmd)
		})
	},
}

var fillCmd = &cobra.Command{
	Use:   "fill <ref|selector> <text>",
	Short: "Fill input directly",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.ActionSimple(client, base, token, "fill", args, cmd)
		})
	},
}

var hoverCmd = &cobra.Command{
	Use:   "hover <ref>",
	Short: "Hover element",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		}
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Action(client, base, token, "hover", ref, cmd)
		})
	},
}

var scrollCmd = &cobra.Command{
	Use:   "scroll <ref|pixels>",
	Short: "Scroll to element or by pixels",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.ActionSimple(client, base, token, "scroll", args, cmd)
		})
	},
}

var evalCmd = &cobra.Command{
	Use:   "eval <expression>",
	Short: "Evaluate JavaScript",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Evaluate(client, base, token, args, cmd)
		})
	},
}

var pdfCmd = &cobra.Command{
	Use:   "pdf",
	Short: "Export the current page as PDF",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.PDF(client, base, token, cmd)
		})
	},
}

var textCmd = &cobra.Command{
	Use:   "text",
	Short: "Extract page text",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Text(client, base, token, cmd)
		})
	},
}

var downloadCmd = &cobra.Command{
	Use:   "download <url>",
	Short: "Download a file via browser session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = urlutil.Normalize(args[0])
		output, _ := cmd.Flags().GetString("output")
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Download(client, base, token, args, output)
		})
	},
}

var uploadCmd = &cobra.Command{
	Use:   "upload <file-path>",
	Short: "Upload a file to a file input element",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		selector, _ := cmd.Flags().GetString("selector")
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Upload(client, base, token, args, selector)
		})
	},
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List browser profiles",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Profiles(client, base, token)
		})
	},
}

var instanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "Manage browser instances",
}

var findCmd = &cobra.Command{
	Use:   "find <query>",
	Short: "Find elements by natural language query",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.Find(client, base, token, args[0], cmd)
		})
	},
}

var selectCmd = &cobra.Command{
	Use:   "select <ref> <value>",
	Short: "Select option in dropdown",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		runCLIWith(cfg, func(client *http.Client, base, token string) {
			browseractions.ActionSimple(client, base, token, "select", args, cmd)
		})
	},
}

func init() {
	quickCmd.GroupID = "browser"
	navCmd.GroupID = "browser"
	backCmd.GroupID = "browser"
	forwardCmd.GroupID = "browser"
	reloadCmd.GroupID = "browser"
	snapCmd.GroupID = "browser"
	clickCmd.GroupID = "browser"
	typeCmd.GroupID = "browser"
	screenshotCmd.GroupID = "browser"
	tabsCmd.GroupID = "browser"
	instancesCmd.GroupID = "management"
	healthCmd.GroupID = "management"
	pressCmd.GroupID = "browser"
	fillCmd.GroupID = "browser"
	hoverCmd.GroupID = "browser"
	scrollCmd.GroupID = "browser"
	evalCmd.GroupID = "browser"
	pdfCmd.GroupID = "browser"
	textCmd.GroupID = "browser"
	profilesCmd.GroupID = "management"
	downloadCmd.GroupID = "browser"
	uploadCmd.GroupID = "browser"
	findCmd.GroupID = "browser"
	selectCmd.GroupID = "browser"

	tabsCmd.AddCommand(&cobra.Command{
		Use:   "new [url]",
		Short: "Open a new tab",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				body := map[string]any{"action": "new"}
				if len(args) > 0 {
					body["url"] = urlutil.Normalize(args[0])
				}
				browseractions.TabNew(client, base, token, body)
			})
		},
	})
	tabsCmd.AddCommand(&cobra.Command{
		Use:   "close <id>",
		Short: "Close a tab by ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				browseractions.TabClose(client, base, token, args[0])
			})
		},
	})

	uploadCmd.Flags().StringP("selector", "s", "", "CSS selector for file input")
	downloadCmd.Flags().StringP("output", "o", "", "Save downloaded file to path")

	clickCmd.Flags().String("css", "", "CSS selector instead of ref")
	clickCmd.Flags().Float64("x", 0, "X coordinate for click")
	clickCmd.Flags().Float64("y", 0, "Y coordinate for click")
	clickCmd.Flags().Bool("wait-nav", false, "Wait for navigation after click")
	dblclickCmd.Flags().String("css", "", "CSS selector instead of ref")
	dblclickCmd.Flags().Float64("x", 0, "X coordinate for dblclick")
	dblclickCmd.Flags().Float64("y", 0, "Y coordinate for dblclick")
	hoverCmd.Flags().String("css", "", "CSS selector instead of ref")
	hoverCmd.Flags().Float64("x", 0, "X coordinate for hover")
	hoverCmd.Flags().Float64("y", 0, "Y coordinate for hover")

	snapCmd.Flags().BoolP("interactive", "i", false, "Filter interactive elements only")
	snapCmd.Flags().BoolP("compact", "c", false, "Compact output format")
	snapCmd.Flags().Bool("text", false, "Text output format")
	snapCmd.Flags().BoolP("diff", "d", false, "Show diff from previous snapshot")
	snapCmd.Flags().StringP("selector", "s", "", "CSS selector to scope snapshot")
	snapCmd.Flags().String("max-tokens", "", "Maximum token budget")
	snapCmd.Flags().String("depth", "", "Tree depth limit")
	snapCmd.Flags().String("tab", "", "Tab ID")

	screenshotCmd.Flags().StringP("output", "o", "", "Save screenshot to file path")
	screenshotCmd.Flags().StringP("quality", "q", "", "JPEG quality (0-100)")
	screenshotCmd.Flags().String("tab", "", "Tab ID")

	pdfCmd.Flags().StringP("output", "o", "", "Save PDF to file path")
	pdfCmd.Flags().String("tab", "", "Tab ID")
	pdfCmd.Flags().Bool("landscape", false, "Landscape orientation")
	pdfCmd.Flags().String("scale", "", "Page scale (e.g. 0.5)")
	pdfCmd.Flags().String("paper-width", "", "Paper width (inches)")
	pdfCmd.Flags().String("paper-height", "", "Paper height (inches)")
	pdfCmd.Flags().String("margin-top", "", "Top margin")
	pdfCmd.Flags().String("margin-bottom", "", "Bottom margin")
	pdfCmd.Flags().String("margin-left", "", "Left margin")
	pdfCmd.Flags().String("margin-right", "", "Right margin")
	pdfCmd.Flags().String("page-ranges", "", "Page ranges (e.g. 1-3)")
	pdfCmd.Flags().Bool("prefer-css-page-size", false, "Use CSS page size")
	pdfCmd.Flags().Bool("display-header-footer", false, "Show header/footer")
	pdfCmd.Flags().String("header-template", "", "Header HTML template")
	pdfCmd.Flags().String("footer-template", "", "Footer HTML template")
	pdfCmd.Flags().Bool("generate-tagged-pdf", false, "Generate tagged PDF")
	pdfCmd.Flags().Bool("generate-document-outline", false, "Generate document outline")
	pdfCmd.Flags().Bool("file-output", false, "Use server-side file output")
	pdfCmd.Flags().String("path", "", "Server-side output path")

	findCmd.Flags().String("tab", "", "Tab ID")
	findCmd.Flags().String("threshold", "", "Minimum similarity score (0-1)")
	findCmd.Flags().Bool("explain", false, "Show score breakdown")
	findCmd.Flags().Bool("ref-only", false, "Output just the element ref")

	textCmd.Flags().Bool("raw", false, "Raw extraction mode")
	textCmd.Flags().String("tab", "", "Tab ID")

	navCmd.Flags().Bool("new-tab", false, "Open in new tab")
	navCmd.Flags().Bool("block-images", false, "Block image loading")
	navCmd.Flags().Bool("block-ads", false, "Block ads")
	navCmd.Flags().String("tab", "", "Tab ID")
	backCmd.Flags().String("tab", "", "Tab ID")
	forwardCmd.Flags().String("tab", "", "Tab ID")
	reloadCmd.Flags().String("tab", "", "Tab ID")

	clickCmd.Flags().String("tab", "", "Tab ID")
	dblclickCmd.Flags().String("tab", "", "Tab ID")
	hoverCmd.Flags().String("tab", "", "Tab ID")
	typeCmd.Flags().String("tab", "", "Tab ID")
	pressCmd.Flags().String("tab", "", "Tab ID")
	fillCmd.Flags().String("tab", "", "Tab ID")
	scrollCmd.Flags().String("tab", "", "Tab ID")
	selectCmd.Flags().String("tab", "", "Tab ID")
	evalCmd.Flags().String("tab", "", "Tab ID")

	rootCmd.AddCommand(quickCmd)
	rootCmd.AddCommand(navCmd)
	rootCmd.AddCommand(backCmd)
	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(reloadCmd)
	rootCmd.AddCommand(snapCmd)
	rootCmd.AddCommand(clickCmd)
	rootCmd.AddCommand(dblclickCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(screenshotCmd)
	rootCmd.AddCommand(tabsCmd)
	rootCmd.AddCommand(instancesCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(pressCmd)
	rootCmd.AddCommand(fillCmd)
	rootCmd.AddCommand(hoverCmd)
	rootCmd.AddCommand(scrollCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(pdfCmd)
	rootCmd.AddCommand(textCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(selectCmd)

	instanceCmd.GroupID = "management"

	startInstanceCmd := &cobra.Command{
		Use:   "start",
		Short: "Start a browser instance",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				browseractions.InstanceStart(client, base, token, cmd)
			})
		},
	}
	startInstanceCmd.Flags().String("profile", "", "Profile to use")
	startInstanceCmd.Flags().String("mode", "", "Instance mode")
	startInstanceCmd.Flags().String("port", "", "Port number")
	startInstanceCmd.Flags().StringArray("extension", nil, "Load browser extension (repeatable)")
	instanceCmd.AddCommand(startInstanceCmd)

	instanceCmd.AddCommand(&cobra.Command{
		Use:   "navigate <id> <url>",
		Short: "Navigate an instance to a URL",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			args[1] = urlutil.Normalize(args[1])
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				browseractions.InstanceNavigate(client, base, token, args)
			})
		},
	})
	instanceCmd.AddCommand(&cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a browser instance",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				browseractions.InstanceStop(client, base, token, args)
			})
		},
	})
	instanceCmd.AddCommand(&cobra.Command{
		Use:   "logs <id>",
		Short: "Get instance logs",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.Load()
			runCLIWith(cfg, func(client *http.Client, base, token string) {
				browseractions.InstanceLogs(client, base, token, args)
			})
		},
	})
	rootCmd.AddCommand(instanceCmd)
}

func runCLIWith(cfg *config.RuntimeConfig, fn func(client *http.Client, base, token string)) {
	client := &http.Client{Timeout: 60 * time.Second}

	// Default: http://127.0.0.1:{port}
	port := cfg.Port
	if port == "" {
		port = "9867"
	}
	base := fmt.Sprintf("http://127.0.0.1:%s", port)

	// --server flag overrides
	if serverURL != "" {
		base = strings.TrimRight(serverURL, "/")
	}

	// Token from config, env var overrides
	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}

	fn(client, base, token)
}
