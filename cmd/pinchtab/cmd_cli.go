package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func printHelp() {
	fmt.Printf(`pinchtab %s - Browser control for AI agents

MODES:
  pinchtab                 Start server (default port 9867)
  pinchtab connect <name>  Get URL for a running profile instance

BROWSER COMMANDS:
  pinchtab nav <url>              Navigate to URL
  pinchtab snap [url]             Accessibility snapshot (-i interactive, -c compact)
  pinchtab find <query> [--url u] Semantic element search (--top N, --threshold F)
  pinchtab text [url]             Extract readable text
  pinchtab screenshot [url]       Capture screenshot (--out file)
  pinchtab pdf [url]              Export PDF (--out file)
  pinchtab click <ref>            Click element by ref
  pinchtab type <ref> <text>      Type text into element
  pinchtab fill <ref> <text>      Fill/replace input value
  pinchtab press <key>            Press keyboard key (Enter, Tab, Escape, ...)
  pinchtab hover <ref>            Hover over element
  pinchtab scroll [ref]           Scroll element into view (or page)
  pinchtab select <ref> <value>   Select dropdown option
  pinchtab eval <expression>      Evaluate JavaScript

MANAGEMENT COMMANDS:
  pinchtab health        Server health check
  pinchtab profiles      List available profiles
  pinchtab instances     List running instances
  pinchtab tabs          List open tabs (all instances)
  pinchtab config init   Initialize config file
  pinchtab config show   Display current configuration
  pinchtab help          Show this help

ENVIRONMENT:
  PINCHTAB_URL    Server URL (default: http://127.0.0.1:9867)
  PINCHTAB_TOKEN  Auth token for API requests
`, version)
}

var cliCommands = map[string]bool{
	"health":     true,
	"help":       true,
	"config":     true,
	"profiles":   true,
	"instances":  true,
	"tabs":       true,
	"connect":    true,
	"nav":        true,
	"navigate":   true,
	"snap":       true,
	"snapshot":   true,
	"find":       true,
	"text":       true,
	"screenshot": true,
	"ss":         true,
	"pdf":        true,
	"click":      true,
	"type":       true,
	"fill":       true,
	"press":      true,
	"hover":      true,
	"scroll":     true,
	"select":     true,
	"eval":       true,
	"evaluate":   true,
}

func isCLICommand(cmd string) bool {
	return cliCommands[cmd]
}

func runCLI(cfg *config.RuntimeConfig) {
	cmd := os.Args[1]

	base := fmt.Sprintf("http://%s:%s", cfg.Bind, cfg.Port)
	if envURL := os.Getenv("PINCHTAB_URL"); envURL != "" {
		base = strings.TrimRight(envURL, "/")
	}

	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}

	client := &http.Client{Timeout: 30 * time.Second}

	args := os.Args[2:]

	switch cmd {
	case "health":
		cliHealth(client, base, token)
	case "profiles":
		cliProfiles(client, base, token)
	case "instances":
		cliInstances(client, base, token)
	case "tabs":
		cliTabs(client, base, token)
	case "help":
		printHelp()
	case "config":
		// Handled in main.go
	case "nav", "navigate":
		cliNavigate(client, base, token, args)
	case "snap", "snapshot":
		cliSnapshot(client, base, token, args)
	case "find":
		cliFind(client, base, token, args)
	case "text":
		cliText(client, base, token, args)
	case "screenshot", "ss":
		cliScreenshot(client, base, token, args)
	case "pdf":
		cliPDF(client, base, token, args)
	case "click", "hover", "scroll", "press", "type", "fill", "select":
		cliAction(client, base, token, cmd, args)
	case "eval", "evaluate":
		cliEval(client, base, token, args)
	}
}

// --- health ---

func cliHealth(client *http.Client, base, token string) {
	result := doGet(client, base, token, "/health", nil)
	if status, ok := result["status"].(string); ok && status == "ok" {
		fmt.Println("✅ Server is healthy")
		if strategy, ok := result["strategy"].(string); ok {
			fmt.Printf("   Strategy: %s\n", strategy)
		}
	} else {
		fmt.Println("❌ Server health check failed")
		os.Exit(1)
	}
}

// --- profiles ---

func cliProfiles(client *http.Client, base, token string) {
	result := doGet(client, base, token, "/profiles", nil)

	if profiles, ok := result["profiles"].([]interface{}); ok && len(profiles) > 0 {
		fmt.Println("\n📋 Available Profiles:")
		fmt.Println()
		for _, prof := range profiles {
			if m, ok := prof.(map[string]any); ok {
				name, _ := m["name"].(string)
				fmt.Printf("  👤 %s\n", name)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("No profiles available")
	}
}

// --- instances ---

func cliInstances(client *http.Client, base, token string) {
	result := doGet(client, base, token, "/instances", nil)

	if instances, ok := result["instances"].([]interface{}); ok {
		if len(instances) == 0 {
			fmt.Println("No instances running")
			fmt.Println("\nTo launch an instance:")
			fmt.Println("  1. Start dashboard: pinchtab")
			fmt.Println("  2. Open browser: http://localhost:9867/dashboard")
			fmt.Println("  3. Click 'Profiles' → select profile → 'Launch'")
			return
		}

		fmt.Println("\n🚀 Running Instances:")
		fmt.Println()

		for _, inst := range instances {
			if m, ok := inst.(map[string]any); ok {
				id, _ := m["id"].(string)
				port, _ := m["port"].(string)
				status, _ := m["status"].(string)
				headless, _ := m["headless"].(bool)

				mode := "headless"
				if !headless {
					mode = "headed"
				}

				icon := "▶️"
				if status != "running" {
					icon = "⏸️"
				}

				fmt.Printf("  %s %s (port %s, %s)\n", icon, id, port, mode)
				if port != "" && status == "running" {
					fmt.Printf("     → http://localhost:%s\n", port)
				}
			}
		}
		fmt.Println()
	} else {
		fmt.Println("Failed to get instances")
		os.Exit(1)
	}
}

// --- tabs ---

func cliTabs(client *http.Client, base, token string) {
	result := doGet(client, base, token, "/tabs", nil)

	if tabs, ok := result["tabs"].([]interface{}); ok {
		if len(tabs) == 0 {
			fmt.Println("No tabs open across all instances")
			return
		}

		fmt.Println("\n📑 Open Tabs:")
		fmt.Println()

		for _, tab := range tabs {
			if m, ok := tab.(map[string]any); ok {
				id, _ := m["id"].(string)
				url, _ := m["url"].(string)
				title, _ := m["title"].(string)

				if title == "" {
					title = "(untitled)"
				}

				fmt.Printf("  [%s] %s\n", id, title)
				fmt.Printf("       %s\n", url)
			}
		}
		fmt.Println()
	}
}

// --- browser commands ---

func cliNavigate(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab nav <url>")
		os.Exit(1)
	}
	result := doPost(client, base, token, "/navigate", map[string]any{"url": args[0]})
	if tabID, ok := result["tabId"].(string); ok {
		fmt.Printf("Navigated [%s] → %s\n", tabID, args[0])
	}
}

func cliSnapshot(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-i", "--interactive":
			params.Set("filter", "interactive")
		case "-c", "--compact":
			params.Set("compact", "true")
		case "--text":
			params.Set("format", "text")
		case "-d", "--depth":
			if i+1 < len(args) {
				i++
				params.Set("depth", args[i])
			}
		default:
			// Positional arg = URL
			if strings.HasPrefix(args[i], "http") {
				params.Set("url", args[i])
			}
		}
	}
	result := doGet(client, base, token, "/snapshot", params)
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
}

func cliFind(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab find <query> [--url <url>] [--top N] [--threshold F]")
		os.Exit(1)
	}

	body := map[string]any{"query": args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--url":
			if i+1 < len(args) {
				i++
				body["url"] = args[i]
			}
		case "--top":
			if i+1 < len(args) {
				i++
				if n, err := strconv.Atoi(args[i]); err == nil {
					body["topK"] = n
				}
			}
		case "--threshold":
			if i+1 < len(args) {
				i++
				if f, err := strconv.ParseFloat(args[i], 64); err == nil {
					body["threshold"] = f
				}
			}
		}
	}

	result := doPost(client, base, token, "/find", body)

	// Pretty print matches
	if matches, ok := result["matches"].([]any); ok {
		if len(matches) == 0 {
			fmt.Println("No matches found")
			return
		}
		for _, m := range matches {
			if entry, ok := m.(map[string]any); ok {
				ref, _ := entry["ref"].(string)
				name, _ := entry["name"].(string)
				role, _ := entry["role"].(string)
				score, _ := entry["score"].(float64)
				fmt.Printf("  [%s] %.2f  %s: %s\n", ref, score, role, name)
			}
		}
		if best, ok := result["best_ref"].(string); ok {
			fmt.Printf("\nBest: %s\n", best)
		}
	}
}

func cliText(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "http") {
			params.Set("url", arg)
		}
	}
	result := doGet(client, base, token, "/text", params)
	if text, ok := result["text"].(string); ok {
		fmt.Println(text)
	} else {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	}
}

func cliScreenshot(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	outFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out":
			if i+1 < len(args) {
				i++
				outFile = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "http") {
				params.Set("url", args[i])
			}
		}
	}
	if outFile == "" {
		outFile = "screenshot.jpg"
	}
	params.Set("output", "raw")

	rawBody := doGetRaw(client, base, token, "/screenshot", params)
	if err := os.WriteFile(outFile, rawBody, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Screenshot saved to %s (%d bytes)\n", outFile, len(rawBody))
}

func cliPDF(client *http.Client, base, token string, args []string) {
	params := url.Values{}
	outFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out":
			if i+1 < len(args) {
				i++
				outFile = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "http") {
				params.Set("url", args[i])
			}
		}
	}
	if outFile == "" {
		outFile = "page.pdf"
	}
	params.Set("raw", "true")

	rawBody := doGetRaw(client, base, token, "/pdf", params)
	if err := os.WriteFile(outFile, rawBody, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("PDF saved to %s (%d bytes)\n", outFile, len(rawBody))
}

func cliAction(client *http.Client, base, token, kind string, args []string) {
	body := map[string]any{"kind": kind}

	switch kind {
	case "click", "hover", "focus":
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: pinchtab %s <ref>\n", kind)
			os.Exit(1)
		}
		body["ref"] = args[0]
	case "type":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: pinchtab type <ref> <text>")
			os.Exit(1)
		}
		body["ref"] = args[0]
		body["text"] = strings.Join(args[1:], " ")
	case "fill":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: pinchtab fill <ref> <text>")
			os.Exit(1)
		}
		body["ref"] = args[0]
		body["text"] = strings.Join(args[1:], " ")
	case "press":
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: pinchtab press <key>")
			os.Exit(1)
		}
		body["key"] = args[0]
	case "scroll":
		if len(args) > 0 {
			body["ref"] = args[0]
		}
	case "select":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: pinchtab select <ref> <value>")
			os.Exit(1)
		}
		body["ref"] = args[0]
		body["value"] = args[1]
	}

	result := doPost(client, base, token, "/action", body)
	if msg, ok := result["status"].(string); ok {
		fmt.Println(msg)
	} else {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	}
}

func cliEval(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab eval <expression>")
		os.Exit(1)
	}
	expr := strings.Join(args, " ")
	result := doPost(client, base, token, "/evaluate", map[string]any{"expression": expr})
	if val, ok := result["result"]; ok {
		out, _ := json.MarshalIndent(val, "", "  ")
		fmt.Println(string(out))
	}
}

// --- helpers ---

func doPost(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Make sure pinchtab server is running on %s\n", base)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("warning: error unmarshaling response: %v", err)
	}
	return result
}

func doGetRaw(client *http.Client, base, token, path string, params url.Values) []byte {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	return body
}

func doGet(client *http.Client, base, token, path string, params url.Values) map[string]any {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Make sure pinchtab server is running on %s\n", base)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("warning: error unmarshaling response: %v", err)
	}
	return result
}
