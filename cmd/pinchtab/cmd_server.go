package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/pinchtab/pinchtab/internal/safelog"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		maybeRunWizard()

		cfg, loadDiags := loadConfigDeferringDiagnostics()
		verbose, _ := cmd.Flags().GetBool("verbose")
		cfg.VerboseBanner = verbose
		logLevel, _ := cmd.Flags().GetString("log-level")
		resolveLogLevel(cfg, logLevel, verbose)
		config.EmitLoadDiagnostics(loadDiags)

		backgroundMarker, _ := cmd.Flags().GetString(backgroundChildFlagName)
		cfg.BackgroundMarker = backgroundMarker

		bind, _ := cmd.Flags().GetString("bind")
		port, _ := cmd.Flags().GetString("port")
		addressChanged := applyServerAddressFlags(cfg, bind, port)

		yolo, _ := cmd.Flags().GetBool("yolo")
		if yolo {
			fc, _, err := config.LoadFileConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("--yolo: load config: %v", err)))
				os.Exit(1)
			}
			if err := workflow.BuildGuardsDownConfig(fc); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("--yolo: %v", err)))
				os.Exit(1)
			}
			config.ApplyFileConfigToRuntime(cfg, fc)
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, "YOLO mode: guards down for this run only (config file unchanged)"))
		}

		headed, _ := cmd.Flags().GetBool("headed")
		if headed {
			cfg.Headless = false
			cfg.HeadlessSet = true
		}
		exts, _ := cmd.Flags().GetStringArray("extension")
		if len(exts) > 0 {
			cfg.ExtensionPaths = append(cfg.ExtensionPaths, exts...)
		}

		browserName, _ := cmd.Flags().GetString("browser")
		if browserName != "" {
			browser, err := config.ParseBrowser(browserName, cfg.BrowsersAvailable)
			if err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
				os.Exit(1)
			}
			cfg.DefaultBrowser = browser
		}

		if background, _ := cmd.Flags().GetBool("background"); background {
			if err := runServerBackground(cfg, serverBackgroundOptions{
				Yolo:       yolo,
				Headed:     headed,
				Verbose:    verbose,
				LogLevel:   logLevel,
				Extensions: append([]string(nil), exts...),
				Browser:    browserName,
				Bind:       bind,
				Port:       port,
			}, addressChanged); err != nil {
				fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
				os.Exit(1)
			}
			return
		}
		server.RunDashboard(cfg, version)
	},
}

func applyServerAddressFlags(cfg *config.RuntimeConfig, bind, port string) bool {
	configuredBind, configuredPort := cfg.Bind, cfg.Port
	if v := strings.TrimSpace(bind); v != "" {
		cfg.Bind = v
	}
	if v := strings.TrimSpace(port); v != "" {
		cfg.Port = v
	}
	return cfg.Bind != configuredBind || cfg.Port != configuredPort
}

// resolveLogLevel settles a run's threshold from the only inputs that carry it, in
// precedence order: --log-level, then server.logLevel from the config file, then
// -v, then the default. It is the one place that decides that precedence, and no
// caller assigns cfg.LogLevel itself — an unconditional assignment erased
// server.logLevel on every flagless run, which is every daemon-installed and
// auto-started server.
//
// The long-running runtimes call it: `server` and `bridge`. The short-lived
// commands do not, and do not need to. cmd_server_ensure.go and
// cmd_server_background.go log about spawning a server rather than being one, so
// their warnings and errors are recorded at the default level and their two
// slog.Debug lines are deliberately not reachable — a CLI command that
// auto-starts a server has no --log-level of its own to honour, and the server it
// starts resolves its own. Give either of them a level flag and it must route
// through here.
func resolveLogLevel(cfg *config.RuntimeConfig, logLevel string, verbose bool) {
	notice := verboseLevelNotice(logLevel, cfg.LogLevel, verbose)
	if v := strings.TrimSpace(logLevel); v != "" {
		cfg.LogLevel = v
	}
	applyLogLevel(cfg, verbose)
	if notice != "" {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, notice))
	}
}

// verboseLevelNotice explains the one case where -v looks like a no-op: it always adds
// the banner, but it only raises the level when neither --log-level nor server.logLevel
// has set one. It reads both sources BEFORE resolveLogLevel folds the flag into
// cfg.LogLevel, which is the only point at which they are still distinguishable.
func verboseLevelNotice(flagLevel, configLevel string, verbose bool) string {
	if !verbose {
		return ""
	}
	level, source := strings.TrimSpace(flagLevel), "--log-level"
	if level == "" {
		level, source = strings.TrimSpace(configLevel), "server.logLevel in the config file"
	}
	if level == "" {
		return ""
	}
	parsed, err := safelog.ParseLevel(level)
	if err != nil || parsed == slog.LevelDebug {
		return ""
	}
	return fmt.Sprintf("-v: log level stays %s, set by %s; pass --log-level debug to raise it", level, source)
}

func applyLogLevel(cfg *config.RuntimeConfig, verbose bool) {
	if strings.TrimSpace(cfg.LogLevel) != "" {
		level, err := safelog.ParseLevel(cfg.LogLevel)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, "pinchtab: "+err.Error()))
			os.Exit(1)
		}
		safelog.SetLevel(level)
		return
	}
	if verbose {
		safelog.SetLevel(slog.LevelDebug)
		return
	}
	safelog.SetLevel(safelog.DefaultLevel)
}

func init() {
	serverCmd.GroupID = "primary"
	serverCmd.Flags().String("bind", "", "Bind address for the HTTP server (overrides config server.bind)")
	serverCmd.Flags().String("port", "", "Port for the HTTP server (overrides config server.port)")
	serverCmd.Flags().StringArrayP("extension", "e", nil, "Load browser extension (repeatable)")
	serverCmd.Flags().BoolP("headed", "H", false, "Start default instance in headed mode")
	serverCmd.Flags().BoolP("yolo", "y", false, "Apply guards down preset (enables evaluate, macro, download, cookies)")
	serverCmd.Flags().BoolP("verbose", "v", false, "Show the full startup banner and log at debug level")
	serverCmd.Flags().String("log-level", "", "Minimum log level: debug, info (default), warn or error")
	serverCmd.Flags().String("browser", "", "Browser to use: chrome, cloak, or ghost-chrome (overrides config)")
	serverCmd.Flags().BoolP("background", "b", false, "Spawn the server detached and return JSON with pid/url/token")
	serverCmd.Flags().String(backgroundChildFlagName, "", "Internal marker for background server ownership")
	_ = serverCmd.Flags().MarkHidden(backgroundChildFlagName)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverRestartCmd)
	rootCmd.AddCommand(serverCmd)
}
