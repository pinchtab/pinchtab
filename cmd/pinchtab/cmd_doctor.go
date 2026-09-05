package main

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/doctor"
	"github.com/spf13/cobra"
)

var (
	doctorJSON  bool
	doctorCheck string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run discovery and health checks against the configured browser",
	Long: `Run a series of read-only diagnostic checks against the current
PinchTab configuration. On the default provider they cover the config file,
the browser binary (found, version adequate), a headless launch that must
expose CDP, and the request shapes the provider handles; CloakBrowser adds
fingerprint-flag and font checks. The browser is resolved the way the server
resolves it: browser.binary when set, otherwise discovery.

The doctor command inspects the installation and does not require a running
PinchTab server; it may launch a short-lived browser subprocess, which is
always torn down. It never reports on a running browser: if a server answers
at the configured address, doctor names it and the surfaces (/health, pinchtab
security, instance metrics) that carry the runtime state. The summary says
whether any check launched a browser, so a run of skips never reads as a
clean bill of health.

Exit codes:
  0  all checks passed or were skipped
  1  at least one check failed
  2  usage or setup error (e.g. config could not be loaded)`,
	Example: `  pinchtab doctor
  pinchtab doctor --json
  pinchtab doctor browser cloak-eu
  pinchtab doctor --check binary_exists`,
	RunE:          runDoctor,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	cfg, err := loadDoctorConfig()
	if err != nil {
		return newCommandExitError(2, fmt.Errorf("pinchtab doctor: %w", err))
	}
	return runDoctorChecks(cmd, cfg, doctorCheck, "pinchtab doctor", "")
}

// runDoctorChecks validates the optional single-check filter, runs the doctor
// checks, renders JSON/text, and maps the summary to an exit code. errPrefix is
// the command label used in error messages; target is the browser-target label
// passed to the renderers ("" for the top-level doctor command).
func runDoctorChecks(cmd *cobra.Command, cfg *config.RuntimeConfig, check, errPrefix, target string) error {
	check = strings.TrimSpace(check)
	if check != "" {
		if !doctor.KnownCheck(cfg, check) {
			return newCommandExitError(2, fmt.Errorf("%s: unknown check %q for browser=%s", errPrefix, check, cfg.DefaultBrowser))
		}
	}

	results := doctor.Run(cmd.Context(), cfg, check)
	browser := config.NormalizeBrowser(cfg.DefaultBrowser)
	out := cmd.OutOrStdout()
	runtime := doctor.ProbeRuntime(cmd.Context(), cfg)

	if doctorJSON {
		if err := doctor.WriteJSON(out, browser, target, results, runtime); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	} else {
		doctor.WriteText(out, browser, target, results, runtime)
	}

	summary := doctor.Summarize(results)
	code := doctor.ExitCode(summary)
	if code != 0 {
		return newCommandExitError(code, fmt.Errorf("%s: %d check(s) failed", errPrefix, summary.Failed))
	}
	return nil
}

func loadDoctorConfig() (*config.RuntimeConfig, error) {
	cfg := config.Load()
	if cfg == nil {
		return nil, fmt.Errorf("no configuration found")
	}
	return cfg, nil
}

func init() {
	doctorCmd.GroupID = "config"
	doctorCmd.PersistentFlags().BoolVar(&doctorJSON, "json", false, "Emit machine-readable JSON")
	doctorCmd.Flags().StringVar(&doctorCheck, "check", "", "Run a single check by name (e.g. binary_exists)")
	rootCmd.AddCommand(doctorCmd)
}
