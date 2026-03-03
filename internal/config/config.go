package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RuntimeConfig struct {
	Bind              string
	Port              string
	InstancePortStart int // Starting port for instances (default 9868)
	InstancePortEnd   int // Ending port for instances (default 9968)
	CdpURL            string
	Token             string
	StateDir          string
	Headless          bool
	NoRestore         bool
	ProfileDir        string
	ChromeVersion     string
	Timezone          string
	BlockImages       bool
	BlockMedia        bool
	BlockAds          bool
	MaxTabs           int
	ChromeBinary      string
	ChromeExtraFlags  string
	UserAgent         string
	NoAnimations      bool
	StealthLevel      string
	ActionTimeout     time.Duration
	NavigateTimeout   time.Duration
	ShutdownTimeout   time.Duration
	WaitNavDelay      time.Duration

	// Orchestrator strategy settings (dashboard/orchestrator mode only).
	Strategy         string // Allocation strategy: simple, session, explicit (default: "")
	AllocationPolicy string // Instance selection policy: fcfs, round_robin, random (default: "fcfs")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
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

// homeDir returns the user's home directory, checking $HOME first for container compatibility
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	h, _ := os.UserHomeDir()
	return h
}

// userConfigDir returns the OS-appropriate app config directory:
// - macOS: ~/Library/Application Support/pinchtab
// - Linux: ~/.config/pinchtab (or $XDG_CONFIG_HOME/pinchtab)
// - Windows: %APPDATA%\pinchtab
//
// For backwards compatibility, if ~/.pinchtab exists and the new location
// doesn't, it returns ~/.pinchtab (allowing seamless migration).
func userConfigDir() string {
	home := homeDir()
	legacyPath := filepath.Join(home, ".pinchtab")

	// Try to get OS-appropriate config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to legacy location if UserConfigDir fails
		return legacyPath
	}

	newPath := filepath.Join(configDir, "pinchtab")

	// Backwards compatibility: if legacy location exists and new doesn't, use legacy
	legacyExists := dirExists(legacyPath)
	newExists := dirExists(newPath)

	if legacyExists && !newExists {
		return legacyPath
	}

	return newPath
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (c *RuntimeConfig) ListenAddr() string {
	return c.Bind + ":" + c.Port
}

type FileConfig struct {
	Port              string `json:"port"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
	CdpURL            string `json:"cdpUrl,omitempty"`
	Token             string `json:"token,omitempty"`
	StateDir          string `json:"stateDir"`
	ProfileDir        string `json:"profileDir"`
	Headless          *bool  `json:"headless,omitempty"`
	NoRestore         bool   `json:"noRestore"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	TimeoutSec        int    `json:"timeoutSec,omitempty"`
	NavigateSec       int    `json:"navigateSec,omitempty"`

	// Orchestrator strategy settings.
	Strategy         string `json:"strategy,omitempty"`         // simple, session, explicit
	AllocationPolicy string `json:"allocationPolicy,omitempty"` // fcfs, round_robin, random
}

func Load() *RuntimeConfig {
	cfg := &RuntimeConfig{
		Bind:              envOr("BRIDGE_BIND", "127.0.0.1"),
		Port:              envOr("BRIDGE_PORT", "9867"),
		InstancePortStart: envIntOr("INSTANCE_PORT_START", 9868),
		InstancePortEnd:   envIntOr("INSTANCE_PORT_END", 9968),
		CdpURL:            os.Getenv("CDP_URL"),
		Token:             os.Getenv("BRIDGE_TOKEN"),
		StateDir:          envOr("BRIDGE_STATE_DIR", userConfigDir()),
		Headless:          envBoolOr("BRIDGE_HEADLESS", true),
		NoRestore:         os.Getenv("BRIDGE_NO_RESTORE") == "true",
		ProfileDir:        envOr("BRIDGE_PROFILE", filepath.Join(userConfigDir(), "chrome-profile")),
		ChromeVersion:     envOr("BRIDGE_CHROME_VERSION", "144.0.7559.133"),
		Timezone:          os.Getenv("BRIDGE_TIMEZONE"),
		BlockImages:       os.Getenv("BRIDGE_BLOCK_IMAGES") == "true",
		BlockMedia:        os.Getenv("BRIDGE_BLOCK_MEDIA") == "true",
		BlockAds:          envBoolOr("BRIDGE_BLOCK_ADS", false),
		MaxTabs:           envIntOr("BRIDGE_MAX_TABS", 20),
		ChromeBinary:      envOr("CHROME_BIN", os.Getenv("CHROME_BINARY")),
		ChromeExtraFlags:  os.Getenv("CHROME_FLAGS"),
		UserAgent:         os.Getenv("BRIDGE_USER_AGENT"),
		NoAnimations:      os.Getenv("BRIDGE_NO_ANIMATIONS") == "true",
		StealthLevel:      envOr("BRIDGE_STEALTH", "light"),
		ActionTimeout:     30 * time.Second,
		NavigateTimeout:   60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		WaitNavDelay:      1 * time.Second,
		Strategy:          os.Getenv("PINCHTAB_STRATEGY"),
		AllocationPolicy:  envOr("PINCHTAB_ALLOCATION_POLICY", "fcfs"),
	}

	configPath := envOr("BRIDGE_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return cfg
	}

	if fc.Port != "" && os.Getenv("BRIDGE_PORT") == "" {
		cfg.Port = fc.Port
	}
	if fc.InstancePortStart != nil && os.Getenv("INSTANCE_PORT_START") == "" {
		cfg.InstancePortStart = *fc.InstancePortStart
	}
	if fc.InstancePortEnd != nil && os.Getenv("INSTANCE_PORT_END") == "" {
		cfg.InstancePortEnd = *fc.InstancePortEnd
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
	if fc.MaxTabs != nil && os.Getenv("BRIDGE_MAX_TABS") == "" {
		cfg.MaxTabs = *fc.MaxTabs
	}
	if fc.TimeoutSec > 0 && os.Getenv("BRIDGE_TIMEOUT") == "" {
		cfg.ActionTimeout = time.Duration(fc.TimeoutSec) * time.Second
	}
	if fc.NavigateSec > 0 && os.Getenv("BRIDGE_NAV_TIMEOUT") == "" {
		cfg.NavigateTimeout = time.Duration(fc.NavigateSec) * time.Second
	}
	if fc.Strategy != "" && os.Getenv("PINCHTAB_STRATEGY") == "" {
		cfg.Strategy = fc.Strategy
	}
	if fc.AllocationPolicy != "" && os.Getenv("PINCHTAB_ALLOCATION_POLICY") == "" {
		cfg.AllocationPolicy = fc.AllocationPolicy
	}

	return cfg
}

func DefaultFileConfig() FileConfig {
	h := true
	start := 9868
	end := 9968
	return FileConfig{
		Port:              "9867",
		InstancePortStart: &start,
		InstancePortEnd:   &end,
		StateDir:          userConfigDir(),
		ProfileDir:        filepath.Join(userConfigDir(), "chrome-profile"),
		Headless:          &h,
		NoRestore:         false,
		TimeoutSec:        15,
		NavigateSec:       30,
	}
}

func HandleConfigCommand(cfg *RuntimeConfig) {
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

		fc := DefaultFileConfig()
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
		fmt.Printf("  Token:      %s\n", MaskToken(cfg.Token))
		fmt.Printf("  State Dir:  %s\n", cfg.StateDir)
		fmt.Printf("  Profile:    %s\n", cfg.ProfileDir)
		fmt.Printf("  Headless:   %v\n", cfg.Headless)
		fmt.Printf("  Max Tabs:   %d\n", cfg.MaxTabs)
		fmt.Printf("  No Restore: %v\n", cfg.NoRestore)
		fmt.Printf("  Timeouts:   action=%v navigate=%v\n", cfg.ActionTimeout, cfg.NavigateTimeout)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func MaskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
