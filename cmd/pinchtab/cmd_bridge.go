package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var (
	bridgeEngine            string
	bridgeCDPURL            string
	bridgeBrowserProvider   string
	bridgeRemoteBrowserName string
)

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Start single-instance bridge-only server",
	Long: `Start a single-instance bridge server.

By default, the bridge launches Chrome itself. Use --cdp-url to attach the
bridge to an already-running browser process (Chrome or CloakBrowser) via
its remote debugging URL — the external process is never killed by PinchTab.

Examples:
  pinchtab bridge
  pinchtab bridge --cdp-url ws://127.0.0.1:9222/devtools/browser/<id>
  pinchtab bridge --cdp-url http://127.0.0.1:9222 \
    --browser-provider cloak --remote-browser-name cloak-manager-profile
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		engineMode, err := resolveBridgeEngine(bridgeEngine, cfg.Engine)
		if err != nil {
			return err
		}
		cfg.Engine = engineMode

		if v := strings.TrimSpace(bridgeBrowserProvider); v != "" {
			provider, err := config.ParseBrowserProvider(v)
			if err != nil {
				return err
			}
			cfg.BrowserProvider = provider
		}
		if v := strings.TrimSpace(bridgeCDPURL); v != "" {
			cdpURL, err := validateBridgeCDPURL(v)
			if err != nil {
				return err
			}
			cfg.RemoteCDPURL = cdpURL
		}
		if v := strings.TrimSpace(bridgeRemoteBrowserName); v != "" {
			cfg.RemoteBrowserName = v
		}

		server.RunBridgeServer(cfg, version)
		return nil
	},
}

func validateBridgeCDPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid --cdp-url %q: %w", raw, err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("invalid --cdp-url %q: missing scheme", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid --cdp-url %q: missing host", raw)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "ws", "wss":
		if strings.Contains(parsed.Path, "/devtools/page/") {
			return "", fmt.Errorf("invalid --cdp-url %q: page-level CDP URLs are not supported; use a browser-level URL", raw)
		}
		if !strings.Contains(parsed.Path, "/devtools/browser/") {
			return "", fmt.Errorf("invalid --cdp-url %q: expected browser-level path /devtools/browser/<id>", raw)
		}
	case "http", "https":
		if parsed.Path != "" && parsed.Path != "/" && !strings.HasSuffix(parsed.Path, "/json/version") {
			return "", fmt.Errorf("invalid --cdp-url %q: HTTP URL must be the DevTools origin or /json/version", raw)
		}
	default:
		return "", fmt.Errorf("invalid --cdp-url %q: expected ws, wss, http, or https", raw)
	}
	return trimmed, nil
}

func resolveBridgeEngine(flagValue, configValue string) (string, error) {
	engineMode := strings.ToLower(strings.TrimSpace(configValue))
	if strings.TrimSpace(flagValue) != "" {
		engineMode = strings.ToLower(strings.TrimSpace(flagValue))
	}
	if engineMode == "" {
		engineMode = "chrome"
	}
	if engineMode != "chrome" && engineMode != "lite" && engineMode != "auto" {
		return "", fmt.Errorf("invalid --engine %q (expected chrome, lite, or auto)", engineMode)
	}
	return engineMode, nil
}

func init() {
	bridgeCmd.GroupID = "primary"
	bridgeCmd.Flags().StringVar(&bridgeEngine, "engine", "", "Bridge engine: chrome, lite, or auto (overrides config)")
	bridgeCmd.Flags().StringVar(&bridgeCDPURL, "cdp-url", "", "Attach to an existing browser via this CDP URL (ws://… browser-level, or http://… DevTools origin)")
	bridgeCmd.Flags().StringVar(&bridgeBrowserProvider, "browser-provider", "", "Browser provider for remote attach: chrome or cloak (default: from config)")
	bridgeCmd.Flags().StringVar(&bridgeRemoteBrowserName, "remote-browser-name", "", "Opaque label for the externally-managed browser; surfaces in /stealth/status")
	rootCmd.AddCommand(bridgeCmd)
}
