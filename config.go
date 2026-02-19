package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxBodySize = 1 << 20

const targetTypePage = "page"

const filterInteractive = "interactive"

const (
	actionClick      = "click"
	actionType       = "type"
	actionFill       = "fill"
	actionPress      = "press"
	actionFocus      = "focus"
	actionHover      = "hover"
	actionSelect     = "select"
	actionScroll     = "scroll"
	actionHumanClick = "humanClick"
	actionHumanType  = "humanType"
)

const (
	tabActionNew   = "new"
	tabActionClose = "close"
)

//go:embed stealth.js
var stealthScript string

//go:embed readability.js
var readabilityJS string

//go:embed welcome.html
var welcomeHTML string

// RuntimeConfig holds all runtime configuration in a single struct.
// Loaded once at startup from environment variables and optional config file.
type RuntimeConfig struct {
	Port             string
	CdpURL           string
	Token            string
	StateDir         string
	Headless         bool
	NoRestore        bool
	ProfileDir       string
	ChromeVersion    string
	Timezone         string
	BlockImages      bool
	BlockMedia       bool
	ChromeBinary     string
	ChromeExtraFlags string
	NoAnimations     bool
	StealthLevel     string
	ActionTimeout    time.Duration
	NavigateTimeout  time.Duration
	ShutdownTimeout  time.Duration
	WaitNavDelay     time.Duration
}

// cfg is the package-level runtime configuration, populated by loadConfig().
var cfg RuntimeConfig

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBoolOr(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// FileConfig is the JSON config file format.
type FileConfig struct {
	Port        string `json:"port"`
	CdpURL      string `json:"cdpUrl,omitempty"`
	Token       string `json:"token,omitempty"`
	StateDir    string `json:"stateDir"`
	ProfileDir  string `json:"profileDir"`
	Headless    *bool  `json:"headless,omitempty"`
	NoRestore   bool   `json:"noRestore"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	NavigateSec int    `json:"navigateSec,omitempty"`
}

func loadConfig() {
	// Populate from environment with defaults
	cfg = RuntimeConfig{
		Port:             envOr("BRIDGE_PORT", "9867"),
		CdpURL:           os.Getenv("CDP_URL"),
		Token:            os.Getenv("BRIDGE_TOKEN"),
		StateDir:         envOr("BRIDGE_STATE_DIR", filepath.Join(homeDir(), ".pinchtab")),
		Headless:         envBoolOr("BRIDGE_HEADLESS", true),
		NoRestore:        os.Getenv("BRIDGE_NO_RESTORE") == "true",
		ProfileDir:       envOr("BRIDGE_PROFILE", filepath.Join(homeDir(), ".pinchtab", "chrome-profile")),
		ChromeVersion:    envOr("BRIDGE_CHROME_VERSION", "144.0.7559.133"),
		Timezone:         os.Getenv("BRIDGE_TIMEZONE"),
		BlockImages:      os.Getenv("BRIDGE_BLOCK_IMAGES") == "true",
		BlockMedia:       os.Getenv("BRIDGE_BLOCK_MEDIA") == "true",
		ChromeBinary:     os.Getenv("CHROME_BINARY"),
		ChromeExtraFlags: os.Getenv("CHROME_FLAGS"),
		NoAnimations:     os.Getenv("BRIDGE_NO_ANIMATIONS") == "true",
		StealthLevel:     envOr("BRIDGE_STEALTH", "light"),
		ActionTimeout:    15 * time.Second,
		NavigateTimeout:  30 * time.Second,
		ShutdownTimeout:  10 * time.Second,
		WaitNavDelay:     1 * time.Second,
	}

	// Override from config file (env vars take precedence)
	configPath := envOr("BRIDGE_CONFIG", filepath.Join(homeDir(), ".pinchtab", "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		slog.Warn("invalid JSON in config file, ignoring", "path", configPath, "err", err)
		return
	}

	if fc.Port != "" && os.Getenv("BRIDGE_PORT") == "" {
		cfg.Port = fc.Port
	}
	if fc.CdpURL != "" && os.Getenv("CDP_URL") == "" {
		cfg.CdpURL = fc.CdpURL
	}
	if fc.Token != "" && os.Getenv("BRIDGE_TOKEN") == "" {
		cfg.Token = fc.Token
	}
	if fc.StateDir != "" && os.Getenv("BRIDGE_STATE_DIR") == "" {
		cfg.StateDir = fc.StateDir
	}
	if fc.ProfileDir != "" && os.Getenv("BRIDGE_PROFILE") == "" {
		cfg.ProfileDir = fc.ProfileDir
	}
	if fc.Headless != nil && os.Getenv("BRIDGE_HEADLESS") == "" {
		cfg.Headless = *fc.Headless
	}
	if fc.NoRestore && os.Getenv("BRIDGE_NO_RESTORE") == "" {
		cfg.NoRestore = true
	}
	if fc.TimeoutSec > 0 && os.Getenv("BRIDGE_TIMEOUT") == "" {
		cfg.ActionTimeout = time.Duration(fc.TimeoutSec) * time.Second
	}
	if fc.NavigateSec > 0 && os.Getenv("BRIDGE_NAV_TIMEOUT") == "" {
		cfg.NavigateTimeout = time.Duration(fc.NavigateSec) * time.Second
	}
}

func defaultFileConfig() FileConfig {
	h := true
	return FileConfig{
		Port:        "9867",
		StateDir:    filepath.Join(homeDir(), ".pinchtab"),
		ProfileDir:  filepath.Join(homeDir(), ".pinchtab", "chrome-profile"),
		Headless:    &h,
		NoRestore:   false,
		TimeoutSec:  15,
		NavigateSec: 30,
	}
}

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

		if _, err := os.Stat(configPath); err == nil {
			fmt.Printf("Config file already exists at %s\n", configPath)
			fmt.Print("Overwrite? (y/N): ")
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				return
			}
		}

		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			os.Exit(1)
		}

		fc := defaultFileConfig()
		data, _ := json.MarshalIndent(fc, "", "  ")
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
  "stateDir": "` + fc.StateDir + `",
  "profileDir": "` + fc.ProfileDir + `"
}`)

	case "show":
		fmt.Println("Current configuration:")
		fmt.Printf("  Port:       %s\n", cfg.Port)
		fmt.Printf("  CDP URL:    %s\n", cfg.CdpURL)
		fmt.Printf("  Token:      %s\n", maskToken(cfg.Token))
		fmt.Printf("  State Dir:  %s\n", cfg.StateDir)
		fmt.Printf("  Profile:    %s\n", cfg.ProfileDir)
		fmt.Printf("  Headless:   %v\n", cfg.Headless)
		fmt.Printf("  No Restore: %v\n", cfg.NoRestore)
		fmt.Printf("  Timeouts:   action=%v navigate=%v\n", cfg.ActionTimeout, cfg.NavigateTimeout)

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
