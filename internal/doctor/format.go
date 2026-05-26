package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type jsonReport struct {
	Browser string        `json:"browser"`
	Target  string        `json:"target,omitempty"`
	Results []CheckResult `json:"results"`
	Summary Summary       `json:"summary"`
}

func WriteText(w io.Writer, browser, target string, results []CheckResult) {
	header := fmt.Sprintf("pinchtab doctor (browser=%s", browser)
	if target != "" {
		header += ", target=" + target
	}
	header += ")"
	_, _ = fmt.Fprintln(w, header)
	_, _ = fmt.Fprintln(w)

	for _, r := range results {
		_, _ = fmt.Fprintln(w, formatResultLine(r))
	}

	s := Summarize(results)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%d passed, %d failed, %d skipped, %d warnings.\n",
		s.Passed, s.Failed, s.Skipped, s.Warnings)
}

func formatResultLine(r CheckResult) string {
	marker := statusMarker(r.Status)
	detail := r.Detail
	if detail == "" && r.ErrMsg != "" {
		detail = r.ErrMsg
	}
	return fmt.Sprintf("%s %-28s %s (%s)", marker, r.Name, detail, shortDuration(r.Duration))
}

func statusMarker(s CheckStatus) string {
	switch s {
	case StatusPass:
		return "OK  "
	case StatusFail:
		return "FAIL"
	case StatusWarn:
		return "WARN"
	case StatusSkip:
		return "SKIP"
	default:
		return "?   "
	}
}

func shortDuration(d time.Duration) string {
	if d <= 0 {
		return "0ms"
	}
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func WriteJSON(w io.Writer, browser, target string, results []CheckResult) error {
	// Populate ErrMsg for JSON consumers in case a check forgot to copy from Err.
	for i := range results {
		if results[i].Err != nil && results[i].ErrMsg == "" {
			results[i].ErrMsg = results[i].Err.Error()
		}
	}
	report := jsonReport{
		Browser: browser,
		Target:  target,
		Results: results,
		Summary: Summarize(results),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
