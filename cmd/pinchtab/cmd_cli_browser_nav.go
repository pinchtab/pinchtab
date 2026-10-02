package main

import (
	"errors"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"strings"

	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

var quickCmd = &cobra.Command{
	Use:        "quick <url>",
	Short:      "Deprecated: use nav <url> --snap",
	Deprecated: "use 'pinchtab nav <url> --snap' instead",
	Args:       cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = urls.Normalize(args[0])
		runCLI(func(rt apiclient.Conn) {
			browseractions.Quick(rt, args)
		})
	},
}

var navCmd = &cobra.Command{
	Use:     "nav <url>",
	Aliases: []string{"goto", "navigate", "open"},
	Short:   "Navigate to URL",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := urls.Normalize(args[0])
		runCLIEnsuringServer("nav", func(rt apiclient.Conn) {
			tabID := browseractions.Navigate(rt, url, cmd)
			WriteTabStateFile(tabID)
		})
	},
}

var backCmd = &cobra.Command{
	Use:   "back",
	Short: "Go back in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.Back(rt, cmd)
		})
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "Go forward in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.Forward(rt, cmd)
		})
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload current page",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.Reload(rt, cmd)
		})
	},
}

var tabsCmd = &cobra.Command{
	Use:     "tab [id]",
	Aliases: []string{"tabs"},
	Short:   "List tabs, or focus a tab by ID",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			if len(args) == 0 {
				browseractions.TabList(rt, cmd, defaultTabState.target())
				return
			}
			tabID := browseractions.TabFocus(rt, args[0], cmd)
			WriteTabStateFile(tabID)
		})
	},
}

var closeCmd = newTabCloseCmd()
var tabCloseCmd = newTabCloseCmd()
var handoffCmd = newTabHandoffCmd()
var tabHandoffCmd = newTabHandoffCmd()
var resumeCmd = newTabResumeCmd()
var tabResumeCmd = newTabResumeCmd()
var handoffStatusCmd = newTabHandoffStatusCmd()
var tabHandoffStatusCmd = newTabHandoffStatusCmd()

func newTabCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "Close a tab by ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt apiclient.Conn) {
				browseractions.TabClose(rt, args[0], cmd)
				ClearTabStateFileIfCurrent(args[0])
			})
		},
	}
}

// errNoCurrentTab refuses locally instead of issuing /tabs//<verb>: an empty path
// segment 404s at the mux, which the CLI then misdiagnoses as an outdated server.
var errNoCurrentTab = errors.New("no current tab: pass a tab id, or run `pinchtab nav <url>` to open one")

func mustResolveTabArg(args []string) (string, error) {
	tabID := resolveTabArg(args)
	if strings.TrimSpace(tabID) == "" {
		return "", errNoCurrentTab
	}
	return tabID, nil
}

func newTabHandoffCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "handoff [id]",
		Short:         "Pause tab automation for human handoff",
		Long:          "Mark a tab as paused_handoff so action routes block until resumed or timeout expires. Defaults to the current tab from the state file.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reason, _ := cmd.Flags().GetString("reason")
			timeoutMS, _ := cmd.Flags().GetInt("timeout-ms")
			tabID, err := mustResolveTabArg(args)
			if err != nil {
				return err
			}
			runCLI(func(rt apiclient.Conn) {
				browseractions.TabHandoff(rt, tabID, reason, timeoutMS, cmd)
			})
			return nil
		},
	}
}

func newTabResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "resume [id]",
		Short:         "Resume a paused_handoff tab",
		Long:          "Resume automation on a paused tab. Defaults to the current tab from the state file.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, _ := cmd.Flags().GetString("status")
			tabID, err := mustResolveTabArg(args)
			if err != nil {
				return err
			}
			runCLI(func(rt apiclient.Conn) {
				browseractions.TabResume(rt, tabID, status, cmd)
			})
			return nil
		},
	}
}

func newTabHandoffStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "handoff-status [id]",
		Short:         "Show handoff status for a tab",
		Long:          "Show handoff status. Defaults to the current tab from the state file.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			tabID, err := mustResolveTabArg(args)
			if err != nil {
				return err
			}
			runCLI(func(rt apiclient.Conn) {
				browseractions.TabHandoffStatus(rt, tabID, cmd)
			})
			return nil
		},
	}
}

func configureBrowserNavFlags() {
	navCmd.Flags().Bool("new-tab", false, "Open in new tab")
	navCmd.Flags().Float64("timeout", 0, "Navigation timeout in seconds (max 120); overrides the 30s new-tab ceiling")
	navCmd.Flags().Bool("block-images", false, "Block image loading")
	navCmd.Flags().Bool("block-ads", false, "Block ads")
	addPostActionFlags(navCmd, "navigation", true)
	navCmd.Flags().Bool("dismiss-banners", false, "After landing, click any visible cookie/consent dismissal button or remove obvious overlay containers")
	addPostActionFlags(backCmd, "navigation", true)
	backCmd.Flags().Bool("dismiss-banners", false, "After landing, dismiss cookie/consent banners")
	addPostActionFlags(forwardCmd, "navigation", true)
	forwardCmd.Flags().Bool("dismiss-banners", false, "After landing, dismiss cookie/consent banners")
	addPostActionFlags(reloadCmd, "reload", true)
	reloadCmd.Flags().Bool("dismiss-banners", false, "After reload, dismiss cookie/consent banners")
	addTabFlag(navCmd)
	addTabFlag(backCmd)
	addTabFlag(forwardCmd)
	addTabFlag(reloadCmd)
	navCmd.Flags().Bool("print-tab-id", false, "Print only the tab ID on stdout; with --snap or --text the payload owns stdout and the tab ID goes to stderr (also triggered automatically when stdout is a pipe)")
	handoffCmd.Flags().String("reason", "", "Reason for human handoff (default: manual_handoff)")
	handoffCmd.Flags().Int("timeout-ms", 0, "Optional auto-resume timeout in milliseconds")
	tabHandoffCmd.Flags().String("reason", "", "Reason for human handoff (default: manual_handoff)")
	tabHandoffCmd.Flags().Int("timeout-ms", 0, "Optional auto-resume timeout in milliseconds")
	resumeCmd.Flags().String("status", "", "Optional resume status note (e.g. completed, failed)")
	tabResumeCmd.Flags().String("status", "", "Optional resume status note (e.g. completed, failed)")
	addJSONFlag(backCmd)
	addJSONFlag(forwardCmd)
	addJSONFlag(reloadCmd)
	addJSONFlag(navCmd)
	addJSONFlag(tabsCmd)
	addJSONFlag(closeCmd)
	addJSONFlag(tabCloseCmd)
	addJSONFlag(handoffCmd)
	addJSONFlag(tabHandoffCmd)
	addJSONFlag(resumeCmd)
	addJSONFlag(tabResumeCmd)
	addJSONFlag(handoffStatusCmd)
	addJSONFlag(tabHandoffStatusCmd)
}
