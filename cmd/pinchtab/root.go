package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/safelog"
	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "pinchtab",
	Short: "PinchTab - Browser control for AI agents",
	Long: `PinchTab provides a lightweight, API-driven way for AI agents to control
browsers, manage tabs, and perform interactive tasks.`,
	Example: `  pinchtab server
  pinchtab nav https://pinchtab.com`,
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()
		printAgentHints(loadLocalConfig())
	},
}

// versionCmd mirrors --version as a subcommand, since `pinchtab version` is a
// common first instinct and cobra only wires up the --version flag by default.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the PinchTab version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pinchtab %s\n", version)
	},
}

// maybeRunWizard checks if the security wizard should run and triggers it.
func maybeRunWizard() {
	fileCfg, configPath, err := config.LoadFileConfig()
	if err != nil || configPath == "" {
		return // No config file context — skip wizard
	}

	if !config.NeedsWizard(fileCfg) {
		return
	}

	isNew := config.IsFirstRun(fileCfg)
	runSecurityWizard(fileCfg, configPath, isNew)
}

func Execute() {
	safelog.InstallDefault()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(commandExitCode(err))
	}
}

type commandExitError struct {
	code int
	err  error
}

func newCommandExitError(code int, err error) error {
	return &commandExitError{code: code, err: err}
}

func (e *commandExitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return e.err.Error()
}

func (e *commandExitError) Unwrap() error {
	return e.err
}

func (e *commandExitError) ExitCode() int {
	return e.code
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded *commandExitError
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}

// serverURL is the global --server flag for CLI commands
var serverURL string
var cliAgentID string

func init() {
	config.SetConfigSchemaVersion(version)

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("pinchtab {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "PinchTab server URL (default: http://127.0.0.1:<server.port>)")
	rootCmd.PersistentFlags().StringVar(&cliAgentID, "agent-id", "", "Agent identifier recorded in activity logs")

	primaryGroup := &cobra.Group{ID: "primary", Title: "Primary Commands"}
	browserGroup := &cobra.Group{ID: "browser", Title: "Browser Control"}
	mgmtGroup := &cobra.Group{ID: "management", Title: "Profiles and Instances"}
	configGroup := &cobra.Group{ID: "config", Title: "Configuration & Setup"}

	rootCmd.AddGroup(primaryGroup, browserGroup, mgmtGroup, configGroup)

	rootCmd.AddCommand(versionCmd)

	cli.SetupUsage(rootCmd)
}
