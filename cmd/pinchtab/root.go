package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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

type healthSnapshot struct {
	Status   string `json:"status"`
	Mode     string `json:"mode"`
	Version  string `json:"version"`
	Security *struct {
		Level                     string   `json:"level"`
		AllowedDomains            []string `json:"allowedDomains"`
		IDPIEnabled               bool     `json:"idpiEnabled"`
		EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
		GuardsDown                bool     `json:"guardsDown"`
	} `json:"security"`
}

type healthSnapshotState string

const (
	healthSnapshotStopped   healthSnapshotState = "stopped"
	healthSnapshotRunning   healthSnapshotState = "running"
	healthSnapshotProtected healthSnapshotState = "protected listener"
	healthSnapshotUnhealthy healthSnapshotState = "unhealthy"
	healthSnapshotInvalid   healthSnapshotState = "invalid health response"
)

func formatAllowedDomains(domains []string) string {
	if len(domains) == 0 {
		return "all"
	}
	for _, d := range domains {
		if strings.TrimSpace(d) == "*" {
			return "all"
		}
	}
	return strings.Join(domains, ", ")
}

func fetchHealthSnapshot(port string) (*healthSnapshot, healthSnapshotState) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost:%s/health", port), nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, healthSnapshotStopped
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, healthSnapshotProtected
	default:
		return nil, healthSnapshotUnhealthy
	}
	var snap healthSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, healthSnapshotInvalid
	}
	if snap.Status != "ok" || snap.Mode != "dashboard" || strings.TrimSpace(snap.Version) == "" {
		return nil, healthSnapshotInvalid
	}
	return &snap, healthSnapshotRunning
}

func printAgentHints(cfg *config.RuntimeConfig) {
	snap, healthState := fetchHealthSnapshot(cfg.Port)
	running := healthState == healthSnapshotRunning
	out := os.Stdout

	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "PinchTab")+" "+cli.StyleStdout(cli.MutedStyle, version))
	_, _ = fmt.Fprintln(out)

	if running {
		serverStatus := "running"
		serverStyle := cli.SuccessStyle
		if snap != nil && snap.Security != nil && snap.Security.GuardsDown {
			serverStatus = "running (YOLO — guards down for this run)"
			serverStyle = cli.WarningStyle
		}
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "server", cli.StyleStdout(serverStyle, serverStatus))
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "listen", cli.StyleStdout(cli.ValueStyle, cfg.ListenAddr()))
		if snap != nil && snap.Security != nil {
			if eps := snap.Security.EnabledSensitiveEndpoints; len(eps) > 0 {
				_, _ = fmt.Fprintf(out, "  %-20s %s\n", "sensitive", cli.StyleStdout(cli.WarningStyle, strings.Join(eps, ", ")))
			}
		}
	} else {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "server", cli.StyleStdout(cli.WarningStyle, string(healthState)))
	}

	var domains []string
	idpiEnabled := cfg.IDPI.Enabled
	if running && snap != nil && snap.Security != nil {
		domains = snap.Security.AllowedDomains
		idpiEnabled = snap.Security.IDPIEnabled
	} else {
		domains = cfg.AllowedDomains
	}
	formatted := formatAllowedDomains(domains)
	domStyle := cli.ValueStyle
	if formatted == "all" {
		domStyle = cli.WarningStyle
	}
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "allowedDomains", cli.StyleStdout(domStyle, formatted))

	idpiStatus := "disabled"
	idpiStyle := cli.WarningStyle
	if idpiEnabled {
		idpiStatus = "enabled"
		idpiStyle = cli.SuccessStyle
	}
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "idpi", cli.StyleStdout(idpiStyle, idpiStatus))
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "Next steps:"))
	switch healthState {
	case healthSnapshotRunning:
		_, _ = fmt.Fprintf(out, "  %-64s %s\n", cli.StyleStdout(cli.CommandStyle, "export PINCHTAB_SESSION=$(pinchtab session create --agent-id <id>)"), cli.StyleStdout(cli.MutedStyle, "# start a dedicated session"))
		_, _ = fmt.Fprintf(out, "  %-64s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab nav <url>"), cli.StyleStdout(cli.MutedStyle, "# open a page in the current tab"))
		_, _ = fmt.Fprintf(out, "  %-64s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab snap"), cli.StyleStdout(cli.MutedStyle, "# inspect interactive elements"))
	case healthSnapshotProtected:
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config token"), cli.StyleStdout(cli.MutedStyle, "# copy configured API token"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab health --json"), cli.StyleStdout(cli.MutedStyle, "# retry health with the current token"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config show"), cli.StyleStdout(cli.MutedStyle, "# inspect configured port and token"))
	case healthSnapshotInvalid, healthSnapshotUnhealthy:
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab health --json"), cli.StyleStdout(cli.MutedStyle, "# inspect the current listener"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config show"), cli.StyleStdout(cli.MutedStyle, "# verify configured port/token"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab server"), cli.StyleStdout(cli.MutedStyle, "# start after freeing the port"))
	default:
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab server"), cli.StyleStdout(cli.MutedStyle, "# start the server (foreground)"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab server -y"), cli.StyleStdout(cli.MutedStyle, "# start with guards down (this run only)"))
		_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon install"), cli.StyleStdout(cli.MutedStyle, "# install background service"))
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "Configure:"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config show"), cli.StyleStdout(cli.MutedStyle, "# view current config"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security"), cli.StyleStdout(cli.MutedStyle, "# review security posture"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab --help"), cli.StyleStdout(cli.MutedStyle, "# full command list"))
}

func Execute() {
	safelog.InstallDefault()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serverURL is the global --server flag for CLI commands
var serverURL string
var cliAgentID string

func init() {
	config.SetConfigSchemaVersion(version)

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("pinchtab {{.Version}}\n")

	// Global flags
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "PinchTab server URL (default: http://127.0.0.1:<server.port>)")
	rootCmd.PersistentFlags().StringVar(&cliAgentID, "agent-id", "", "Agent identifier recorded in activity logs")

	// Grouping commands
	primaryGroup := &cobra.Group{ID: "primary", Title: "Primary Commands"}
	browserGroup := &cobra.Group{ID: "browser", Title: "Browser Control"}
	mgmtGroup := &cobra.Group{ID: "management", Title: "Profiles and Instances"}
	configGroup := &cobra.Group{ID: "config", Title: "Configuration & Setup"}

	rootCmd.AddGroup(primaryGroup, browserGroup, mgmtGroup, configGroup)

	cli.SetupUsage(rootCmd)
}
