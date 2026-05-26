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
	bridgeCDPAttach         string
	bridgeBrowser           string
	bridgeRemoteBrowserName string
	bridgeBind              string
	bridgePort              string
)

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Start single-instance bridge-only server",
	Long: `Start a single-instance bridge server.

By default, the bridge launches Chrome itself. Use --cdp-attach to attach the
bridge to an already-running browser process (Chrome or CloakBrowser) via
its remote debugging URL — the external process is never killed by PinchTab.

Examples:
  pinchtab bridge
  pinchtab bridge --cdp-attach ws://127.0.0.1:9222/devtools/browser/<id>
  pinchtab bridge --cdp-attach http://127.0.0.1:9222 \
    --browser cloak --remote-browser-name cloak-manager-profile
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		if v := strings.TrimSpace(bridgeCDPAttach); v != "" {
			cdpURL, err := validateBridgeCDPURL(v)
			if err != nil {
				return err
			}
			cfg.CDPAttachURL = cdpURL
			cfg.RemoteCDPURL = cdpURL
		}
		if v := strings.TrimSpace(bridgeBind); v != "" {
			cfg.Bind = v
		}
		if v := strings.TrimSpace(bridgePort); v != "" {
			cfg.Port = v
		}

		if browser, err := resolveBridgeBrowser(bridgeBrowser, cfg.BrowsersAvailable); err != nil {
			return err
		} else if browser != "" {
			cfg.DefaultBrowser = browser
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
		return "", fmt.Errorf("invalid --cdp-attach %q: %w", raw, err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("invalid --cdp-attach %q: missing scheme", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid --cdp-attach %q: missing host", raw)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "ws", "wss":
		if strings.Contains(parsed.Path, "/devtools/page/") {
			return "", fmt.Errorf("invalid --cdp-attach %q: page-level CDP URLs are not supported; use a browser-level URL", raw)
		}
		if !strings.Contains(parsed.Path, "/devtools/browser/") {
			return "", fmt.Errorf("invalid --cdp-attach %q: expected browser-level path /devtools/browser/<id>", raw)
		}
	case "http", "https":
		if parsed.Path != "" && parsed.Path != "/" && !strings.HasSuffix(parsed.Path, "/json/version") {
			return "", fmt.Errorf("invalid --cdp-attach %q: HTTP URL must be the DevTools origin or /json/version", raw)
		}
	default:
		return "", fmt.Errorf("invalid --cdp-attach %q: expected ws, wss, http, or https", raw)
	}
	return trimmed, nil
}

func resolveBridgeBrowser(browserFlag string, configured []string) (string, error) {
	v := strings.TrimSpace(browserFlag)
	if v == "" {
		return "", nil
	}
	return config.ParseBrowser(v, configured)
}

func init() {
	bridgeCmd.GroupID = "primary"
	bridgeCmd.Flags().StringVar(&bridgeCDPAttach, "cdp-attach", "", "Attach to an existing browser via this CDP URL (ws://... browser-level, or http://... DevTools origin)")
	bridgeCmd.Flags().StringVar(&bridgeBind, "bind", "", "Bind address for the bridge HTTP server (overrides config server.bind)")
	bridgeCmd.Flags().StringVar(&bridgePort, "port", "", "Port for the bridge HTTP server (overrides config server.port)")
	bridgeCmd.Flags().StringVar(&bridgeBrowser, "browser", "", "Browser to use: chrome, cloak, or ghost-chrome (overrides config)")
	bridgeCmd.Flags().StringVar(&bridgeRemoteBrowserName, "remote-browser-name", "", "Opaque label for the externally-managed browser; surfaces in /stealth/status")
	rootCmd.AddCommand(bridgeCmd)
}
