package main

import "github.com/spf13/cobra"

func init() {
	registerBrowserCommands()
	registerManagementCommands()
}

func registerBrowserCommands() {
	setCommandGroup("browser",
		quickCmd,
		navCmd,
		backCmd,
		forwardCmd,
		reloadCmd,
		snapCmd,
		clickCmd,
		dblclickCmd,
		typeCmd,
		screenshotCmd,
		tabsCmd,
		pressCmd,
		fillCmd,
		hoverCmd,
		focusCmd,
		scrollCmd,
		evalCmd,
		pdfCmd,
		textCmd,
		downloadCmd,
		uploadCmd,
		findCmd,
		selectCmd,
		checkCmd,
		uncheckCmd,
		networkCmd,
		waitCmd,
		keyboardCmd,
		keydownCmd,
		keyupCmd,
		scrollintoviewCmd,
		dialogCmd,
		consoleCmd,
		errorsCmd,
	)

	tabsCmd.AddCommand(tabNewCmd, tabCloseCmd)
	keyboardCmd.AddCommand(keyboardTypeCmd, keyboardInsertTextCmd)
	dialogCmd.AddCommand(dialogAcceptCmd, dialogDismissCmd)

	configureBrowserFlags()

	addRootCommands(
		quickCmd,
		navCmd,
		backCmd,
		forwardCmd,
		reloadCmd,
		snapCmd,
		clickCmd,
		dblclickCmd,
		typeCmd,
		screenshotCmd,
		tabsCmd,
		pressCmd,
		fillCmd,
		hoverCmd,
		focusCmd,
		scrollCmd,
		evalCmd,
		pdfCmd,
		textCmd,
		downloadCmd,
		uploadCmd,
		findCmd,
		selectCmd,
		checkCmd,
		uncheckCmd,
		networkCmd,
		waitCmd,
		keyboardCmd,
		keydownCmd,
		keyupCmd,
		scrollintoviewCmd,
		dialogCmd,
		consoleCmd,
		errorsCmd,
	)
}

func registerManagementCommands() {
	setCommandGroup("management", instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)

	instanceCmd.AddCommand(startInstanceCmd, instanceNavigateCmd, instanceStopCmd, instanceLogsCmd)
	activityCmd.AddCommand(activityTabCmd)

	configureManagementFlags()

	addRootCommands(instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)
}

func configureBrowserFlags() {
	uploadCmd.Flags().StringP("selector", "s", "", "CSS selector for file input")
	downloadCmd.Flags().StringP("output", "o", "", "Save downloaded file to path")

	clickCmd.Flags().String("css", "", "CSS selector instead of ref")
	addPointFlags(clickCmd, "click")
	clickCmd.Flags().Bool("wait-nav", false, "Wait for navigation after click")

	dblclickCmd.Flags().String("css", "", "CSS selector instead of ref")
	addPointFlags(dblclickCmd, "dblclick")

	hoverCmd.Flags().String("css", "", "CSS selector instead of ref")
	addPointFlags(hoverCmd, "hover")

	focusCmd.Flags().String("css", "", "CSS selector instead of ref")

	snapCmd.Flags().BoolP("interactive", "i", false, "Filter interactive elements only")
	snapCmd.Flags().BoolP("compact", "c", false, "Compact output format")
	snapCmd.Flags().Bool("text", false, "Text output format")
	snapCmd.Flags().BoolP("diff", "d", false, "Show diff from previous snapshot")
	snapCmd.Flags().StringP("selector", "s", "", "CSS selector to scope snapshot")
	snapCmd.Flags().String("max-tokens", "", "Maximum token budget")
	snapCmd.Flags().String("depth", "", "Tree depth limit")

	screenshotCmd.Flags().StringP("output", "o", "", "Save screenshot to file path")
	screenshotCmd.Flags().StringP("quality", "q", "", "JPEG quality (0-100)")

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

	textCmd.Flags().Bool("raw", false, "Raw extraction mode")

	navCmd.Flags().Bool("new-tab", false, "Open in new tab")
	navCmd.Flags().Bool("block-images", false, "Block image loading")
	navCmd.Flags().Bool("block-ads", false, "Block ads")

	addTabFlag(
		navCmd,
		backCmd,
		forwardCmd,
		reloadCmd,
		snapCmd,
		screenshotCmd,
		pdfCmd,
		findCmd,
		textCmd,
		clickCmd,
		dblclickCmd,
		hoverCmd,
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
	)

	scrollintoviewCmd.Flags().String("css", "", "CSS selector instead of ref")

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

	activityCmd.PersistentFlags().Int("limit", 200, "Maximum number of events to return")
	activityCmd.PersistentFlags().Int("age-sec", 0, "Only include events from the last N seconds")
}

func setCommandGroup(groupID string, cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.GroupID = groupID
	}
}

func addRootCommands(cmds ...*cobra.Command) {
	rootCmd.AddCommand(cmds...)
}

func addTabFlag(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.Flags().String("tab", "", "Tab ID")
	}
}

func addPointFlags(cmd *cobra.Command, action string) {
	cmd.Flags().Float64("x", 0, "X coordinate for "+action)
	cmd.Flags().Float64("y", 0, "Y coordinate for "+action)
}
