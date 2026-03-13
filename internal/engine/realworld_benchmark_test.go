package engine

// realworld_benchmark_test.go — Real-world JS-heavy website benchmarks
//
// Tests Lite vs Lightpanda against real public websites that require
// JavaScript execution to render their main content. This validates
// LP's value as a lightweight JS engine on actual production sites.
//
// These tests hit the real internet and require LIGHTPANDA_URL to be set.
// They are gated behind REALWORLD_BENCH=1 to avoid running in CI.
//
// Run:
//   REALWORLD_BENCH=1 LIGHTPANDA_URL=ws://127.0.0.1:19222 \
//     go test ./internal/engine/ -run TestRealWorldBenchmark -v -timeout 120s
//
// LP Docker:
//   docker run --rm --network=host --entrypoint lightpanda \
//     lightpanda/browser:nightly serve --host 127.0.0.1 --port 19222

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

type realWorldSite struct {
	name      string
	url       string
	category  string
	framework string
}

func realWorldSites() []realWorldSite {
	return []realWorldSite{
		{
			name:      "excalidraw",
			url:       "https://excalidraw.com",
			category:  "Drawing tool",
			framework: "React",
		},
		{
			name:      "youtube-music",
			url:       "https://music.youtube.com",
			category:  "Media streaming",
			framework: "Polymer/Lit",
		},
		{
			name:      "notion-templates",
			url:       "https://www.notion.so/templates",
			category:  "Productivity",
			framework: "React",
		},
		{
			name:      "stackblitz",
			url:       "https://stackblitz.com",
			category:  "Dev tools",
			framework: "Angular",
		},
		{
			name:      "weather",
			url:       "https://weather.com",
			category:  "Weather/info",
			framework: "React/Next.js",
		},
	}
}

type realWorldResult struct {
	engine     string
	site       string
	category   string
	framework  string
	navigateMs float64
	snapshotMs float64
	totalMs    float64
	nodeCount  int
	interCount int
	textLen    int
	err        string
}

func TestRealWorldBenchmark(t *testing.T) {
	if os.Getenv("REALWORLD_BENCH") != "1" {
		t.Skip("REALWORLD_BENCH=1 not set — skipping real-world benchmarks")
	}

	lpURL := os.Getenv("LIGHTPANDA_URL")
	if lpURL == "" {
		t.Skip("LIGHTPANDA_URL not set — need LP for real-world comparison")
	}

	sites := realWorldSites()
	var results []realWorldResult

	// --- Lite engine ---
	for _, site := range sites {
		t.Run("lite/"+site.name, func(t *testing.T) {
			lite := NewLiteEngine()
			defer func() { _ = lite.Close() }()

			r := benchRealWorldSite(t, lite, "lite", site)
			results = append(results, r)
		})
	}

	// --- Lightpanda engine ---
	lp, err := NewLightpandaEngine(lpURL)
	if err != nil {
		t.Fatalf("NewLightpandaEngine: %v", err)
	}
	defer func() { _ = lp.Close() }()

	for _, site := range sites {
		t.Run("lightpanda/"+site.name, func(t *testing.T) {
			r := benchRealWorldSite(t, lp, "lightpanda", site)
			results = append(results, r)
		})
	}

	// --- Print results ---
	printRealWorldSummary(t, results, sites)
}

func benchRealWorldSite(t *testing.T, eng Engine, engineName string, site realWorldSite) realWorldResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r := realWorldResult{
		engine:    engineName,
		site:      site.name,
		category:  site.category,
		framework: site.framework,
	}

	// Navigate
	navStart := time.Now()
	_, err := eng.Navigate(ctx, site.url)
	r.navigateMs = float64(time.Since(navStart).Nanoseconds()) / 1e6
	if err != nil {
		r.err = err.Error()
		t.Logf("navigate error: %v", err)
		return r
	}

	// Snapshot (all nodes)
	snapStart := time.Now()
	nodes, err := eng.Snapshot(ctx, "")
	r.snapshotMs = float64(time.Since(snapStart).Nanoseconds()) / 1e6
	if err != nil {
		r.err = err.Error()
		t.Logf("snapshot error: %v", err)
		return r
	}
	r.nodeCount = len(nodes)
	r.totalMs = r.navigateMs + r.snapshotMs

	// Interactive count
	interNodes, _ := eng.Snapshot(ctx, "interactive")
	r.interCount = len(interNodes)

	// Text
	text, _ := eng.Text(ctx)
	r.textLen = len(text)

	t.Logf("nav=%.0fms snap=%.0fms total=%.0fms nodes=%d interactive=%d text=%d",
		r.navigateMs, r.snapshotMs, r.totalMs,
		r.nodeCount, r.interCount, r.textLen)

	return r
}

func printRealWorldSummary(t *testing.T, results []realWorldResult, sites []realWorldSite) {
	t.Helper()

	// Build lookup maps.
	liteMap := make(map[string]realWorldResult)
	lpMap := make(map[string]realWorldResult)
	for _, r := range results {
		switch r.engine {
		case "lite":
			liteMap[r.site] = r
		case "lightpanda":
			lpMap[r.site] = r
		}
	}

	// ── Table 1: JS Rendering Comparison ──
	t.Log("")
	t.Log("### Real-World JS-Heavy Sites: Lite vs Lightpanda")
	t.Log("")
	t.Log("| Site | Category | Framework | Lite Nodes | Lite Interactive | LP Nodes | LP Interactive | LP Text | Winner |")
	t.Log("|------|----------|-----------|:----------:|:----------------:|:--------:|:--------------:|:-------:|--------|")

	for _, site := range sites {
		lr := liteMap[site.name]
		lpr := lpMap[site.name]

		liteNodes := "ERR"
		liteInter := "ERR"
		if lr.err == "" {
			liteNodes = fmt.Sprintf("%d", lr.nodeCount)
			liteInter = fmt.Sprintf("%d", lr.interCount)
		}

		lpNodes := "ERR"
		lpInter := "ERR"
		lpText := "ERR"
		if lpr.err == "" {
			lpNodes = fmt.Sprintf("%d", lpr.nodeCount)
			lpInter = fmt.Sprintf("%d", lpr.interCount)
			lpText = fmt.Sprintf("%d", lpr.textLen)
		}

		winner := "—"
		if lr.err == "" && lpr.err == "" {
			if lpr.nodeCount > lr.nodeCount*2 {
				winner = "**LP**"
			} else if lr.nodeCount > lpr.nodeCount*2 {
				winner = "**Lite**"
			} else if lpr.interCount > lr.interCount {
				winner = "**LP**"
			} else if lr.interCount > lpr.interCount {
				winner = "**Lite**"
			} else {
				winner = "Tie"
			}
		} else if lr.err != "" && lpr.err == "" {
			winner = "**LP**"
		} else if lr.err == "" && lpr.err != "" {
			winner = "**Lite**"
		}

		t.Logf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |",
			site.name, site.category, site.framework,
			liteNodes, liteInter, lpNodes, lpInter, lpText, winner)
	}

	// ── Table 2: Latency Comparison ──
	t.Log("")
	t.Log("### Latency (ms)")
	t.Log("")
	t.Log("| Site | Lite Nav | Lite Snap | Lite Total | LP Nav | LP Snap | LP Total |")
	t.Log("|------|:--------:|:---------:|:----------:|:------:|:-------:|:--------:|")

	for _, site := range sites {
		lr := liteMap[site.name]
		lpr := lpMap[site.name]

		liteNav, liteSnap, liteTotal := "ERR", "ERR", "ERR"
		if lr.err == "" {
			liteNav = fmt.Sprintf("%.0f", lr.navigateMs)
			liteSnap = fmt.Sprintf("%.0f", lr.snapshotMs)
			liteTotal = fmt.Sprintf("%.0f", lr.totalMs)
		}

		lpNav, lpSnap, lpTotal := "ERR", "ERR", "ERR"
		if lpr.err == "" {
			lpNav = fmt.Sprintf("%.0f", lpr.navigateMs)
			lpSnap = fmt.Sprintf("%.0f", lpr.snapshotMs)
			lpTotal = fmt.Sprintf("%.0f", lpr.totalMs)
		}

		t.Logf("| %s | %s | %s | %s | %s | %s | %s |",
			site.name, liteNav, liteSnap, liteTotal, lpNav, lpSnap, lpTotal)
	}
}
