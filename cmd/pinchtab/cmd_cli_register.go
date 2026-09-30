package main

import (
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
		downloadCmd, uploadCmd, findCmd, extractCmd, selectCmd, checkCmd, uncheckCmd, networkCmd, waitCmd,
		keyboardCmd, keydownCmd, keyupCmd, scrollintoviewCmd, dialogCmd, consoleCmd, errorsCmd,
		clipboardCmd, cacheCmd, cookiesCmd, setCmd, storageCmd, stateCmd, closeCmd, handoffCmd,
		resumeCmd, handoffStatusCmd, recordCmd, auditCmd, compareCmd, scrapeCmd, a11yCmd, memoryCmd,
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
	networkCmd.AddCommand(networkRouteCmd, networkUnrouteCmd, networkRulesCmd)
	recordCmd.AddCommand(recordStartCmd, recordStopCmd, recordStatusCmd)
	a11yCmd.AddCommand(a11yAuditCmd)
	memoryCmd.AddCommand(memorySnapshotCmd, memorySummaryCmd, memoryCompareCmd)

	configureBrowserInspectFlags()
	configureBrowserActionsFlags()
	configureBrowserRecordFlags()
	configureBrowserNavFlags()
	configureEmulationFlags()
	configureCacheFlags()
	configureCookiesFlags()
	configureNetworkRouteFlags()
	configureConsoleFlags()
	configureAuditFlags()
	configureScrapeFlags()
	configureCompareFlags()
	configureA11yFlags()
	configureMemoryFlags()

	addRootCommands(rootCmds...)
}

func registerManagementCommands() {
	setCommandGroup("management", instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)

	instanceCmd.AddCommand(instanceListCmd, startInstanceCmd, instanceNavigateCmd, instanceStopCmd, instanceRestartCmd, instanceLogsCmd)
	activityCmd.AddCommand(activityTabCmd)
	profilesCmd.AddCommand(profilesCreateCmd, profilesPruneCmd)

	configureManagementFlags()

	addRootCommands(instancesCmd, healthCmd, profilesCmd, activityCmd, instanceCmd)
}

func setCommandGroup(groupID string, cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.GroupID = groupID
	}
}

func addRootCommands(cmds ...*cobra.Command) {
	rootCmd.AddCommand(cmds...)
}
