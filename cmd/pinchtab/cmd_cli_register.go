package main

import (
	"net"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/spf13/cobra"
)

func init() {
	registerBrowserCommands()
	registerManagementCommands()
}

// browserRootCommands is the canonical ordered list of top-level browser
// shorthand commands, registered on the root and assigned the "browser" group.
func browserRootCommands() []*cobra.Command {
	return []*cobra.Command{
		quickCmd, navCmd, backCmd, forwardCmd, reloadCmd, snapCmd, frameCmd, clickCmd,
		dblclickCmd, dragCmd, typeCmd, screenshotCmd, annotateCmd, captureCmd, tabsCmd, pressCmd, fillCmd,
		hoverCmd, mouseCmd, focusCmd, scrollCmd, evalCmd, pdfCmd, textCmd, titleCmd, urlCmd,
		htmlCmd, stylesCmd, valueCmd, attrCmd, countCmd, boxCmd, visibleCmd, enabledCmd, checkedCmd,
		downloadCmd, uploadCmd, findCmd, selectCmd, checkCmd, uncheckCmd, networkCmd, waitCmd,
		keyboardCmd, keydownCmd, keyupCmd, scrollintoviewCmd, dialogCmd, consoleCmd, errorsCmd,
		clipboardCmd, cacheCmd, cookiesCmd, setCmd, storageCmd, stateCmd, closeCmd, handoffCmd,
		resumeCmd, handoffStatusCmd, recordCmd,
	}
}

func registerBrowserCommands() {
	rootCmds := browserRootCommands()
	setCommandGroup("browser", rootCmds...)
	// Tab subcommands live under tabsCmd but need the browser group too.
	setCommandGroup("browser", tabCloseCmd, tabHandoffCmd, tabResumeCmd, tabHandoffStatusCmd)

	// tabsCmd needs the same group registered so cobra accepts grouped tab subcommands.
	tabsCmd.AddGroup(&cobra.Group{ID: "browser", Title: "Browser"})
	tabsCmd.AddCommand(tabCloseCmd, tabHandoffCmd, tabResumeCmd, tabHandoffStatusCmd)
	clipboardCmd.AddCommand(clipboardReadCmd, clipboardWriteCmd, clipboardCopyCmd, clipboardPasteCmd)
	keyboardCmd.AddCommand(keyboardTypeCmd, keyboardInsertTextCmd)
	dialogCmd.AddCommand(dialogAcceptCmd, dialogDismissCmd)
	mouseCmd.AddCommand(mouseMoveCmd, mouseDownCmd, mouseUpCmd, mouseWheelCmd)
	networkCmd.AddCommand(networkRouteCmd, networkUnrouteCmd)
	recordCmd.AddCommand(recordStartCmd, recordStopCmd, recordStatusCmd)

	configureBrowserFlags()

	addRootCommands(rootCmds...)
}

func registerManagementCommands() {
	setCommandGroup("management", instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)

	instanceCmd.AddCommand(startInstanceCmd, instanceNavigateCmd, instanceStopCmd, instanceRestartCmd, instanceLogsCmd)
	activityCmd.AddCommand(activityTabCmd)

	configureManagementFlags()

	addRootCommands(instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)
}

func configureBrowserFlags() {
	uploadCmd.Flags().StringP("selector", "s", "", "CSS selector for file input")
	downloadCmd.Flags().StringP("output", "o", "", "Save downloaded file to path")

	addPointerActionFlags(clickCmd, "click")
	clickCmd.Flags().Bool("wait-nav", false, "Wait for navigation after click")
	clickCmd.Flags().Bool("dismiss-banners", false, "Dismiss cookie/consent banners after a wait-nav click (no-op without --wait-nav)")
	addPostActionFlags(clickCmd, "action", true)
	clickCmd.Flags().String("dialog-action", "", "Auto-handle a JS dialog opened by the click: accept | dismiss")
	clickCmd.Flags().String("dialog-text", "", "Prompt response text (with --dialog-action accept on prompt())")
	clickCmd.Flags().String("mode", "", "Click delivery mode override: dom | dispatch")

	addPointerActionFlags(dblclickCmd, "dblclick")

	addPointerActionFlags(hoverCmd, "hover")

	addPointerActionFlags(mouseMoveCmd, bridge.ActionMouseMove)

	addPointerActionFlags(mouseDownCmd, bridge.ActionMouseDown)
	mouseDownCmd.Flags().String("button", "left", "Mouse button: left, right, middle")

	addPointerActionFlags(mouseUpCmd, bridge.ActionMouseUp)
	mouseUpCmd.Flags().String("button", "left", "Mouse button: left, right, middle")

	addPointerActionFlags(mouseWheelCmd, bridge.ActionMouseWheel)

	typeCmd.Flags().Bool("humanize", false, "Use humanized per-character keypress timing (overrides instance config)")
	addPostActionFlags(pressCmd, "key press", true)
	mouseWheelCmd.Flags().Int("dx", 0, "Wheel delta X")
	mouseWheelCmd.Flags().Int("dy", 0, "Wheel delta Y")

	dragCmd.Flags().String("button", "left", "Mouse button: left, right, middle")
	dragCmd.Flags().Int("drag-x", 0, "Horizontal pixel offset for single-step drag action")
	dragCmd.Flags().Int("drag-y", 0, "Vertical pixel offset for single-step drag action")

	focusCmd.Flags().String("css", "", "CSS selector instead of ref")

	snapCmd.Flags().BoolP("interactive", "i", true, "Filter interactive elements + headings (default true, use --interactive=false for all)")
	snapCmd.Flags().BoolP("compact", "c", true, "Compact output format (default true, use --compact=false for JSON)")
	snapCmd.Flags().Bool("full", false, "Full JSON output (shorthand for --interactive=false --compact=false)")
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
	screenshotCmd.Flags().String("format", "", "Image format: 'jpeg' (default) or 'png'")
	screenshotCmd.Flags().Bool("beyond-viewport", false, "Capture the entire scrollable document, not just the visible viewport. Ignored when --selector is set.")
	// Back-compat: --css-1x was removed (replaced by --scale). Keep it as a
	// deprecated no-op so old scripts get a notice instead of a hard error.
	screenshotCmd.Flags().Bool("css-1x", false, "deprecated: use --scale")
	_ = screenshotCmd.Flags().MarkDeprecated("css-1x", "css-1x was removed; use --scale to rescale output")

	annotateCmd.Flags().StringP("selector", "s", "", "Scope the overlay to elements within this selector (ref/CSS/XPath/text)")
	annotateCmd.Flags().String("tab", "", "Tab ID")
	annotateCmd.Flags().Bool("clear", false, "Remove the persistent annotation overlay instead of injecting it")

	captureCmd.Flags().StringP("output", "o", "", "Save the captured image to this local file path (default: capture-<ts>.jpg)")
	captureCmd.Flags().StringP("selector", "s", "", "Scope: clips screenshot and filters snapshot subtree to the same element")
	captureCmd.Flags().String("filter", "", "Snapshot filter: 'interactive' or 'all' (default: interactive)")
	captureCmd.Flags().String("format", "", "Image format: 'jpeg' (default) or 'png'")
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

	recordStartCmd.Flags().Int("fps", 5, "Frames per second (1-30)")
	recordStartCmd.Flags().Int("quality", 80, "JPEG capture quality (1-100)")
	recordStartCmd.Flags().Float64("scale", 1.0, "Resolution scale multiplier")
	addTabFlag(recordStartCmd)

	findCmd.Flags().String("threshold", "", "Minimum similarity score (0-1)")
	findCmd.Flags().Bool("explain", false, "Show score breakdown")
	findCmd.Flags().Bool("ref-only", false, "Output just the element ref")

	textCmd.Flags().Bool("raw", false, "Raw extraction mode (alias of --full)")
	textCmd.Flags().Bool("full", false, "Return the full page text (document.body.innerText) instead of the default Readability-filtered content")
	textCmd.Flags().String("frame", "", "Extract text from a specific iframe by frameId. If unset, uses the tab's active frame scope (set via `pinchtab frame`) or the top-level document.")
	textCmd.Flags().StringP("selector", "s", "", "Element selector to extract text from (ref/CSS/XPath/text)")
	textCmd.Flags().Bool("json", false, "Output full JSON response instead of just text content")
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

	navCmd.Flags().Bool("new-tab", false, "Open in new tab")
	navCmd.Flags().Bool("block-images", false, "Block image loading")
	navCmd.Flags().Bool("block-ads", false, "Block ads")
	addPostActionFlags(navCmd, "navigation", false)
	navCmd.Flags().Bool("dismiss-banners", false, "After landing, click any visible cookie/consent dismissal button or remove obvious overlay containers")

	addPostActionFlags(backCmd, "navigation", true)
	backCmd.Flags().Bool("dismiss-banners", false, "After landing, dismiss cookie/consent banners")
	addPostActionFlags(forwardCmd, "navigation", true)
	forwardCmd.Flags().Bool("dismiss-banners", false, "After landing, dismiss cookie/consent banners")
	addPostActionFlags(reloadCmd, "reload", true)
	reloadCmd.Flags().Bool("dismiss-banners", false, "After reload, dismiss cookie/consent banners")
	addPostActionFlags(fillCmd, "fill", true)
	addPostActionFlags(selectCmd, "select", true)
	addPostActionFlags(scrollCmd, "scroll", false)

	addTabFlag(
		navCmd,
		backCmd,
		forwardCmd,
		reloadCmd,
		snapCmd,
		frameCmd,
		screenshotCmd,
		pdfCmd,
		findCmd,
		textCmd,
		titleCmd,
		urlCmd,
		htmlCmd,
		stylesCmd,
		valueCmd,
		attrCmd,
		countCmd,
		boxCmd,
		visibleCmd,
		enabledCmd,
		checkedCmd,
		clickCmd,
		dblclickCmd,
		hoverCmd,
		mouseMoveCmd,
		mouseDownCmd,
		mouseUpCmd,
		mouseWheelCmd,
		dragCmd,
		focusCmd,
		typeCmd,
		pressCmd,
		fillCmd,
		scrollCmd,
		selectCmd,
		evalCmd,
		checkCmd,
		uncheckCmd,
		keyboardTypeCmd,
		keyboardInsertTextCmd,
		keydownCmd,
		keyupCmd,
		scrollintoviewCmd,
		networkCmd,
		waitCmd,
		dialogAcceptCmd,
		dialogDismissCmd,
		setViewportCmd,
		setGeoCmd,
		setOfflineCmd,
		setHeadersCmd,
		setCredentialsCmd,
		setMediaCmd,
	)

	evalCmd.Flags().Bool("await-promise", false, "Resolve a returned Promise before responding")
	navCmd.Flags().Bool("print-tab-id", false, "Print only the tab ID on stdout (also triggered automatically when stdout is a pipe)")
	for _, cmd := range []*cobra.Command{handoffCmd, tabHandoffCmd} {
		cmd.Flags().String("reason", "", "Reason for human handoff (default: manual_handoff)")
		cmd.Flags().Int("timeout-ms", 0, "Optional auto-resume timeout in milliseconds")
	}
	for _, cmd := range []*cobra.Command{resumeCmd, tabResumeCmd} {
		cmd.Flags().String("status", "", "Optional resume status note (e.g. completed, failed)")
	}

	addJSONFlag(
		clickCmd,
		dblclickCmd,
		hoverCmd,
		mouseMoveCmd,
		mouseDownCmd,
		mouseUpCmd,
		mouseWheelCmd,
		dragCmd,
		focusCmd,
		typeCmd,
		pressCmd,
		fillCmd,
		scrollCmd,
		selectCmd,
		checkCmd,
		uncheckCmd,
		scrollintoviewCmd,
		waitCmd,
		dialogAcceptCmd,
		dialogDismissCmd,
		backCmd,
		forwardCmd,
		reloadCmd,
		navCmd,
		findCmd,
		evalCmd,
		tabsCmd,
		closeCmd,
		tabCloseCmd,
		handoffCmd,
		tabHandoffCmd,
		resumeCmd,
		tabResumeCmd,
		handoffStatusCmd,
		tabHandoffStatusCmd,
		healthCmd,
		cacheClearCmd,
		cacheStatusCmd,
		cookiesClearCmd,
		frameCmd,
		networkCmd,
		setViewportCmd,
		setGeoCmd,
		setOfflineCmd,
		setHeadersCmd,
		setCredentialsCmd,
		setMediaCmd,
	)

	scrollintoviewCmd.Flags().String("css", "", "CSS selector instead of ref")

	networkRouteCmd.Flags().Bool("abort", false, "Block matching requests instead of letting them through")
	networkRouteCmd.Flags().String("body", "", "Fulfill matching requests with this JSON body (mutually exclusive with --abort)")
	networkRouteCmd.Flags().String("resource-type", "", "Limit to a CDP resource category (e.g. script, image, xhr, fetch)")
	networkRouteCmd.Flags().String("content-type", "", "(With --body) Response Content-Type (default application/json)")
	networkRouteCmd.Flags().Int("status", 0, "(With --body) Response status code (default 200)")
	networkRouteCmd.Flags().String("method", "", "Limit to an HTTP method (GET, POST, ...). Fulfill rules without --method skip OPTIONS preflights to avoid breaking CORS.")
	addTabFlag(networkRouteCmd, networkUnrouteCmd)
	addJSONFlag(networkRouteCmd, networkUnrouteCmd)

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
	waitCmd.Flags().Int("timeout", 0, "Timeout in milliseconds (default 10000, max 30000)")

	consoleCmd.Flags().Bool("clear", false, "Clear console logs")
	consoleCmd.Flags().String("limit", "", "Maximum entries to return")
	errorsCmd.Flags().Bool("clear", false, "Clear error logs")
	errorsCmd.Flags().String("limit", "", "Maximum entries to return")

	addTabFlag(consoleCmd, errorsCmd)
}

func configureManagementFlags() {
	startInstanceCmd.Flags().String("profile", "", "Profile to use")
	startInstanceCmd.Flags().String("mode", "", "Instance mode")
	startInstanceCmd.Flags().String("port", "", "Port number")
	startInstanceCmd.Flags().StringArray("extension", nil, "Load browser extension (repeatable)")
	startInstanceCmd.Flags().StringArray("allow-domain", nil, "Add an instance-scoped IDPI allowed domain (repeatable)")
	startInstanceCmd.Flags().String("browser", "", "Named browser target to use (e.g. chrome, cloak)")
	startInstanceCmd.Flags().StringArray("browser-fallback", nil, "Named browser target to fall back to if the primary fails (repeatable; overrides config browser.fallbackOrder)")

	activityCmd.PersistentFlags().Int("limit", 200, "Maximum number of events to return")
	activityCmd.PersistentFlags().Int("age-sec", 0, "Only include events from the last N seconds")

	instancesCmd.Flags().Bool("json", false, "Output full JSON response instead of terse status")
	profilesCmd.Flags().Bool("json", false, "Output full JSON response instead of terse status")
}

func setCommandGroup(groupID string, cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.GroupID = groupID
	}
}

func addRootCommands(cmds ...*cobra.Command) {
	rootCmd.AddCommand(cmds...)
}

// addTabFlag wires a --tab flag onto the given commands. Anonymous CLI calls
// default its value from the state file written by `nav`, which lets local
// single-agent workflows avoid threading `--tab "$TAB"` through every command:
//
//	pinchtab nav http://example.com   # writes tab ID to state file
//	pinchtab snap -i -c               # auto-reads from state file
//
// Explicit --tab still wins (cobra flag precedence). Identified callers
// (PINCHTAB_SESSION, --agent-id, or PINCHTAB_AGENT_ID) leave --tab unset so the
// server-side scoped current-tab store is authoritative. If no state file is
// set, the server picks the active tab as before.
func addTabFlag(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.Flags().String("tab", "", "Tab ID")
		existingPreRun := cmd.PreRun
		cmd.PreRun = func(cmd *cobra.Command, args []string) {
			defaultTabFlagFromState(cmd)
			if existingPreRun != nil {
				existingPreRun(cmd, args)
			}
		}
	}
}

func portIsListening(baseURL string) bool {
	host := strings.TrimPrefix(baseURL, "http://")
	host = strings.TrimPrefix(host, "https://")
	conn, err := net.DialTimeout("tcp", host, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func addJSONFlag(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.Flags().Bool("json", false, "Output full JSON response instead of terse status")
	}
}

func addPointFlags(cmd *cobra.Command, action string) {
	cmd.Flags().Float64("x", 0, "X coordinate for "+action)
	cmd.Flags().Float64("y", 0, "Y coordinate for "+action)
}

// addPointerActionFlags adds the css-selector, point-coordinate, and humanize
// flags shared by pointer actions. Callers add any action-specific flags after.
func addPointerActionFlags(cmd *cobra.Command, action string) {
	cmd.Flags().String("css", "", "CSS selector instead of ref")
	addPointFlags(cmd, action)
	cmd.Flags().Bool("humanize", false, "Use humanized bezier+jitter input path (overrides instance config)")
}

// addPostActionFlags registers the standard post-action output flags (snap,
// snap-diff, and optionally text) with descriptions interpolated from verb, so
// the bundle is defined once instead of repeated across browser commands.
func addPostActionFlags(cmd *cobra.Command, verb string, withText bool) {
	cmd.Flags().Bool("snap", false, "Output interactive snapshot after "+verb)
	cmd.Flags().Bool("snap-diff", false, "Output snapshot diff after "+verb+" (changes only)")
	if withText {
		cmd.Flags().Bool("text", false, "Output page text after "+verb+" (for verification)")
	}
}
