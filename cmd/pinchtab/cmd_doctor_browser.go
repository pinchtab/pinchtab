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
	return runDoctorChecks(cmd, cfg, doctorBrowserCheck, "pinchtab doctor browser", target)
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

func writeBrowserOverview(w io.Writer, report doctor.BrowsersReport, focus string) {
	_, _ = fmt.Fprintf(w, "pinchtab doctor browser\n\n")

	_, _ = fmt.Fprintf(w, "Supported browsers:\n\n")

	for _, bi := range report.Browsers {
		if focus != "" && bi.Name != focus {
			continue
		}

		_, _ = fmt.Fprintf(w, "  %s %-14s %s\n", doctor.BrowserStatusMarker(bi.Status), bi.Name, bi.StatusDetail)

		if hint := browserOverviewHint(bi); hint != "" {
			_, _ = fmt.Fprintf(w, "    %s\n", hint)
		}

		for _, c := range bi.Checks {
			doctor.WriteBrowserCheckRow(w, c)
		}
	}

	if focus == "" {
		_, _ = fmt.Fprintln(w)
		if report.DefaultBrowser != "" {
			_, _ = fmt.Fprintf(w, "Default: %s\n", report.DefaultBrowser)
		}
		_, _ = fmt.Fprintf(w, "\n%s\n", doctor.BrowserLegend)
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

func browserOverviewHint(bi doctor.BrowserInfo) string {
	if bi.Name == "cloak" {
		presence := cloakPresenceResult(bi)
		if presence != nil && presence.Status != doctor.StatusPass {
			if strings.Contains(presence.Detail, "browser.binary is unset") {
				return "Set browser.binary (or browser.targets.<name>.binary) to the discovered CloakBrowser executable."
			}
			if strings.Contains(presence.Detail, "configured browser.binary") {
				return "Fix browser.binary (or browser.targets.<name>.binary) so it points to an executable CloakBrowser binary."
			}
		}
		if presence != nil && presence.Status == doctor.StatusPass && hasFailedCheck(bi, "cdp_reachable") {
			if binary := cloakPresentBinary(presence.Detail); binary != "" {
				return fmt.Sprintf("CloakBrowser was found at %s but did not accept CDP. Try updating or reinstalling it.", binary)
			}
		}
	}
	if bi.Status != "ready" {
		return doctor.BrowserInstallHints[bi.Name]
	}
	return ""
}

func cloakPresenceResult(bi doctor.BrowserInfo) *doctor.CheckResult {
	for i := range bi.Checks {
		if bi.Checks[i].Name == "cloakbrowser_present" {
			return &bi.Checks[i]
		}
	}
	return nil
}

func hasFailedCheck(bi doctor.BrowserInfo, name string) bool {
	for _, check := range bi.Checks {
		if check.Name == name && check.Status == doctor.StatusFail {
			return true
		}
	}
	return false
}

func cloakPresentBinary(detail string) string {
	binary, _, ok := strings.Cut(detail, " -> ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(binary)
}

func init() {
	doctorBrowserCmd.Flags().StringVar(&doctorBrowserCheck, "check", "", "Run a single check by name (e.g. binary_exists)")
	doctorCmd.AddCommand(doctorBrowserCmd)
}
