package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/scrape"
	"github.com/spf13/cobra"
)

// scrapeTimeout bounds a whole site scrape: the HTTP crawl plus every
// browser-rendered page, well above the default per-request client timeout.
const scrapeTimeout = 15 * time.Minute

const scrapeFormatMarkdown = "md"

// Scrape runs a site scrape via POST /scrape and prints or writes the report.
func Scrape(client *http.Client, base, token string, cmd *cobra.Command, target string) (err error) {
	format := renderFormat(cmd)
	if format != "json" && format != scrapeFormatMarkdown {
		return fmt.Errorf("unsupported --format %q (json or md)", format)
	}

	base, cleanup, err := applyRunAuth(client, base, token, cmd, target)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			if err == nil {
				err = cleanupErr
			} else {
				err = fmt.Errorf("%w (also %v)", err, cleanupErr)
			}
		}
	}()

	body := map[string]any{"url": target}
	if v, _ := cmd.Flags().GetInt("max-pages"); v > 0 {
		body["maxPages"] = v
	}
	if v, _ := cmd.Flags().GetInt("max-per-pattern"); v > 0 {
		body["maxPerPattern"] = v
	}
	if v, _ := cmd.Flags().GetStringArray("include"); len(v) > 0 {
		body["includePatterns"] = v
	}
	if v, _ := cmd.Flags().GetStringArray("exclude"); len(v) > 0 {
		body["excludePatterns"] = v
	}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		body["concurrency"] = v
	}
	if v, _ := cmd.Flags().GetInt("timeout"); v > 0 {
		body["timeoutSeconds"] = v
	}
	if mustBool(cmd, "enrich-all") {
		body["enrichAll"] = true
	}
	if mustBool(cmd, "no-browser") {
		body["noBrowser"] = true
	}
	preview := mustBool(cmd, "preview")
	if preview {
		body["preview"] = true
	}
	if v, _ := cmd.Flags().GetStringArray("only"); len(v) > 0 {
		body["only"] = v
	}

	longClient := &http.Client{Transport: client.Transport, Timeout: scrapeTimeout}
	raw, err := apiclient.DoPostRawE(longClient, base, token, "/scrape", body)
	if err != nil {
		return err
	}

	var report scrape.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("parse scrape report: %w", err)
	}

	if dir := mustString(cmd, "output-dir"); dir != "" {
		if err := writeScrapeArtifacts(dir, raw, report, format); err != nil {
			return fmt.Errorf("write artifacts: %w", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", filepath.Join(dir, "report.json"))
	} else if format == scrapeFormatMarkdown {
		fmt.Println(string(scrape.RenderMarkdown(report)))
		return nil
	}

	if mustBool(cmd, "json") {
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
		return nil
	}
	if preview {
		printPreviewSummary(report)
		return nil
	}
	printScrapeSummary(report)
	return nil
}

// writeScrapeArtifacts writes report.json (the server response, indented)
// and, for --format md, the rendered report.md next to it.
func writeScrapeArtifacts(dir string, raw []byte, report scrape.Report, format string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var indented json.RawMessage = raw
	if out, err := json.MarshalIndent(json.RawMessage(raw), "", "  "); err == nil {
		indented = out
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), indented, 0o644); err != nil {
		return err
	}
	if format == scrapeFormatMarkdown {
		return os.WriteFile(filepath.Join(dir, "report.md"), scrape.RenderMarkdown(report), 0o644)
	}
	return nil
}

// printPreviewSummary prints the outline: one line per page with its size,
// routing verdict, and a snippet, so the caller can pick pages to expand with
// `scrape <url> --only <picked-urls>`.
func printPreviewSummary(report scrape.Report) {
	fmt.Printf("Preview: %d page(s) discovered of %d · pick pages to expand with --only\n",
		len(report.Pages), report.Site.TotalDiscovered)
	for _, p := range report.Pages {
		verdict := ""
		if p.BrowserRecommended {
			verdict = " · needs browser: " + strings.Join(p.BrowserReasons, ",")
		}
		if p.Error != "" {
			fmt.Printf("  %s · error: %s\n", p.URL, p.Error)
			continue
		}
		fmt.Printf("  %s · %d chars%s\n", p.URL, p.CharCount, verdict)
		if p.Snippet != "" {
			fmt.Printf("      %s\n", p.Snippet)
		}
	}
}

func printScrapeSummary(report scrape.Report) {
	fmt.Printf("Scraped %d page(s) · %d http · %d browser-rendered · %d failed\n",
		len(report.Pages), report.Summary.HTTPPages, report.Summary.BrowserPages, report.Summary.FailedPages)
	for _, p := range report.Pages {
		status := "source: " + p.Source
		if p.BrowserError != "" {
			status += " · browser failed: " + p.BrowserError
		}
		if p.Error != "" {
			status = "error: " + p.Error
		}
		fmt.Printf("  %s · %s\n", p.URL, status)
	}
}
