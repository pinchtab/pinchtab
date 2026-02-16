package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Body size limit for POST handlers.
const maxBodySize = 1 << 20 // 1MB

// Target type for Chrome DevTools Protocol.
const targetTypePage = "page"

// Snapshot filter values.
const filterInteractive = "interactive"

// Action kinds for /action endpoint.
const (
	actionClick  = "click"
	actionType   = "type"
	actionFill   = "fill"
	actionPress  = "press"
	actionFocus  = "focus"
	actionHover  = "hover"
	actionSelect = "select"
	actionScroll = "scroll"
)

// Tab actions for /tab endpoint.
const (
	tabActionNew   = "new"
	tabActionClose = "close"
)

//go:embed stealth.js
var stealthScript string

//go:embed readability.js
var readabilityJS string

var (
	port            = envOr("BRIDGE_PORT", "9867")
	cdpURL          = os.Getenv("CDP_URL") // empty = launch Chrome ourselves
	token           = os.Getenv("BRIDGE_TOKEN")
	stateDir        = envOr("BRIDGE_STATE_DIR", filepath.Join(homeDir(), ".pinchtab"))
	headless        = os.Getenv("BRIDGE_HEADLESS") == "true"
	noRestore       = os.Getenv("BRIDGE_NO_RESTORE") == "true"
	profileDir      = envOr("BRIDGE_PROFILE", filepath.Join(homeDir(), ".pinchtab", "chrome-profile"))
	actionTimeout   = 15 * time.Second
	navigateTimeout = 30 * time.Second
	shutdownTimeout = 10 * time.Second
	waitNavDelay    = 1 * time.Second
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// Config represents the configuration file structure
type Config struct {
	Port        string `json:"port"`
	CdpURL      string `json:"cdpUrl,omitempty"`
	Token       string `json:"token,omitempty"`
	StateDir    string `json:"stateDir"`
	ProfileDir  string `json:"profileDir"`
	Headless    bool   `json:"headless"`
	NoRestore   bool   `json:"noRestore"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	NavigateSec int    `json:"navigateSec,omitempty"`
}

// loadConfig loads configuration from file or environment variables
func loadConfig() {
	// Default config file location
	configPath := envOr("BRIDGE_CONFIG", filepath.Join(homeDir(), ".pinchtab", "config.json"))

	// Try to load config file
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil {
			// Apply config file values (env vars take precedence)
			if cfg.Port != "" && os.Getenv("BRIDGE_PORT") == "" {
				port = cfg.Port
			}
			if cfg.CdpURL != "" && os.Getenv("CDP_URL") == "" {
				cdpURL = cfg.CdpURL
			}
			if cfg.Token != "" && os.Getenv("BRIDGE_TOKEN") == "" {
				token = cfg.Token
			}
			if cfg.StateDir != "" && os.Getenv("BRIDGE_STATE_DIR") == "" {
				stateDir = cfg.StateDir
			}
			if cfg.ProfileDir != "" && os.Getenv("BRIDGE_PROFILE") == "" {
				profileDir = cfg.ProfileDir
			}
			if cfg.Headless && os.Getenv("BRIDGE_HEADLESS") == "" {
				headless = true
			}
			if cfg.NoRestore && os.Getenv("BRIDGE_NO_RESTORE") == "" {
				noRestore = true
			}
			if cfg.TimeoutSec > 0 && os.Getenv("BRIDGE_TIMEOUT") == "" {
				actionTimeout = time.Duration(cfg.TimeoutSec) * time.Second
			}
			if cfg.NavigateSec > 0 && os.Getenv("BRIDGE_NAV_TIMEOUT") == "" {
				navigateTimeout = time.Duration(cfg.NavigateSec) * time.Second
			}
		}
	}
}

// defaultConfig returns a default configuration
func defaultConfig() Config {
	return Config{
		Port:        "9867",
		StateDir:    filepath.Join(homeDir(), ".pinchtab"),
		ProfileDir:  filepath.Join(homeDir(), ".pinchtab", "chrome-profile"),
		Headless:    true,
		NoRestore:   false,
		TimeoutSec:  15,
		NavigateSec: 30,
	}
}

// handleConfigCommand handles the 'config' subcommand
func handleConfigCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pinchtab config <command>")
		fmt.Println("Commands:")
		fmt.Println("  init    - Create default config file")
		fmt.Println("  show    - Show current configuration")
		return
	}

	switch os.Args[2] {
	case "init":
		configPath := filepath.Join(homeDir(), ".pinchtab", "config.json")

		// Check if config already exists
		if _, err := os.Stat(configPath); err == nil {
			fmt.Printf("Config file already exists at %s\n", configPath)
			fmt.Print("Overwrite? (y/N): ")
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				return
			}
		}

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			os.Exit(1)
		}

		// Write default config
		cfg := defaultConfig()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			fmt.Printf("Error writing config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Config file created at %s\n", configPath)
		fmt.Println("\nExample with auth token:")
		fmt.Println(`{
  "port": "9867",
  "token": "your-secret-token",
  "headless": true,
  "stateDir": "` + cfg.StateDir + `",
  "profileDir": "` + cfg.ProfileDir + `"
}`)

	case "show":
		fmt.Println("Current configuration:")
		fmt.Printf("  Port:       %s\n", port)
		fmt.Printf("  CDP URL:    %s\n", cdpURL)
		fmt.Printf("  Token:      %s\n", maskToken(token))
		fmt.Printf("  State Dir:  %s\n", stateDir)
		fmt.Printf("  Profile:    %s\n", profileDir)
		fmt.Printf("  Headless:   %v\n", headless)
		fmt.Printf("  No Restore: %v\n", noRestore)
		fmt.Printf("  Timeouts:   action=%v navigate=%v\n", actionTimeout, navigateTimeout)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func maskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
