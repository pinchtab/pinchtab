package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorBrowsersCmd = &cobra.Command{
	Use:   "browsers",
	Short: "Report configured and known browsers with availability and capabilities",
	Long: `Enumerate all configured and known browsers, showing for each
whether it is registered in the browser registry, configured in the
runtime config, and which request shapes it can handle.

Doctor checks contributed by each registered browser are also executed
and their results included in the report.`,
	RunE:          runDoctorBrowsers,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func runDoctorBrowsers(cmd *cobra.Command, _ []string) error {
	cfg, err := loadDoctorConfig()
	if err != nil {
		return newCommandExitError(2, fmt.Errorf("pinchtab doctor browsers: %w", err))
	}

	report := doctor.ReportBrowsers(cmd.Context(), cfg)
	out := cmd.OutOrStdout()

	if doctorJSON {
		return writeBrowsersJSON(out, report)
	}
	writeBrowsersText(out, report)
	return nil
}

func writeBrowsersJSON(w io.Writer, report doctor.BrowsersReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeBrowsersText(w io.Writer, report doctor.BrowsersReport) {
	_, _ = fmt.Fprintf(w, "pinchtab doctor browsers\n\n")

	// Compact status summary.
	for _, bi := range report.Browsers {
		_, _ = fmt.Fprintf(w, "  %-18s %-12s %s\n", bi.Name, bi.Status, bi.StatusDetail)
	}
	_, _ = fmt.Fprintln(w)

	// Config summary.
	if len(report.ConfiguredBrowsers) > 0 {
		_, _ = fmt.Fprintf(w, "Configured: %s\n", strings.Join(report.ConfiguredBrowsers, ", "))
	} else {
		_, _ = fmt.Fprintln(w, "Configured: (none)")
	}
	if report.DefaultBrowser != "" {
		_, _ = fmt.Fprintf(w, "Default:    %s\n", report.DefaultBrowser)
	}
	_, _ = fmt.Fprintf(w, "Known:      %s\n\n", strings.Join(report.KnownBrowsers, ", "))

	for _, bi := range report.Browsers {
		_, _ = fmt.Fprintf(w, "Browser: %s\n", bi.Name)

		if bi.Registered {
			_, _ = fmt.Fprintln(w, "  Status: registered")
		} else {
			_, _ = fmt.Fprintln(w, "  Status: not registered")
		}
		if bi.IsDefault {
			_, _ = fmt.Fprintln(w, "  Default: yes")
		}
		if bi.Configured {
			_, _ = fmt.Fprintln(w, "  Configured: yes")
		} else {
			_, _ = fmt.Fprintln(w, "  Configured: no")
		}
		if len(bi.Handles) > 0 {
			_, _ = fmt.Fprintf(w, "  Handles: %s\n", strings.Join(bi.Handles, ", "))
		}
		if len(bi.SkipsOrFails) > 0 {
			_, _ = fmt.Fprintf(w, "  Skips: %s\n", strings.Join(bi.SkipsOrFails, ", "))
		}

		if len(bi.Checks) > 0 {
			_, _ = fmt.Fprintln(w, "  Checks:")
			for _, c := range bi.Checks {
				marker := checkMarker(c.Status)
				detail := c.Detail
				if detail == "" && c.ErrMsg != "" {
					detail = c.ErrMsg
				}
				_, _ = fmt.Fprintf(w, "    %s %s: %s\n", marker, c.Name, detail)
			}
		} else if bi.Registered {
			_, _ = fmt.Fprintln(w, "  Checks: (none)")
		}
		_, _ = fmt.Fprintln(w)
	}
}

func checkMarker(s doctor.CheckStatus) string {
	switch s {
	case doctor.StatusPass:
		return "OK  "
	case doctor.StatusFail:
		return "FAIL"
	case doctor.StatusWarn:
		return "WARN"
	case doctor.StatusSkip:
		return "SKIP"
	default:
		return "?   "
	}
}

func init() {
	doctorCmd.AddCommand(doctorBrowsersCmd)
}
