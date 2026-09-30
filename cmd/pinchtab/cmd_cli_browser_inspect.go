package main

import (
	"fmt"

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
	Short: "Check if an element is rendered (display/visibility/opacity/size) by ref",
	Long: `Check if an element is rendered by ref: display, visibility, opacity and a
non-zero box. Scroll position is not an input, so an element far below the fold
is still rendered. Use --json to also read onScreen, the viewport-intersection
answer that the capture snapshot's per-node visible field publishes.`,
	Args: cobra.ExactArgs(1),
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
			browseractions.Download(rt.client, rt.base, rt.token, args, stringFlag(cmd, "output"), stringFlag(cmd, "tab"))
		})
	},
}

var uploadCmd = &cobra.Command{
	Use:   "upload <file-path>",
	Short: "Upload a file to a file input element",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Upload(rt.client, rt.base, rt.token, args, stringFlag(cmd, "selector"), stringFlag(cmd, "tab"))
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

var extractCmd = &cobra.Command{
	Use:   "extract --schema <file|->",
	Short: "Extract schema-typed data from the page as JSON",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Extract(rt.client, rt.base, rt.token, cmd)
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

func configureBrowserInspectFlags() {
	uploadCmd.Flags().StringP("selector", "s", "", "CSS selector for file input")
	downloadCmd.Flags().StringP("output", "o", "", "Save downloaded file to path")
	snapCmd.Flags().BoolP("interactive", "i", true, "Filter interactive elements + headings (default true, use --interactive=false for all)")
	snapCmd.Flags().BoolP("compact", "c", true, "Compact output format (default true, use --compact=false for JSON)")
	snapCmd.Flags().Bool("full", false, "Full JSON output (shorthand for --interactive=false --json)")
	snapCmd.Flags().Bool("json", false, "JSON output, keeping the interactive filter (same as --compact=false)")
	snapCmd.Flags().Bool("text", false, "Text output format")
	snapCmd.Flags().BoolP("diff", "d", false, "Show diff from previous snapshot")
	snapCmd.Flags().StringP("selector", "s", "", "CSS selector to scope snapshot")
	snapCmd.Flags().String("max-tokens", "", "Maximum token budget")
	snapCmd.Flags().String("depth", "", "Tree depth limit")
	screenshotCmd.Flags().StringP("output", "o", "", "Save screenshot to file path")
	screenshotCmd.Flags().StringP("quality", "q", "", "JPEG quality (0-100)")
	screenshotCmd.Flags().StringP("selector", "s", "", "Element selector to capture (ref/CSS/XPath/text)")
	screenshotCmd.Flags().String("scale", "", "Rescale the output image (e.g. 0.5 = half size, 0.25 = quarter). Default 1.")
	screenshotCmd.Flags().Bool("annotate", false, "Overlay numbered ref boxes on interactive elements (or on --selector matches). Prints a [n] ref legend to stdout.")
	screenshotCmd.Flags().String("format", "", "Image format: 'jpeg' or 'png' (default: inferred from -o .png, otherwise jpeg)")
	screenshotCmd.Flags().Bool("beyond-viewport", false, "Capture the entire scrollable document, not just the visible viewport. Ignored when --selector is set.")
	// Back-compat: --css-1x was removed (replaced by --scale). Keep it as a
	// deprecated no-op so old scripts get a notice instead of a hard error.
	screenshotCmd.Flags().Bool("css-1x", false, "deprecated: use --scale")
	_ = screenshotCmd.Flags().MarkDeprecated("css-1x", "css-1x was removed; use --scale to rescale output")
	annotateCmd.Flags().StringP("selector", "s", "", "Scope the overlay to elements within this selector (ref/CSS/XPath/text)")
	annotateCmd.Flags().Bool("clear", false, "Remove the persistent annotation overlay instead of injecting it")
	captureCmd.Flags().StringP("output", "o", "", "Save the captured image to this local file path (default: capture-<ts>.jpg)")
	captureCmd.Flags().StringP("selector", "s", "", "Scope: clips screenshot and filters snapshot subtree to the same element")
	captureCmd.Flags().String("filter", "", "Snapshot filter: 'interactive' or 'all' (default: interactive)")
	captureCmd.Flags().String("format", "", "Image format: 'jpeg' or 'png' (default: inferred from -o .png, otherwise jpeg)")
	captureCmd.Flags().StringP("quality", "q", "", "JPEG quality (0-100)")
	captureCmd.Flags().String("depth", "", "Snapshot max depth (-1 for full)")
	captureCmd.Flags().String("wait", "", "Lifecycle wait: stable (default) | load | none")
	captureCmd.Flags().Bool("with-bounds", true, "Populate boundingBox per snapshot node (default true)")
	captureCmd.Flags().Bool("beyond-viewport", false, "Capture the entire scrollable document; coordinate space becomes 'document'")
	captureCmd.Flags().String("scale", "", "Rescale the output image (e.g. 0.5 = half size, 0.25 = quarter). Default 1.")
	captureCmd.Flags().Bool("require-pair", false, "Return 409 if navigation is observed during the capture window")
	captureCmd.Flags().Bool("json", false, "Output full JSON response instead of terse summary")
	pdfCmd.Flags().StringP("output", "o", "", "Save PDF to file path")
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
	findCmd.Flags().String("threshold", "", "Minimum similarity score (0-1)")
	findCmd.Flags().Bool("explain", false, "Show score breakdown")
	findCmd.Flags().Bool("ref-only", false, "Output just the element ref")
	extractCmd.Flags().String("schema", "", "JSON schema file to extract against, or - to read it from stdin")
	_ = extractCmd.MarkFlagRequired("schema")
	extractCmd.Flags().String("scope", "", "Confine every field to one element's subtree (ref, role:, text: or a plain query)")
	extractCmd.Flags().Int("max-items", 0, "Cap on items per array (server default 100)")
	extractCmd.Flags().Bool("fields", false, "After the data, print a field<TAB>ref<TAB>confidence table so a value can be acted on")
	extractCmd.Flags().Bool("explain", false, "Print the field table with score, source and reason columns")
	textCmd.Flags().Bool("raw", false, "Raw extraction mode (alias of --full)")
	textCmd.Flags().Bool("full", false, "Return the full page text (document.body.innerText, the API's mode=full/mode=raw) instead of the default Readability-filtered content")
	textCmd.Flags().Bool("markdown", false, "Return the page as Markdown (preserves headings, links and tables); mutually exclusive with --full/--raw")
	textCmd.Flags().StringP("output", "o", "", "Write the extracted text to this file and print a one-line confirmation instead of the body")
	textCmd.Flags().String("frame", "", "Extract text from a specific iframe by frameId. If unset, uses the tab's active frame scope (set via `pinchtab frame`) or the top-level document.")
	textCmd.Flags().StringP("selector", "s", "", "Element selector to extract text from (ref/CSS/XPath/text)")
	textCmd.Flags().Bool("json", false, "Output full JSON response instead of just text content")
	textCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		markdown, _ := cmd.Flags().GetBool("markdown")
		raw, _ := cmd.Flags().GetBool("raw")
		full, _ := cmd.Flags().GetBool("full")
		if markdown && (raw || full) {
			return fmt.Errorf("--markdown cannot be combined with --full or --raw; each selects a different extraction mode")
		}
		return nil
	}
	titleCmd.Flags().String("frame", "", "Read title from a specific iframe by frameId. If unset, uses the tab's active frame scope or top-level document.")
	titleCmd.Flags().Bool("json", false, "Output full JSON response instead of just title")
	urlCmd.Flags().String("frame", "", "Read URL from a specific iframe by frameId. If unset, uses the tab's active frame scope or top-level document.")
	urlCmd.Flags().Bool("json", false, "Output full JSON response instead of just URL")
	htmlCmd.Flags().String("frame", "", "Read HTML from a specific iframe by frameId. If unset, uses the tab's active frame scope or top-level document.")
	htmlCmd.Flags().StringP("selector", "s", "", "Element selector to extract HTML from (ref/CSS/XPath/text)")
	htmlCmd.Flags().String("max-chars", "", "Maximum number of HTML characters to return")
	htmlCmd.Flags().Bool("json", false, "Output full JSON response instead of just HTML")
	stylesCmd.Flags().String("frame", "", "Read computed styles from a specific iframe by frameId. If unset, uses the tab's active frame scope or top-level document.")
	stylesCmd.Flags().StringP("selector", "s", "", "Element selector to extract styles from (ref/CSS/XPath/text). If omitted, returns computed styles for the root element.")
	stylesCmd.Flags().String("prop", "", "Return only a single computed style property")
	stylesCmd.Flags().Bool("json", false, "Output full JSON response instead of just styles")
	valueCmd.Flags().Bool("json", false, "Output full JSON response instead of just value")
	attrCmd.Flags().Bool("json", false, "Output full JSON response instead of just attribute value")
	countCmd.Flags().Bool("json", false, "Output full JSON response instead of just count")
	boxCmd.Flags().Bool("json", false, "Output full JSON response instead of just bounding box")
	visibleCmd.Flags().Bool("json", false, "Output full JSON response instead of just visibility")
	enabledCmd.Flags().Bool("json", false, "Output full JSON response instead of just enabled state")
	checkedCmd.Flags().Bool("json", false, "Output full JSON response instead of just checked state")
	addTabFlag(snapCmd)
	addTabFlag(frameCmd)
	addTabFlag(screenshotCmd)
	addTabFlag(captureCmd)
	addTabFlag(annotateCmd)
	addTabFlag(pdfCmd)
	addTabFlag(findCmd)
	addTabFlag(extractCmd)
	addTabFlag(textCmd)
	addTabFlag(titleCmd)
	addTabFlag(urlCmd)
	addTabFlag(htmlCmd)
	addTabFlag(stylesCmd)
	addTabFlag(valueCmd)
	addTabFlag(attrCmd)
	addTabFlag(countCmd)
	addTabFlag(boxCmd)
	addTabFlag(visibleCmd)
	addTabFlag(enabledCmd)
	addTabFlag(checkedCmd)
	addTabFlag(evalCmd)
	addTabFlag(uploadCmd)
	addTabFlag(downloadCmd)
	addTabFlag(networkCmd)
	addTabFlag(waitCmd)
	evalCmd.Flags().Bool("await-promise", false, "Resolve a returned Promise before responding")
	addJSONFlag(waitCmd)
	addJSONFlag(findCmd)
	addJSONFlag(extractCmd)
	addJSONFlag(evalCmd)
	addJSONFlag(frameCmd)
	addJSONFlag(networkCmd)
	networkCmd.Flags().String("filter", "", "URL pattern filter")
	networkCmd.Flags().String("method", "", "HTTP method filter (GET, POST, etc)")
	networkCmd.Flags().String("status", "", "Status code range (e.g. 4xx, 5xx, 200)")
	networkCmd.Flags().String("type", "", "Resource type filter (xhr, fetch, document, etc)")
	networkCmd.Flags().String("limit", "", "Maximum entries to return")
	networkCmd.Flags().Bool("body", false, "Include response body (with requestId)")
	networkCmd.Flags().Bool("clear", false, "Clear captured network data")
	networkCmd.Flags().String("buffer-size", "", "Per-tab network buffer size (default 100)")
	networkCmd.Flags().Bool("stream", false, "Stream network entries in real-time (like tail -f)")
	waitCmd.Flags().String("text", "", "Wait for text on page")
	waitCmd.Flags().String("not-text", "", "Wait for text to disappear from page")
	waitCmd.Flags().String("url", "", "Wait for URL glob match")
	waitCmd.Flags().String("load", "", "Wait for load state (networkidle)")
	waitCmd.Flags().String("fn", "", "Wait for JS expression to be truthy")
	waitCmd.Flags().String("state", "", "Element state: visible (default) or hidden")
	waitCmd.Flags().Int("timeout-ms", 0, "Timeout in milliseconds (default 10000, max 30000)")
	// --timeout on wait is milliseconds, but the same bare flag is SECONDS on
	// nav/scrape — a 1000x footgun. Keep it for back-compat as a deprecated alias
	// (still ms); cobra prints its deprecation note pointing at --timeout-ms on use.
	waitCmd.Flags().Int("timeout", 0, "Deprecated: use --timeout-ms")
	_ = waitCmd.Flags().MarkDeprecated("timeout", "use --timeout-ms (this flag is milliseconds; bare --timeout means seconds on nav and scrape)")
}
