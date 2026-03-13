package engine

// engine_benchmark_test.go — Cross-engine benchmark study
//
// Compares Lite (Gost-DOM), Lightpanda (CDP), and Chrome (CDP) engines
// across representative page types. Measures navigate+snapshot latency,
// snapshot node count, and estimated token cost.
//
// Run with local Lite engine only (always available):
//   go test ./internal/engine/ -run TestEngineBenchmark -v
//
// Include Lightpanda (requires lightpanda serve on :9222):
//   LIGHTPANDA_URL=ws://127.0.0.1:9222 go test ./internal/engine/ -run TestEngineBenchmark -v
//
// Include Chrome (requires PINCHTAB_CHROME_PATH or chrome in PATH):
//   CHROME_BENCH=1 go test ./internal/engine/ -run TestEngineBenchmark -v
//
// All three:
//   LIGHTPANDA_URL=ws://127.0.0.1:9222 CHROME_BENCH=1 go test ./internal/engine/ -run TestEngineBenchmark -v

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------
// Test pages — representative of real-world agent targets
// -----------------------------------------------------------------------

type benchPage struct {
	name string
	html string
}

func benchPages() []benchPage {
	return []benchPage{
		{"static-article", staticArticlePage},
		{"form-heavy", benchFormHeavyPage},
		{"deep-nesting", deepNestingPage},
		{"large-dom", generateLargeDOMPage(500)},
		{"spa-shell", spaShellPage},
		{"data-table", dataTablePage},
		{"dashboard", dashboardPage},
		{"ecommerce", ecommercePage},
	}
}

// -----------------------------------------------------------------------
// Benchmark result tracking
// -----------------------------------------------------------------------

type benchResult struct {
	engine     string
	page       string
	navigateNs int64
	snapshotNs int64
	totalNs    int64
	nodeCount  int
	interCount int
	estTokens  int // estimated LLM tokens for snapshot
	textLen    int
	err        string
}

// estimateTokens returns a rough token count for a snapshot.
// ~4 chars per token is the standard approximation.
func estimateTokens(nodes []SnapshotNode) int {
	total := 0
	for _, n := range nodes {
		// ref + role + name + value + structural overhead
		total += len(n.Ref) + len(n.Role) + len(n.Name) + len(n.Value) + 20
	}
	return total / 4
}

// -----------------------------------------------------------------------
// Engine providers
// -----------------------------------------------------------------------

type engineProvider struct {
	name    string
	create  func(t *testing.T) Engine
	cleanup func()
}

func availableEngines(t *testing.T) []engineProvider {
	t.Helper()
	var engines []engineProvider

	// Lite is always available — no external dependencies.
	engines = append(engines, engineProvider{
		name:   "lite",
		create: func(t *testing.T) Engine { return NewLiteEngine() },
	})

	// Lightpanda if LIGHTPANDA_URL is set.
	if lpURL := os.Getenv("LIGHTPANDA_URL"); lpURL != "" {
		engines = append(engines, engineProvider{
			name: "lightpanda",
			create: func(t *testing.T) Engine {
				lp, err := NewLightpandaEngine(lpURL)
				if err != nil {
					t.Skipf("lightpanda engine unavailable: %v", err)
					return nil
				}
				return lp
			},
		})
	}

	// Chrome if CHROME_BENCH=1.
	if os.Getenv("CHROME_BENCH") == "1" {
		engines = append(engines, engineProvider{
			name: "chrome-via-lite",
			create: func(t *testing.T) Engine {
				t.Skip("chrome benchmark requires manual setup — use integration tests")
				return nil
			},
		})
	}

	return engines
}

// -----------------------------------------------------------------------
// Main benchmark
// -----------------------------------------------------------------------

func TestEngineBenchmark(t *testing.T) {
	pages := benchPages()
	engines := availableEngines(t)

	if len(engines) == 0 {
		t.Skip("no engines available")
	}

	var results []benchResult

	for _, ep := range engines {
		t.Run(ep.name, func(t *testing.T) {
			for _, page := range pages {
				t.Run(page.name, func(t *testing.T) {
					ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						_, _ = w.Write([]byte(page.html))
					}))
					defer ts.Close()

					eng := ep.create(t)
					if eng == nil {
						return
					}
					defer func() { _ = eng.Close() }()

					ctx := context.Background()
					br := benchResult{engine: ep.name, page: page.name}

					// Navigate
					navStart := time.Now()
					_, err := eng.Navigate(ctx, ts.URL)
					br.navigateNs = time.Since(navStart).Nanoseconds()
					if err != nil {
						br.err = err.Error()
						results = append(results, br)
						t.Logf("navigate error: %v", err)
						return
					}

					// Snapshot (all nodes)
					snapStart := time.Now()
					nodes, err := eng.Snapshot(ctx, "")
					br.snapshotNs = time.Since(snapStart).Nanoseconds()
					if err != nil {
						br.err = err.Error()
						results = append(results, br)
						t.Logf("snapshot error: %v", err)
						return
					}
					br.nodeCount = len(nodes)
					br.estTokens = estimateTokens(nodes)

					// Interactive count
					interNodes, _ := eng.Snapshot(ctx, "interactive")
					br.interCount = len(interNodes)

					// Text
					text, _ := eng.Text(ctx)
					br.textLen = len(text)

					br.totalNs = br.navigateNs + br.snapshotNs

					results = append(results, br)

					t.Logf("nav=%s snap=%s total=%s nodes=%d interactive=%d tokens≈%d text=%d",
						fmtDuration(br.navigateNs),
						fmtDuration(br.snapshotNs),
						fmtDuration(br.totalNs),
						br.nodeCount, br.interCount, br.estTokens, br.textLen)
				})
			}
		})
	}

	// Print summary table.
	if len(results) > 0 {
		printBenchSummary(t, results)
	}
}

// TestEngineBenchmark_SnapshotLatency is a Go benchmark (testing.B) for
// hot-path snapshot latency after navigation.
func TestEngineBenchmark_SnapshotLatency(t *testing.T) {
	pages := benchPages()

	for _, page := range pages[:3] { // first 3 pages for quick iteration
		t.Run(page.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(page.html))
			}))
			defer ts.Close()

			lite := NewLiteEngine()
			defer func() { _ = lite.Close() }()

			ctx := context.Background()
			_, err := lite.Navigate(ctx, ts.URL)
			if err != nil {
				t.Fatalf("navigate: %v", err)
			}

			// Warm up
			_, _ = lite.Snapshot(ctx, "")

			const iterations = 100
			start := time.Now()
			for i := 0; i < iterations; i++ {
				_, _ = lite.Snapshot(ctx, "")
			}
			elapsed := time.Since(start)

			t.Logf("%d snapshots in %s (avg %s/op)",
				iterations, elapsed, fmtDuration(elapsed.Nanoseconds()/int64(iterations)))
		})
	}
}

// TestEngineBenchmark_NavigateLatency measures cold-start navigate across
// different page sizes.
func TestEngineBenchmark_NavigateLatency(t *testing.T) {
	sizes := []struct {
		name  string
		elems int
	}{
		{"tiny-10", 10},
		{"small-50", 50},
		{"medium-200", 200},
		{"large-500", 500},
		{"xlarge-1000", 1000},
	}

	for _, sz := range sizes {
		t.Run(sz.name, func(t *testing.T) {
			page := generateLargeDOMPage(sz.elems)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(page))
			}))
			defer ts.Close()

			const iterations = 20
			var totalNav, totalSnap int64

			for i := 0; i < iterations; i++ {
				lite := NewLiteEngine()
				ctx := context.Background()

				navStart := time.Now()
				_, err := lite.Navigate(ctx, ts.URL)
				navElapsed := time.Since(navStart).Nanoseconds()
				if err != nil {
					_ = lite.Close()
					t.Fatalf("navigate: %v", err)
				}
				totalNav += navElapsed

				snapStart := time.Now()
				nodes, _ := lite.Snapshot(ctx, "")
				totalSnap += time.Since(snapStart).Nanoseconds()

				_ = lite.Close()

				if i == 0 {
					t.Logf("page: %d elements, %d snapshot nodes, %d bytes HTML",
						sz.elems, len(nodes), len(page))
				}
			}

			t.Logf("avg navigate=%s snapshot=%s total=%s (%d iterations)",
				fmtDuration(totalNav/int64(iterations)),
				fmtDuration(totalSnap/int64(iterations)),
				fmtDuration((totalNav+totalSnap)/int64(iterations)),
				iterations)
		})
	}
}

// TestEngineBenchmark_TokenEfficiency measures how many tokens each engine
// produces per interactive element — the key metric for LLM agent cost.
func TestEngineBenchmark_TokenEfficiency(t *testing.T) {
	pages := benchPages()

	type tokenResult struct {
		page           string
		totalNodes     int
		interNodes     int
		totalTokens    int
		interTokens    int
		tokensPerInter float64
	}

	var results []tokenResult

	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(page.html))
			}))
			defer ts.Close()

			lite := NewLiteEngine()
			defer func() { _ = lite.Close() }()

			ctx := context.Background()
			_, err := lite.Navigate(ctx, ts.URL)
			if err != nil {
				t.Fatalf("navigate: %v", err)
			}

			allNodes, _ := lite.Snapshot(ctx, "")
			interNodes, _ := lite.Snapshot(ctx, "interactive")

			totalTok := estimateTokens(allNodes)
			interTok := estimateTokens(interNodes)

			var tpi float64
			if len(interNodes) > 0 {
				tpi = float64(interTok) / float64(len(interNodes))
			}

			tr := tokenResult{
				page:           page.name,
				totalNodes:     len(allNodes),
				interNodes:     len(interNodes),
				totalTokens:    totalTok,
				interTokens:    interTok,
				tokensPerInter: tpi,
			}
			results = append(results, tr)

			// Also measure raw JSON size for comparison.
			allJSON, _ := json.Marshal(allNodes)
			interJSON, _ := json.Marshal(interNodes)

			t.Logf("nodes: %d total / %d interactive", len(allNodes), len(interNodes))
			t.Logf("tokens: %d total / %d interactive (%.1f per interactive elem)",
				totalTok, interTok, tpi)
			t.Logf("json: %d bytes total / %d bytes interactive",
				len(allJSON), len(interJSON))
		})
	}

	if len(results) > 0 {
		t.Log("\n=== Token Efficiency Summary ===")
		t.Logf("%-20s %8s %8s %8s %8s %10s", "Page", "Nodes", "Inter", "Tokens", "IntTok", "Tok/Inter")
		t.Logf("%-20s %8s %8s %8s %8s %10s", "----", "-----", "-----", "------", "------", "---------")
		for _, r := range results {
			t.Logf("%-20s %8d %8d %8d %8d %10.1f",
				r.page, r.totalNodes, r.interNodes, r.totalTokens, r.interTokens, r.tokensPerInter)
		}
	}
}

// -----------------------------------------------------------------------
// Output formatting
// -----------------------------------------------------------------------

func printBenchSummary(t *testing.T, results []benchResult) {
	t.Helper()

	// Group by engine and build lookup.
	byEngine := make(map[string][]benchResult)
	enginePageMap := make(map[string]map[string]benchResult)
	for _, r := range results {
		byEngine[r.engine] = append(byEngine[r.engine], r)
		if enginePageMap[r.engine] == nil {
			enginePageMap[r.engine] = make(map[string]benchResult)
		}
		enginePageMap[r.engine][r.page] = r
	}

	var engineNames []string
	for name := range byEngine {
		engineNames = append(engineNames, name)
	}
	sort.Strings(engineNames)

	// Collect ordered page list from first engine.
	var pages []string
	for _, r := range byEngine[engineNames[0]] {
		pages = append(pages, r.page)
	}

	// ── Table 1: Per-Page Response Times ──
	// Matches #201 format: | Page | Category | Engine1 Nav | Engine2 Nav | ... |
	t.Log("")
	t.Log("### Per-Page Response Times")
	t.Log("")

	// Build header.
	header := "| Page | Category"
	sep := "|------|----------"
	for _, e := range engineNames {
		header += fmt.Sprintf(" | %s Nav | %s Snap", e, e)
		sep += "|:--------:|:---------:"
	}
	header += " |"
	sep += "|"
	t.Log(header)
	t.Log(sep)

	for _, pg := range pages {
		cat := pageCategory(pg)
		line := fmt.Sprintf("| %s | %s", pg, cat)
		for _, e := range engineNames {
			r, ok := enginePageMap[e][pg]
			if !ok || r.err != "" {
				line += " | FAIL | —"
			} else {
				line += fmt.Sprintf(" | %s | %s", fmtDuration(r.navigateNs), fmtDuration(r.snapshotNs))
			}
		}
		line += " |"
		t.Log(line)
	}

	// ── Table 2: Aggregate Totals ──
	t.Log("")
	t.Log("### Aggregate Totals")
	t.Log("")

	header2 := "| Metric"
	sep2 := "|--------"
	for _, e := range engineNames {
		header2 += fmt.Sprintf(" | %s", e)
		sep2 += "|----------:"
	}
	header2 += " |"
	sep2 += "|"
	t.Log(header2)
	t.Log(sep2)

	for _, metric := range []string{"Navigate Total", "Snapshot Total", "**Grand Total**"} {
		line := fmt.Sprintf("| %s", metric)
		for _, e := range engineNames {
			var total int64
			ok := 0
			for _, r := range byEngine[e] {
				if r.err != "" {
					continue
				}
				ok++
				switch metric {
				case "Navigate Total":
					total += r.navigateNs
				case "Snapshot Total":
					total += r.snapshotNs
				default:
					total += r.totalNs
				}
			}
			if ok == 0 {
				line += " | —"
			} else {
				line += fmt.Sprintf(" | %s", fmtDuration(total))
			}
		}
		line += " |"
		t.Log(line)
	}

	// ── Table 3: Per-Page Winners ──
	if len(engineNames) > 1 {
		t.Log("")
		t.Log("### Per-Page Winners")
		t.Log("")
		t.Log("| Page | Winner | Why |")
		t.Log("|------|--------|-----|")

		for _, pg := range pages {
			bestEngine := ""
			var bestTime int64 = 1<<62 - 1
			for _, e := range engineNames {
				r, ok := enginePageMap[e][pg]
				if !ok || r.err != "" {
					continue
				}
				if r.totalNs < bestTime {
					bestTime = r.totalNs
					bestEngine = e
				}
			}
			if bestEngine == "" {
				t.Logf("| %s | — | All engines failed |", pg)
			} else {
				r := enginePageMap[bestEngine][pg]
				t.Logf("| %s | **%s** (%s) | %d nodes, ~%d tokens |",
					pg, bestEngine, fmtDuration(r.totalNs), r.nodeCount, r.estTokens)
			}
		}
	}

	// ── Table 4: Token Efficiency (always useful) ──
	t.Log("")
	t.Log("### Token Efficiency")
	t.Log("")
	t.Log("| Page | Nodes | Interactive | ~Tokens | Tok/Interactive |")
	t.Log("|------|------:|------------:|--------:|----------------:|")

	// Use first engine for token counts (all engines see the same HTML).
	for _, pg := range pages {
		r, ok := enginePageMap[engineNames[0]][pg]
		if !ok || r.err != "" {
			continue
		}
		var tpi float64
		if r.interCount > 0 {
			tpi = float64(r.estTokens) / float64(r.interCount)
		}
		t.Logf("| %s | %d | %d | %d | %.1f |",
			pg, r.nodeCount, r.interCount, r.estTokens, tpi)
	}
}

func pageCategory(name string) string {
	switch name {
	case "static-article":
		return "Content"
	case "form-heavy":
		return "Forms"
	case "deep-nesting":
		return "Navigation"
	case "large-dom":
		return "Stress"
	case "spa-shell":
		return "SPA"
	case "data-table":
		return "Data"
	case "dashboard":
		return "Dashboard"
	case "ecommerce":
		return "E-commerce"
	default:
		return "Other"
	}
}

func fmtDuration(ns int64) string {
	d := time.Duration(ns)
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", ns)
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(ns)/1000)
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(ns)/1e6)
	default:
		return fmt.Sprintf("%.2fs", float64(ns)/1e9)
	}
}

// -----------------------------------------------------------------------
// Test pages — HTML content
// -----------------------------------------------------------------------

const staticArticlePage = `<!DOCTYPE html>
<html lang="en">
<head><title>Understanding Neural Networks</title></head>
<body>
<header><nav aria-label="Main">
  <a href="/">Home</a>
  <a href="/articles">Articles</a>
  <a href="/about">About</a>
  <a href="/contact">Contact</a>
</nav></header>
<main>
  <article>
    <h1>Understanding Neural Networks</h1>
    <p>Published: March 2026 | Author: Dr. Smith</p>
    <h2>Introduction</h2>
    <p>Neural networks are computational models inspired by the human brain.
       They consist of interconnected nodes organized in layers that process
       information using connectionist approaches to computation.</p>
    <h2>Architecture</h2>
    <p>A typical neural network consists of an input layer, one or more hidden
       layers, and an output layer. Each connection between neurons has an
       associated weight that is adjusted during training.</p>
    <figure>
      <img src="/nn-diagram.png" alt="Neural network architecture diagram showing layers">
    </figure>
    <h2>Training Process</h2>
    <p>Training involves forward propagation, loss calculation, and
       backpropagation. The network adjusts its weights to minimize the
       difference between predicted and actual outputs.</p>
    <blockquote>The key insight is that gradient descent on differentiable
       functions allows us to optimize complex models end-to-end.</blockquote>
    <h2>Applications</h2>
    <ul>
      <li>Computer vision and image recognition</li>
      <li>Natural language processing</li>
      <li>Speech recognition and synthesis</li>
      <li>Autonomous vehicles</li>
      <li>Drug discovery</li>
    </ul>
    <h2>Further Reading</h2>
    <p>For more information, see our <a href="/deep-learning">deep learning guide</a>
       or <a href="/resources">resources page</a>.</p>
  </article>
</main>
<footer><p>© 2026 Tech Blog</p></footer>
</body>
</html>`

const benchFormHeavyPage = `<!DOCTYPE html>
<html lang="en">
<head><title>Patient Registration</title></head>
<body>
<main>
  <h1>Patient Registration Form</h1>
  <form action="/register" method="post">
    <fieldset>
      <legend>Personal Information</legend>
      <label for="fname">First Name</label>
      <input type="text" id="fname" name="fname" placeholder="John" required>
      <label for="lname">Last Name</label>
      <input type="text" id="lname" name="lname" placeholder="Doe" required>
      <label for="dob">Date of Birth</label>
      <input type="date" id="dob" name="dob" required>
      <label for="gender">Gender</label>
      <select id="gender" name="gender">
        <option value="">Select...</option>
        <option value="m">Male</option>
        <option value="f">Female</option>
        <option value="o">Other</option>
        <option value="na">Prefer not to say</option>
      </select>
      <label for="email">Email</label>
      <input type="email" id="email" name="email" placeholder="john@example.com">
      <label for="phone">Phone</label>
      <input type="tel" id="phone" name="phone" placeholder="+1 555-0100">
    </fieldset>
    <fieldset>
      <legend>Address</legend>
      <label for="addr1">Street Address</label>
      <input type="text" id="addr1" name="addr1" placeholder="123 Main St">
      <label for="addr2">Apt/Suite</label>
      <input type="text" id="addr2" name="addr2" placeholder="Apt 4B">
      <label for="city">City</label>
      <input type="text" id="city" name="city" placeholder="Springfield">
      <label for="state">State</label>
      <select id="state" name="state">
        <option value="">Select state...</option>
        <option value="CA">California</option>
        <option value="NY">New York</option>
        <option value="TX">Texas</option>
        <option value="FL">Florida</option>
      </select>
      <label for="zip">ZIP Code</label>
      <input type="text" id="zip" name="zip" placeholder="62701" pattern="[0-9]{5}">
    </fieldset>
    <fieldset>
      <legend>Insurance</legend>
      <label for="provider">Insurance Provider</label>
      <input type="text" id="provider" name="provider">
      <label for="policy">Policy Number</label>
      <input type="text" id="policy" name="policy">
      <label for="group">Group Number</label>
      <input type="text" id="group" name="group">
    </fieldset>
    <fieldset>
      <legend>Medical History</legend>
      <label>Do you have any allergies?</label>
      <input type="radio" id="allergy-yes" name="allergies" value="yes"><label for="allergy-yes">Yes</label>
      <input type="radio" id="allergy-no" name="allergies" value="no"><label for="allergy-no">No</label>
      <label for="allergy-details">If yes, please list</label>
      <textarea id="allergy-details" name="allergy-details" rows="3" placeholder="List allergies..."></textarea>
      <label><input type="checkbox" name="conditions[]" value="diabetes"> Diabetes</label>
      <label><input type="checkbox" name="conditions[]" value="hypertension"> Hypertension</label>
      <label><input type="checkbox" name="conditions[]" value="asthma"> Asthma</label>
      <label><input type="checkbox" name="conditions[]" value="heart"> Heart Disease</label>
    </fieldset>
    <fieldset>
      <legend>Consent</legend>
      <label><input type="checkbox" name="consent" required> I agree to the terms and conditions</label>
      <label><input type="checkbox" name="hipaa" required> I acknowledge the HIPAA notice</label>
    </fieldset>
    <button type="submit">Register Patient</button>
    <button type="reset">Clear Form</button>
  </form>
</main>
</body>
</html>`

const deepNestingPage = `<!DOCTYPE html>
<html lang="en">
<head><title>Project Dashboard</title></head>
<body>
<div role="banner">
  <nav aria-label="Primary">
    <ul>
      <li><a href="/dashboard">Dashboard</a></li>
      <li><a href="/projects">Projects</a>
        <ul>
          <li><a href="/projects/active">Active</a></li>
          <li><a href="/projects/archived">Archived</a>
            <ul>
              <li><a href="/projects/archived/2025">2025</a></li>
              <li><a href="/projects/archived/2024">2024</a></li>
            </ul>
          </li>
        </ul>
      </li>
      <li><a href="/team">Team</a></li>
      <li><a href="/settings">Settings</a></li>
    </ul>
  </nav>
</div>
<main>
  <section aria-label="Project Overview">
    <h1>Project Dashboard</h1>
    <div>
      <div>
        <div>
          <div>
            <h2>Sprint 42</h2>
            <div>
              <div>
                <p>Status: <strong>In Progress</strong></p>
                <div>
                  <button>Start Sprint</button>
                  <button>End Sprint</button>
                  <a href="/sprint/42/board">View Board</a>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <section aria-label="Tasks">
      <h2>Current Tasks</h2>
      <table>
        <thead><tr><th>Task</th><th>Assignee</th><th>Status</th><th>Actions</th></tr></thead>
        <tbody>
          <tr><td>Fix login bug</td><td>Alice</td><td>In Progress</td><td><button>Edit</button><button>Delete</button></td></tr>
          <tr><td>Add search feature</td><td>Bob</td><td>To Do</td><td><button>Edit</button><button>Delete</button></td></tr>
          <tr><td>Update docs</td><td>Carol</td><td>Done</td><td><button>Edit</button><button>Delete</button></td></tr>
          <tr><td>Performance audit</td><td>Dave</td><td>Blocked</td><td><button>Edit</button><button>Delete</button></td></tr>
          <tr><td>Accessibility review</td><td>Eve</td><td>To Do</td><td><button>Edit</button><button>Delete</button></td></tr>
        </tbody>
      </table>
    </section>
  </section>
</main>
<footer><p>© 2026 ProjectCo</p></footer>
</body>
</html>`

const spaShellPage = `<!DOCTYPE html>
<html lang="en">
<head><title>App Dashboard</title></head>
<body>
<div id="app">
  <header>
    <nav aria-label="Main Navigation">
      <a href="/" aria-label="Home">Home</a>
      <a href="/inbox" aria-label="Inbox">Inbox <span aria-label="3 unread">3</span></a>
      <a href="/analytics" aria-label="Analytics">Analytics</a>
      <button aria-label="User menu">Profile</button>
    </nav>
    <div role="search">
      <input type="search" placeholder="Search everything..." aria-label="Global search">
      <button type="submit">Search</button>
    </div>
  </header>
  <aside aria-label="Sidebar">
    <nav aria-label="Sidebar Navigation">
      <ul>
        <li><a href="/inbox/all">All Messages</a></li>
        <li><a href="/inbox/unread">Unread</a></li>
        <li><a href="/inbox/starred">Starred</a></li>
        <li><a href="/inbox/archived">Archived</a></li>
        <li><a href="/inbox/trash">Trash</a></li>
      </ul>
    </nav>
    <div>
      <h3>Labels</h3>
      <ul>
        <li><a href="/label/work">Work</a></li>
        <li><a href="/label/personal">Personal</a></li>
        <li><a href="/label/urgent">Urgent</a></li>
      </ul>
      <button>Create Label</button>
    </div>
  </aside>
  <main>
    <h1>Inbox</h1>
    <div role="toolbar" aria-label="Message actions">
      <input type="checkbox" aria-label="Select all">
      <button>Archive</button>
      <button>Mark Read</button>
      <button>Delete</button>
      <select aria-label="Sort by">
        <option>Newest</option>
        <option>Oldest</option>
        <option>Unread</option>
      </select>
    </div>
    <ul role="list" aria-label="Messages">
      <li role="listitem"><input type="checkbox" aria-label="Select message"><a href="/msg/1"><strong>Alice:</strong> Project update ready for review</a></li>
      <li role="listitem"><input type="checkbox" aria-label="Select message"><a href="/msg/2"><strong>Bob:</strong> Deployment scheduled for tonight</a></li>
      <li role="listitem"><input type="checkbox" aria-label="Select message"><a href="/msg/3"><strong>Carol:</strong> Q4 report attached</a></li>
      <li role="listitem"><input type="checkbox" aria-label="Select message"><a href="/msg/4"><strong>Dave:</strong> Meeting notes from standup</a></li>
      <li role="listitem"><input type="checkbox" aria-label="Select message"><a href="/msg/5"><strong>Eve:</strong> Security audit findings</a></li>
    </ul>
    <nav aria-label="Pagination">
      <button disabled>Previous</button>
      <span>Page 1 of 5</span>
      <button>Next</button>
    </nav>
  </main>
</div>
</body>
</html>`

const dataTablePage = `<!DOCTYPE html>
<html lang="en">
<head><title>Server Monitoring</title></head>
<body>
<main>
  <h1>Server Status Dashboard</h1>
  <div role="toolbar" aria-label="Table controls">
    <input type="search" placeholder="Filter servers..." aria-label="Filter">
    <select aria-label="Region filter">
      <option value="">All Regions</option>
      <option value="us-east">US East</option>
      <option value="us-west">US West</option>
      <option value="eu-west">EU West</option>
      <option value="ap-south">AP South</option>
    </select>
    <button>Refresh</button>
    <button>Export CSV</button>
  </div>
  <table aria-label="Server list">
    <thead>
      <tr>
        <th><input type="checkbox" aria-label="Select all servers"></th>
        <th><button>Hostname ↕</button></th>
        <th><button>Region ↕</button></th>
        <th><button>CPU % ↕</button></th>
        <th><button>Memory ↕</button></th>
        <th><button>Status ↕</button></th>
        <th>Actions</th>
      </tr>
    </thead>
    <tbody>
      <tr><td><input type="checkbox" aria-label="Select web-01"></td><td>web-01</td><td>us-east</td><td>45%</td><td>72%</td><td>Healthy</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select web-02"></td><td>web-02</td><td>us-east</td><td>78%</td><td>85%</td><td>Warning</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select api-01"></td><td>api-01</td><td>us-west</td><td>23%</td><td>41%</td><td>Healthy</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select api-02"></td><td>api-02</td><td>eu-west</td><td>92%</td><td>94%</td><td>Critical</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select db-01"></td><td>db-01</td><td>us-east</td><td>31%</td><td>58%</td><td>Healthy</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select db-02"></td><td>db-02</td><td>ap-south</td><td>55%</td><td>67%</td><td>Healthy</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select cache-01"></td><td>cache-01</td><td>us-east</td><td>12%</td><td>89%</td><td>Warning</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
      <tr><td><input type="checkbox" aria-label="Select worker-01"></td><td>worker-01</td><td>us-west</td><td>67%</td><td>52%</td><td>Healthy</td><td><button>SSH</button><button>Restart</button><button>Details</button></td></tr>
    </tbody>
  </table>
  <nav aria-label="Table pagination">
    <button disabled>« First</button>
    <button disabled>‹ Prev</button>
    <span>Showing 1-8 of 24 servers</span>
    <button>Next ›</button>
    <button>Last »</button>
  </nav>
</main>
</body>
</html>`

const dashboardPage = `<!DOCTYPE html>
<html lang="en">
<head><title>Analytics Dashboard</title></head>
<body>
<header>
  <nav aria-label="Top navigation">
    <a href="/">Logo</a>
    <a href="/dashboard">Dashboard</a>
    <a href="/reports">Reports</a>
    <a href="/settings">Settings</a>
    <button aria-label="Notifications">🔔 5</button>
    <button aria-label="User account">Account</button>
  </nav>
</header>
<main>
  <h1>Analytics Dashboard</h1>
  <div role="toolbar" aria-label="Dashboard controls">
    <select aria-label="Date range">
      <option>Last 7 days</option>
      <option>Last 30 days</option>
      <option>Last 90 days</option>
      <option>Custom range</option>
    </select>
    <button>Apply Filter</button>
    <button>Download Report</button>
  </div>
  <section aria-label="Key Metrics">
    <h2>Key Metrics</h2>
    <div>
      <div aria-label="Revenue card"><h3>Revenue</h3><p>$142,384</p><p>+12.5% vs last period</p></div>
      <div aria-label="Users card"><h3>Active Users</h3><p>23,847</p><p>+8.3% vs last period</p></div>
      <div aria-label="Conversion card"><h3>Conversion Rate</h3><p>3.24%</p><p>-0.5% vs last period</p></div>
      <div aria-label="Sessions card"><h3>Avg. Session</h3><p>4m 32s</p><p>+15s vs last period</p></div>
    </div>
  </section>
  <section aria-label="Charts">
    <h2>Traffic Overview</h2>
    <div aria-label="Traffic chart placeholder">
      <p>[Chart: Daily traffic for last 30 days]</p>
    </div>
    <div role="toolbar" aria-label="Chart controls">
      <button>Daily</button>
      <button>Weekly</button>
      <button>Monthly</button>
    </div>
  </section>
  <section aria-label="Top Pages">
    <h2>Top Pages</h2>
    <table>
      <thead><tr><th>Page</th><th>Views</th><th>Bounce Rate</th><th>Avg Time</th></tr></thead>
      <tbody>
        <tr><td>/home</td><td>45,230</td><td>32%</td><td>2m 15s</td></tr>
        <tr><td>/products</td><td>28,450</td><td>45%</td><td>3m 42s</td></tr>
        <tr><td>/pricing</td><td>15,820</td><td>28%</td><td>4m 10s</td></tr>
        <tr><td>/blog</td><td>12,350</td><td>52%</td><td>5m 30s</td></tr>
        <tr><td>/contact</td><td>8,920</td><td>18%</td><td>1m 45s</td></tr>
      </tbody>
    </table>
  </section>
</main>
<footer><p>© 2026 Analytics Corp</p></footer>
</body>
</html>`

const ecommercePage = `<!DOCTYPE html>
<html lang="en">
<head><title>Wireless Headphones - TechStore</title></head>
<body>
<header>
  <nav aria-label="Store navigation">
    <a href="/">TechStore</a>
    <a href="/electronics">Electronics</a>
    <a href="/audio">Audio</a>
    <a href="/deals">Deals</a>
    <input type="search" placeholder="Search products..." aria-label="Search products">
    <button type="submit">Search</button>
    <a href="/cart" aria-label="Shopping cart (2 items)">Cart (2)</a>
    <a href="/account">Sign In</a>
  </nav>
</header>
<nav aria-label="Breadcrumb">
  <a href="/">Home</a> › <a href="/electronics">Electronics</a> › <a href="/audio">Audio</a> › Headphones
</nav>
<main>
  <article>
    <h1>Premium Wireless Headphones XR-500</h1>
    <img src="/headphones.jpg" alt="Premium Wireless Headphones XR-500 in midnight black">
    <section aria-label="Product details">
      <p>$299.99 <del>$349.99</del> <span>Save 14%</span></p>
      <div>
        <label for="color">Color</label>
        <select id="color" name="color">
          <option>Midnight Black</option>
          <option>Arctic White</option>
          <option>Navy Blue</option>
          <option>Rose Gold</option>
        </select>
      </div>
      <div>
        <label for="qty">Quantity</label>
        <input type="number" id="qty" name="qty" value="1" min="1" max="10">
      </div>
      <button>Add to Cart</button>
      <button>Buy Now</button>
      <button aria-label="Add to wishlist">♡ Wishlist</button>
    </section>
    <details>
      <summary>Product Description</summary>
      <p>Experience premium sound with our XR-500 wireless headphones.
         Features include 40-hour battery life, active noise cancellation,
         and premium memory foam ear cushions for all-day comfort.</p>
      <ul>
        <li>40-hour battery life</li>
        <li>Active noise cancellation (ANC)</li>
        <li>Bluetooth 5.3</li>
        <li>Hi-Res Audio certified</li>
        <li>Multipoint connection</li>
      </ul>
    </details>
    <details>
      <summary>Specifications</summary>
      <table>
        <tr><th>Driver Size</th><td>40mm</td></tr>
        <tr><th>Frequency Response</th><td>4Hz - 40kHz</td></tr>
        <tr><th>Impedance</th><td>32Ω</td></tr>
        <tr><th>Weight</th><td>250g</td></tr>
        <tr><th>Charging</th><td>USB-C, 10min = 5hrs</td></tr>
      </table>
    </details>
    <section aria-label="Customer Reviews">
      <h2>Customer Reviews</h2>
      <p>4.7 out of 5 (2,847 reviews)</p>
      <article>
        <h3>Best headphones I've owned</h3>
        <p>★★★★★ by AudioPhile23 on March 1, 2026</p>
        <p>The noise cancellation is incredible and the battery life is amazing.</p>
        <button>Helpful (42)</button>
        <button>Report</button>
      </article>
      <article>
        <h3>Great value for money</h3>
        <p>★★★★☆ by MusicLover on Feb 28, 2026</p>
        <p>Sound quality rivals headphones twice the price. Only downside is the carrying case.</p>
        <button>Helpful (18)</button>
        <button>Report</button>
      </article>
      <a href="/reviews/xr500">See all 2,847 reviews</a>
    </section>
  </article>
</main>
<footer>
  <nav aria-label="Footer">
    <a href="/about">About</a>
    <a href="/support">Support</a>
    <a href="/returns">Returns</a>
    <a href="/privacy">Privacy</a>
    <a href="/terms">Terms</a>
  </nav>
</footer>
</body>
</html>`

// generateLargeDOMPage creates a page with N interactive elements.
func generateLargeDOMPage(n int) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><title>Large Page</title></head><body>`)
	b.WriteString(`<main><h1>Large Form</h1><form>`)

	for i := 0; i < n; i++ {
		switch i % 5 {
		case 0:
			fmt.Fprintf(&b, `<div><label for="f%d">Field %d</label><input type="text" id="f%d" placeholder="Value %d"></div>`, i, i, i, i)
		case 1:
			fmt.Fprintf(&b, `<button type="button">Action %d</button>`, i)
		case 2:
			fmt.Fprintf(&b, `<a href="/page/%d">Link %d</a>`, i, i)
		case 3:
			fmt.Fprintf(&b, `<select id="s%d" aria-label="Select %d"><option>A</option><option>B</option></select>`, i, i)
		case 4:
			fmt.Fprintf(&b, `<textarea id="t%d" placeholder="Note %d"></textarea>`, i, i)
		}
	}

	b.WriteString(`</form></main></body></html>`)
	return b.String()
}
