package stealth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Report is the on-disk shape written by the stealth-score subagent.
type Report struct {
	Provider       string    `json:"provider"`
	Timestamp      string    `json:"timestamp"`
	StartedAt      string    `json:"started_at,omitempty"`
	CompletedAt    string    `json:"completed_at,omitempty"`
	SitesProcessed int       `json:"sites_processed,omitempty"`
	ArtifactDir    string    `json:"artifact_dir,omitempty"`
	Sites          []SiteRow `json:"sites"`
}

type SiteRow struct {
	Site    string            `json:"site"`
	URL     string            `json:"url,omitempty"`
	TabID   string            `json:"tab_id,omitempty"`
	Metrics map[string]string `json:"metrics"`
	Notes   string            `json:"notes,omitempty"`
}

// Divergence is one metric where two providers disagreed on real values.
type Divergence struct {
	Site   string `json:"site"`
	Metric string `json:"metric"`
}

// HistoryEntry is the on-disk shape appended to history.jsonl per compare run.
type HistoryEntry struct {
	RunID            string         `json:"run_id"`
	AppendedAt       string         `json:"appended_at"`
	Providers        []string       `json:"providers"`
	SitesTotal       int            `json:"sites_total"`
	Captured         map[string]int `json:"captured"`
	Divergences      int            `json:"divergences"`
	DivergentMetrics []Divergence   `json:"divergent_metrics"`
}

const historyRenderLimit = 20

// RunCompare implements `runner stealth compare`.
//
// usage: runner stealth compare [--no-history] [--history-dir <dir>] <report1.json> <report2.json> ...
//
// Loads each report, prints a markdown comparison to stdout, and (unless
// --no-history is set) appends a one-line summary to history.jsonl + regenerates
// history.md in the chosen --history-dir. When exactly two reports are passed,
// a "Divergences" callout is rendered above the per-site tables.
func RunCompare(argv []string, stdout, stderr io.Writer) int {
	var (
		noHistory  bool
		historyDir string
		paths      []string
	)

	i := 0
	for i < len(argv) {
		switch argv[i] {
		case "--no-history":
			noHistory = true
			i++
		case "--history-dir":
			if i+1 >= len(argv) {
				_, _ = fmt.Fprintln(stderr, "stealth compare: --history-dir requires a value")
				return 1
			}
			historyDir = argv[i+1]
			i += 2
		case "-h", "--help":
			_, _ = fmt.Fprintln(stdout, compareUsage)
			return 0
		default:
			if strings.HasPrefix(argv[i], "-") {
				_, _ = fmt.Fprintf(stderr, "stealth compare: unknown option %q\n", argv[i])
				return 1
			}
			paths = append(paths, argv[i])
			i++
		}
	}

	if len(paths) == 0 {
		_, _ = fmt.Fprintln(stderr, compareUsage)
		return 2
	}

	reports := make([]Report, 0, len(paths))
	for _, p := range paths {
		r, err := loadReport(p)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "stealth compare: %v\n", err)
			return 1
		}
		reports = append(reports, r)
	}

	md, divs := render(paths, reports)
	_, _ = io.WriteString(stdout, md)

	if !noHistory && len(reports) == 2 {
		dir := historyDir
		if dir == "" {
			// Default: place history files next to the first report.
			dir = filepath.Dir(paths[0])
			// If reports/ → ../ (so history.jsonl is in tests/stealth-score/).
			if filepath.Base(dir) == "results" {
				dir = filepath.Dir(dir)
			}
		}
		if err := appendHistory(dir, reports, divs); err != nil {
			_, _ = fmt.Fprintf(stderr, "stealth compare: history append failed: %v\n", err)
			// Don't fail the whole run on history errors.
		}
	}
	return 0
}

const compareUsage = `usage: runner stealth compare [--no-history] [--history-dir <dir>] <report1.json> [<report2.json> ...]

Renders a side-by-side markdown comparison from per-provider stealth-score
JSON reports. When exactly two reports are passed, a "Divergences" callout
shows metrics where the providers disagreed on real values (entries marked
"unavailable" on either side are excluded).

Options:
  --no-history          Do not append to history.jsonl / regenerate history.md.
  --history-dir <dir>   Where history.jsonl/history.md live. Default: parent of
                        the reports' directory (so a typical run drops history
                        next to the skill's tests/stealth-score/ directory).`

func loadReport(path string) (Report, error) {
	var r Report
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}

// collectSites returns site IDs in declaration order across reports, plus a
// per-site slot table (one slot per report position).
func collectSites(reports []Report) ([]string, map[string][]*SiteRow) {
	var ordered []string
	seen := map[string]bool{}
	for _, r := range reports {
		for _, s := range r.Sites {
			if !seen[s.Site] {
				seen[s.Site] = true
				ordered = append(ordered, s.Site)
			}
		}
	}
	slots := map[string][]*SiteRow{}
	for _, sid := range ordered {
		slots[sid] = make([]*SiteRow, len(reports))
	}
	for ri := range reports {
		r := &reports[ri]
		for si := range r.Sites {
			s := &r.Sites[si]
			slots[s.Site][ri] = s
		}
	}
	return ordered, slots
}

// divergences returns metrics where two providers gave different real values.
// Only meaningful with exactly two reports; caller guards.
func divergences(sites []string, slots map[string][]*SiteRow) []Divergence {
	var out []Divergence
	for _, sid := range sites {
		entries := slots[sid]
		a, b := entries[0], entries[1]
		if a == nil || b == nil {
			continue
		}
		keys := map[string]bool{}
		for k := range a.Metrics {
			keys[k] = true
		}
		for k := range b.Metrics {
			keys[k] = true
		}
		var ks []string
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			av, aok := a.Metrics[k]
			bv, bok := b.Metrics[k]
			if !aok || !bok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(av), "unavailable") || strings.EqualFold(strings.TrimSpace(bv), "unavailable") {
				continue
			}
			if strings.TrimSpace(av) == strings.TrimSpace(bv) {
				continue
			}
			out = append(out, Divergence{Site: sid, Metric: k})
		}
	}
	return out
}

func shorten(s string, n int) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func render(paths []string, reports []Report) (string, []Divergence) {
	providers := make([]string, len(reports))
	for i, r := range reports {
		providers[i] = r.Provider
		if providers[i] == "" {
			providers[i] = "?"
		}
	}
	sites, slots := collectSites(reports)

	runID := ""
	if len(reports) > 0 {
		runID = reports[0].Timestamp
	}
	if runID == "" {
		runID = time.Now().UTC().Format("20060102T150405Z")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Stealth Score — %s — %s\n\n", strings.Join(providers, " vs "), runID)
	fmt.Fprintf(&b, "**Run id**: `%s`  \n", runID)
	counts := make([]string, len(reports))
	for i, r := range reports {
		counts[i] = fmt.Sprintf("%s: %d", providers[i], len(r.Sites))
	}
	fmt.Fprintf(&b, "**Sites**: %d total (%s)  \n", len(sites), strings.Join(counts, " / "))

	var divs []Divergence
	if len(reports) == 2 {
		divs = divergences(sites, slots)
		fmt.Fprintf(&b, "**Divergent metrics**: %d\n\n", len(divs))
		if len(divs) > 0 {
			b.WriteString("## Divergences (where providers disagree)\n\n")
			fmt.Fprintf(&b, "| site | metric | %s | %s |\n", providers[0], providers[1])
			b.WriteString("|---|---|---|---|\n")
			for _, d := range divs {
				a := slots[d.Site][0]
				bRow := slots[d.Site][1]
				av, bv := a.Metrics[d.Metric], bRow.Metrics[d.Metric]
				fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", d.Site, d.Metric, shorten(av, 60), shorten(bv, 60))
			}
			b.WriteString("\n")
		} else {
			b.WriteString("_no divergent metrics — both providers reported the same values everywhere data was available._\n\n")
		}
	} else {
		b.WriteString("\n")
	}

	b.WriteString("## Full per-site comparison\n\n")
	for _, sid := range sites {
		entries := slots[sid]
		var url string
		for _, e := range entries {
			if e != nil && e.URL != "" {
				url = e.URL
				break
			}
		}
		fmt.Fprintf(&b, "### %s\n%s\n\n", sid, url)

		// Union of keys, in declaration order across providers.
		var keys []string
		seen := map[string]bool{}
		for _, e := range entries {
			if e == nil {
				continue
			}
			var local []string
			for k := range e.Metrics {
				local = append(local, k)
			}
			sort.Strings(local)
			for _, k := range local {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}

		if len(keys) == 0 {
			b.WriteString("_no metrics captured_\n")
			for ri, e := range entries {
				if e != nil && e.Notes != "" {
					fmt.Fprintf(&b, "- _%s note: %s_\n", providers[ri], e.Notes)
				}
			}
			b.WriteString("\n")
			continue
		}

		header := append([]string{"metric"}, providers...)
		b.WriteString("| " + strings.Join(header, " | ") + " |\n")
		b.WriteString("|" + strings.Repeat("---|", len(header)) + "\n")
		for _, k := range keys {
			row := []string{k}
			for _, e := range entries {
				if e == nil {
					row = append(row, "—")
					continue
				}
				v, ok := e.Metrics[k]
				if !ok {
					row = append(row, "—")
					continue
				}
				row = append(row, shorten(v, 60))
			}
			b.WriteString("| " + strings.Join(row, " | ") + " |\n")
		}
		for ri, e := range entries {
			if e != nil && e.Notes != "" {
				fmt.Fprintf(&b, "\n- _%s note: %s_\n", providers[ri], e.Notes)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Artifacts\n")
	for i, p := range paths {
		rel := p
		if abs, err := filepath.Abs(p); err == nil {
			if cwd, err := os.Getwd(); err == nil {
				if r, err := filepath.Rel(cwd, abs); err == nil {
					rel = r
				}
			}
		}
		fmt.Fprintf(&b, "- %s: `%s`\n", providers[i], rel)
	}
	b.WriteString("\n")
	return b.String(), divs
}

func appendHistory(dir string, reports []Report, divs []Divergence) error {
	if dir == "" {
		return fmt.Errorf("history dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir history dir: %w", err)
	}

	providers := make([]string, len(reports))
	captured := map[string]int{}
	maxSites := 0
	for i, r := range reports {
		providers[i] = r.Provider
		captured[r.Provider] = len(r.Sites)
		if len(r.Sites) > maxSites {
			maxSites = len(r.Sites)
		}
	}
	entry := HistoryEntry{
		RunID:            reports[0].Timestamp,
		AppendedAt:       time.Now().UTC().Format(time.RFC3339),
		Providers:        providers,
		SitesTotal:       maxSites,
		Captured:         captured,
		Divergences:      len(divs),
		DivergentMetrics: divs,
	}
	if entry.DivergentMetrics == nil {
		entry.DivergentMetrics = []Divergence{}
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal history entry: %w", err)
	}

	jsonlPath := filepath.Join(dir, "history.jsonl")
	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history.jsonl: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("write history.jsonl: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close history.jsonl: %w", err)
	}

	return renderHistoryMD(dir, historyRenderLimit)
}

func renderHistoryMD(dir string, limit int) error {
	jsonlPath := filepath.Join(dir, "history.jsonl")
	raw, err := os.ReadFile(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read history.jsonl: %w", err)
	}
	var rows []HistoryEntry
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			// Skip malformed lines rather than failing — history is best-effort.
			continue
		}
		rows = append(rows, e)
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	// Reverse so newest renders first.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Stealth Score — History (last %d runs)\n\n", len(rows))
	b.WriteString("| run_id | providers | sites | divergences | divergent metrics |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range rows {
		var sample []string
		for _, d := range r.DivergentMetrics {
			if len(sample) >= 5 {
				break
			}
			sample = append(sample, fmt.Sprintf("%s/%s", d.Site, d.Metric))
		}
		summary := strings.Join(sample, "; ")
		if rest := len(r.DivergentMetrics) - len(sample); rest > 0 {
			summary += fmt.Sprintf(" (+%d more)", rest)
		}
		if summary == "" {
			summary = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %d | %d | %s |\n", r.RunID, strings.Join(r.Providers, ", "), r.SitesTotal, r.Divergences, summary)
	}

	mdPath := filepath.Join(dir, "history.md")
	return os.WriteFile(mdPath, []byte(b.String()), 0o644)
}
