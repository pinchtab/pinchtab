package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorBrowserCheck string

var doctorBrowserCmd = &cobra.Command{
	Use:   "browser [name]",
	Short: "Check browser availability or run checks for a specific target",
	Long: `Without arguments: show all supported browsers, their availability,
and install instructions for any that are missing.

With a target name: run doctor checks scoped to that browser target.`,
	Example: `  pinchtab doctor browser
  pinchtab doctor browser chrome
  pinchtab doctor browser cloak-eu --json
  pinchtab doctor browser cloak-eu --check binary_exists`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runDoctorBrowser,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runDoctorBrowser(cmd *cobra.Command, args []string) error {
	cfg, err := loadDoctorConfig()
	if err != nil {
		return newCommandExitError(2, fmt.Errorf("pinchtab doctor browser: %w", err))
	}

	if len(args) == 0 {
		return runDoctorBrowserOverview(cmd, cfg, "")
	}

	target := strings.TrimSpace(args[0])

	// A bare known browser ID (not a configured target) gets the overview
	// focused on that browser; anything else is an error so scripts can
	// rely on the documented exit contract.
	resolved, err := config.ResolveExplicitBrowserTarget(cfg, target)
	if err != nil {
		id := strings.ToLower(target)
		if _, known := browsers.Get(id); known {
			return runDoctorBrowserOverview(cmd, cfg, id)
		}
		return newCommandExitError(2, fmt.Errorf("pinchtab doctor browser: unknown browser or target %q: %w", target, err))
	}

	cfg = resolved.Config
	check := strings.TrimSpace(doctorBrowserCheck)
	if check != "" {
		if !doctor.KnownCheck(cfg, check) {
			return newCommandExitError(2, fmt.Errorf("pinchtab doctor browser: unknown check %q for browser=%s", check, cfg.DefaultBrowser))
		}
	}

	results := doctor.Run(cmd.Context(), cfg, check)
	browser := config.NormalizeBrowser(cfg.DefaultBrowser)
	out := cmd.OutOrStdout()

	if doctorJSON {
		if err := doctor.WriteJSON(out, browser, target, results); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	} else {
		doctor.WriteText(out, browser, target, results)
	}

	summary := doctor.Summarize(results)
	code := doctor.ExitCode(summary)
	if code != 0 {
		return newCommandExitError(code, fmt.Errorf("pinchtab doctor browser: %d check(s) failed", summary.Failed))
	}
	return nil
}

func runDoctorBrowserOverview(cmd *cobra.Command, cfg *config.RuntimeConfig, focus string) error {
	report := doctor.ReportBrowsers(cmd.Context(), cfg)
	out := cmd.OutOrStdout()

	if doctorJSON {
		return writeBrowsersJSON(out, report)
	}
	writeBrowserOverview(out, report, focus)
	return nil
}

var browserInstallHints = map[string]string{
	"chrome":       "Install Google Chrome: https://www.google.com/chrome/ or via package manager (apt install google-chrome-stable / brew install --cask google-chrome)",
	"cloak":        "CloakBrowser requires a custom build. See: docs/guides/cloakbrowser.md",
	"ghost-chrome": "ghost-chrome is built-in (uses Chrome with static-first routing). Ensure Chrome is installed.",
}

func writeBrowserOverview(w io.Writer, report doctor.BrowsersReport, focus string) {
	_, _ = fmt.Fprintf(w, "pinchtab doctor browser\n\n")

	_, _ = fmt.Fprintf(w, "Supported browsers:\n\n")

	for _, bi := range report.Browsers {
		if focus != "" && bi.Name != focus {
			continue
		}

		var marker string
		switch bi.Status {
		case "ready":
			marker = "✓"
		case "needs-config":
			marker = "~"
		default:
			marker = "✗"
		}

		_, _ = fmt.Fprintf(w, "  %s %-14s %s\n", marker, bi.Name, bi.StatusDetail)

		if bi.Status != "ready" {
			if hint, ok := browserInstallHints[bi.Name]; ok {
				_, _ = fmt.Fprintf(w, "    %s\n", hint)
			}
		}

		if len(bi.Checks) > 0 {
			for _, c := range bi.Checks {
				m := checkMarker(c.Status)
				detail := c.Detail
				if detail == "" && c.ErrMsg != "" {
					detail = c.ErrMsg
				}
				_, _ = fmt.Fprintf(w, "    %s %s: %s\n", m, c.Name, detail)
			}
		}
	}

	if focus == "" {
		_, _ = fmt.Fprintln(w)
		if report.DefaultBrowser != "" {
			_, _ = fmt.Fprintf(w, "Default: %s\n", report.DefaultBrowser)
		}
		_, _ = fmt.Fprintf(w, "\nLegend: ✓ ready  ~ needs config  ✗ not available\n")
	} else {
		found := false
		for _, bi := range report.Browsers {
			if bi.Name == focus {
				found = true
				break
			}
		}
		if !found {
			_, _ = fmt.Fprintf(w, "\n  Browser %q is not a known provider.\n", focus)
			_, _ = fmt.Fprintf(w, "  Known browsers: %s\n", strings.Join(report.KnownBrowsers, ", "))
		}
	}
}

func init() {
	doctorBrowserCmd.Flags().StringVar(&doctorBrowserCheck, "check", "", "Run a single check by name (e.g. binary_exists)")
	doctorCmd.AddCommand(doctorBrowserCmd)
}
