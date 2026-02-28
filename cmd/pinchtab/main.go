package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

var version = "dev"

// Keep these for backwards compatibility / existing code
const chromeStartTimeout = 15 * time.Second

var commonWindowSizes = [][2]int{
	{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
	{1280, 720}, {1600, 900}, {2560, 1440}, {1280, 800},
}

func randomWindowSize() (int, int) {
	s := commonWindowSizes[rand.Intn(len(commonWindowSizes))]
	return s[0], s[1]
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

	// Check if running as bridge-only instance (spawned by orchestrator)
	if os.Getenv("BRIDGE_ONLY") == "1" {
		runBridgeServer(cfg)
		return
	}

	// Default: run dashboard mode
	// (includes 'pinchtab' with no args and unrecognized args like 'dashboard')
	runDashboard(cfg)
}
