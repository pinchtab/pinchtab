package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

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

func cliInstances(client *http.Client, base, token string) {
	// /instances returns an array directly, not {"instances": [...]}
	body := doGetRaw(client, base, token, "/instances", nil)
	
	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to parse instances: %v\n", err)
		os.Exit(1)
	}

	if len(instances) == 0 {
		fmt.Println("No instances running")
		fmt.Println("\nTo launch an instance:")
		fmt.Println("  pinchtab launch            # headless, auto-named")
		fmt.Println("  pinchtab launch myprofile   # named profile")
		return
	}

	fmt.Println("\n🚀 Running Instances:")
	fmt.Println()

	for _, m := range instances {
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
	fmt.Println()
}

func cliLaunch(client *http.Client, base, token string, args []string) {
	body := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--headed":
			body["mode"] = "headed"
		case "--port":
			if i+1 < len(args) {
				i++
				body["port"] = args[i]
			}
		default:
			// Positional arg = profile name
			if !strings.HasPrefix(args[i], "-") {
				body["name"] = args[i]
			}
		}
	}

	result := doPost(client, base, token, "/instances/launch", body)
	id, _ := result["id"].(string)
	port, _ := result["port"].(string)
	fmt.Printf("🚀 Launched %s on port %s\n", id, port)
}

func cliStop(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab stop <instance-id>")
		os.Exit(1)
	}
	doPost(client, base, token, fmt.Sprintf("/instances/%s/stop", args[0]), nil)
	fmt.Printf("⏹️  Stopped %s\n", args[0])
}

func cliTabs(client *http.Client, base, token string) {
	// /tabs returns an array directly, not {"tabs": [...]}
	body := doGetRaw(client, base, token, "/tabs", nil)
	
	var tabs []map[string]any
	if err := json.Unmarshal(body, &tabs); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to parse tabs: %v\n", err)
		os.Exit(1)
	}

	if len(tabs) == 0 {
		fmt.Println("No tabs open")
		return
	}

	fmt.Println("\n📑 Open Tabs:")
	fmt.Println()

	for _, m := range tabs {
		id, _ := m["id"].(string)
		tabURL, _ := m["url"].(string)
		title, _ := m["title"].(string)

		if title == "" {
			title = "(untitled)"
		}

		fmt.Printf("  [%s] %s\n", id, title)
		fmt.Printf("       %s\n", tabURL)
	}
	fmt.Println()
}

func cliOpen(client *http.Client, base, token string, args []string) {
	tabURL := ""
	if len(args) > 0 {
		tabURL = args[0]
	}

	body := map[string]any{"action": "new"}
	if tabURL != "" {
		body["url"] = tabURL
	}

	result := doPost(client, base, token, "/tab", body)
	id, _ := result["tabId"].(string)
	resultURL, _ := result["url"].(string)
	if resultURL == "" {
		resultURL = "about:blank"
	}
	fmt.Printf("📑 Opened [%s] → %s\n", id, resultURL)
}

func cliClose(client *http.Client, base, token string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab close <tab-id>")
		os.Exit(1)
	}
	doDelete(client, base, token, fmt.Sprintf("/tabs/%s", args[0]))
	fmt.Printf("🗑️  Closed %s\n", args[0])
}
