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
	Verdict string        `json:"verdict"`
	Scope   string        `json:"scope"`
	Runtime string        `json:"runtime,omitempty"`
}

func WriteText(w io.Writer, browser, target string, results []CheckResult, runtime string) {
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
	_, _ = fmt.Fprintf(w, "%d passed, %d failed, %d skipped, %d warnings. %s\n",
		s.Passed, s.Failed, s.Skipped, s.Warnings, Verdict(s))
	_, _ = fmt.Fprintln(w, ScopeStatement)
	if runtime != "" {
		_, _ = fmt.Fprintln(w, runtime)
	}
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

func WriteJSON(w io.Writer, browser, target string, results []CheckResult, runtime string) error {
	// Populate ErrMsg for JSON consumers in case a check forgot to copy from Err.
	for i := range results {
		if results[i].Err != nil && results[i].ErrMsg == "" {
			results[i].ErrMsg = results[i].Err.Error()
		}
	}
	summary := Summarize(results)
	report := jsonReport{
		Browser: browser,
		Target:  target,
		Results: results,
		Summary: summary,
		Verdict: Verdict(summary),
		Scope:   ScopeStatement,
		Runtime: runtime,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
