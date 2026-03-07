package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

var version = "dev"

func resolveServerMode(getenv func(string) string) string {
	// PINCHTAB_ONLY is used by orchestrator-spawned child instances and must
	// always force bridge mode.
	if strings.TrimSpace(getenv("PINCHTAB_ONLY")) == "1" {
		return "bridge"
	}

	mode := strings.ToLower(strings.TrimSpace(getenv("PINCHTAB_MODE")))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(getenv("BRIDGE_MODE")))
	}
	switch mode {
	case "bridge", "dashboard":
		return mode
	}

	// Legacy hard-force fallback. Keep this lower precedence than explicit mode.
	if strings.TrimSpace(getenv("BRIDGE_ONLY")) == "1" {
		return "bridge"
	}

	return "dashboard"
}

func main() {
	cfg := config.Load()

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("pinchtab %s\n", version)
		os.Exit(0)
	}

	if len(os.Args) > 1 && (os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "config" {
		config.HandleConfigCommand(cfg)
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "connect" {
		handleConnectCommand(cfg)
		os.Exit(0)
	}

	// CLI commands
	if len(os.Args) > 1 && isCLICommand(os.Args[1]) {
		runCLI(cfg)
		return
	}

	if resolveServerMode(os.Getenv) == "bridge" {
		runBridgeServer(cfg)
		return
	}

	// Default: run dashboard mode
	// (includes 'pinchtab' with no args and unrecognized args like 'dashboard')
	runDashboard(cfg)
}
