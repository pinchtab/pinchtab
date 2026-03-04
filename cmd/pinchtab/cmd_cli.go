package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func printHelp() {
	fmt.Printf(`pinchtab %s - Browser control orchestrator

MODES:
  pinchtab                 Start server (default port 9867)
  pinchtab connect <name>  Get URL for a running profile instance

MANAGEMENT COMMANDS:
  pinchtab health                    Server health check
  pinchtab profiles                  List available profiles
  pinchtab instances                 List running instances
  pinchtab tabs                      List open tabs (all instances)
  pinchtab config init               Initialize config file
  pinchtab config show               Display current configuration
  pinchtab help                      Show this help

ENVIRONMENT (CLIENT):
  PINCHTAB_URL    Server URL (default: http://127.0.0.1:9867)
  PINCHTAB_TOKEN  Auth token for API requests

BROWSER AUTOMATION:
  Use HTTP API directly or client libraries (Playwright, Puppeteer, Cypress)

Examples:
  pinchtab                              # Start server
  pinchtab health                       # Check status
  pinchtab connect work                 # Get instance URL
  curl http://localhost:9867/snapshot   # HTTP API
`, version)
}

var cliCommands = map[string]bool{
	"health":    true,
	"help":      true,
	"config":    true,
	"profiles":  true,
	"instances": true,
	"tabs":      true,
	"connect":   true,
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
	}
}

// --- health ---

func cliHealth(client *http.Client, base, token string) {
	result := doGet(client, base, token, "/health", nil)
	if status, ok := result["status"].(string); ok && status == "ok" {
		fmt.Println("✅ Server is healthy")
		if mode, ok := result["mode"].(string); ok {
			fmt.Printf("   Mode: %s\n", mode)
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

// --- helpers ---

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
