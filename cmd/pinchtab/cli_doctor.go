package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

func cliDoctor(client *http.Client, base string, token string, args []string) {
	fmt.Println("🦀 Pinchtab Doctor - Setup & Diagnostics")
	fmt.Println("")

	passed := 0
	warnings := 0
	failures := 0

	// 1. Check git hooks
	fmt.Print("Checking git hooks configuration... ")
	if checkAndSetupGitHooks() {
		fmt.Println("✅")
		passed++
	} else {
		fmt.Println("⚠️ (run from repo root to configure)")
		warnings++
	}

	// 2. Check server connection
	fmt.Print("Checking server connection... ")
	if checkServerHealth(client, base, token) {
		fmt.Println("✅")
		passed++
	} else {
		fmt.Println("⚠️ (server not running, optional for setup)")
		warnings++
	}

	// 3. Check Go installation
	fmt.Print("Checking Go installation... ")
	if checkGo() {
		fmt.Println("✅")
		passed++
	} else {
		fmt.Println("❌ (required for building)")
		failures++
	}

	// 4. Check Chrome/Chromium
	fmt.Print("Checking Chrome/Chromium... ")
	if checkChrome() {
		fmt.Println("✅")
		passed++
	} else {
		fmt.Println("⚠️ (optional, but required for runtime)")
		warnings++
	}

	fmt.Println("")
	fmt.Printf("Results: %d passed, %d warnings, %d failures\n", passed, warnings, failures)

	if failures == 0 {
		fmt.Println("")
		fmt.Println("✅ Setup complete! Pinchtab is ready to use.")
	} else {
		fmt.Println("")
		fmt.Println("❌ Some checks failed. See above for details.")
		os.Exit(1)
	}
}

// checkAndSetupGitHooks configures git hooks for the repository
func checkAndSetupGitHooks() bool {
	// Check if .githooks directory exists
	if _, err := os.Stat(".githooks"); err != nil {
		fmt.Println("(not in repo root, skipping)")
		return true
	}

	// Check current hooks path
	cmd := exec.Command("git", "config", "core.hooksPath")
	output, err := cmd.Output()
	if err == nil && string(output) == ".githooks\n" {
		return true // Already configured
	}

	// Configure git hooks
	cmd = exec.Command("git", "config", "core.hooksPath", ".githooks")
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

// checkGo verifies Go is installed
func checkGo() bool {
	cmd := exec.Command("go", "version")
	return cmd.Run() == nil
}

// checkChrome verifies Chrome/Chromium is available
func checkChrome() bool {
	// Try common Chrome/Chromium locations
	paths := []string{
		"/usr/bin/google-chrome",                                       // Linux
		"/snap/bin/chromium",                                           // Linux snap
		"/opt/google/chrome/google-chrome",                             // Linux custom
		"/usr/bin/chromium-browser",                                    // Linux
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", // macOS
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",   // Windows
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// checkServerHealth checks if the server is running without exiting
func checkServerHealth(client *http.Client, base string, token string) bool {
	req, _ := http.NewRequest("GET", base+"/health", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == 200
}
