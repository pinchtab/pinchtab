package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/spf13/cobra"
)

type cliRuntime struct {
	client *http.Client
	base   string
	token  string
}

func runCLI(fn func(cliRuntime)) {
	runCLIWith(loadConfig(), fn)
}

func runCLIWithError(fn func(cliRuntime) error) error {
	return fn(newCLIRuntime(loadConfig()))
}

func runCLIWith(cfg *config.RuntimeConfig, fn func(cliRuntime)) {
	fn(newCLIRuntime(cfg))
}

func runCLIEnsuringServer(command string, fn func(cliRuntime)) {
	runCLIWithServerCheck(loadConfig(), command, fn)
}

// runCLIEnsuringServerNoBrowser auto-starts the local control plane if needed but
// skips the browser preflight, for commands that need the server but not a browser
// instance (e.g. session create). Without this, the documented "create a session
// first" step fails cold with a raw "connection refused" on a fresh machine, since
// only browser commands auto-start the server.
func runCLIEnsuringServerNoBrowser(command string, fn func(cliRuntime)) {
	cfg := loadConfig()
	rt := newCLIRuntime(cfg)
	if err := ensureServerForCLI(cfg, rt.base, rt.token, command); err != nil {
		fmt.Fprintf(os.Stderr, "pinchtab: %v\n", err)
		os.Exit(1)
	}
	fn(rt)
}

func runCLIWithServerCheck(cfg *config.RuntimeConfig, command string, fn func(cliRuntime)) {
	rt := newCLIRuntime(cfg)
	// Only preflight when this command would auto-start the local server. For a
	// remote --server the browser lives on that host, so local discovery is moot.
	if canAutoStartServerForCLI(cfg, rt.base) {
		if err := preflightBrowserBinary(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "pinchtab: %v\n", err)
			os.Exit(1)
		}
	}
	if err := ensureServerForCLI(cfg, rt.base, rt.token, command); err != nil {
		fmt.Fprintf(os.Stderr, "pinchtab: %v\n", err)
		os.Exit(1)
	}
	fn(rt)
}

// preflightBrowserBinary fails fast with an actionable message when the active
// provider has no usable browser, instead of letting the launch silently retry
// and surface only the bridge's generic "instance not ready after 10s" timeout.
// It mirrors the launch-time resolution in bridge/runtime.InitBrowser (explicit
// browser.binary wins over discovery) so it can't diverge from what actually runs.
func preflightBrowserBinary(cfg *config.RuntimeConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.CDPAttachURL) != "" {
		return nil // attaching to an external CDP endpoint; no local binary needed
	}
	effective := runtimekit.ResolveEffectiveBrowser(cfg)
	browserID := effective.ID
	if _, ok := browsers.Get(browserID); !ok {
		return nil // unknown provider — let the normal path report it
	}
	if override := strings.TrimSpace(effective.Binary); override != "" {
		if info, err := os.Stat(override); err != nil || info.IsDir() {
			return fmt.Errorf("configured browser executable does not point at a usable executable: %s\n"+
				"       Point the active browser config at an existing browser binary, or unset it to use auto-discovery. "+
				"Run `pinchtab doctor` for details", override)
		}
		return nil
	}
	if effective.Binary == "" {
		return fmt.Errorf("no %s browser found on this machine.\n"+
			"       Install one (e.g. Google Chrome for Testing, or `apt-get install -y chromium` "+
			"on Debian/Ubuntu) or set a browser binary in your config.\n"+
			"       Run `pinchtab doctor` for the full diagnosis", browserID)
	}
	return nil
}

func newCLIRuntime(cfg *config.RuntimeConfig) cliRuntime {
	return cliRuntime{
		client: newCLIHTTPClient(resolveCLIAgentID()),
		base:   resolveCLIBase(cfg),
		token:  resolveCLIToken(cfg),
	}
}

func newCLIHTTPClient(agentID string) *http.Client {
	baseTransport := http.DefaultTransport
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: agentHeaderTransport{
			base:    baseTransport,
			agentID: normalizeCLIAgentID(agentID),
		},
	}
}

func resolveCLIBase(cfg *config.RuntimeConfig) string {
	defaultBase := resolveDefaultCLIBase(cfg)
	resolved := resolveBaseURL(defaultBase)
	if resolved == defaultBase {
		if serverURL != "" {
			output.Hint("--server " + resolved + " is the default and can be omitted")
		} else if os.Getenv("PINCHTAB_SERVER") != "" {
			output.Hint("PINCHTAB_SERVER=" + resolved + " is the default and can be omitted")
		}
	}
	return resolved
}

func resolveDefaultCLIBase(cfg *config.RuntimeConfig) string {
	return fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
}

// resolveBaseURL returns the server base URL from flag/env/default.
// Shared by both the full CLI runtime path and the lightweight tab probe.
func resolveBaseURL(defaultBase string) string {
	if serverURL != "" {
		return strings.TrimRight(serverURL, "/")
	}
	if u := os.Getenv("PINCHTAB_SERVER"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBase
}

// resolveToken returns the auth token from env vars (session takes precedence).
// Shared by both the full CLI runtime path and the lightweight tab probe.
func resolveToken() string {
	if s := os.Getenv("PINCHTAB_SESSION"); s != "" {
		return s
	}
	return os.Getenv("PINCHTAB_TOKEN")
}

func canAutoStartServerForCLI(cfg *config.RuntimeConfig, baseURL string) bool {
	if serverURL != "" || os.Getenv("PINCHTAB_SERVER") != "" {
		return false
	}
	return strings.TrimRight(baseURL, "/") == resolveDefaultCLIBase(cfg)
}

func resolveCLIToken(cfg *config.RuntimeConfig) string {
	if t := resolveToken(); t != "" {
		return t
	}
	return cfg.Token
}

func resolveCLIAgentID() string {
	if trimmed := strings.TrimSpace(cliAgentID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(os.Getenv("PINCHTAB_AGENT_ID"))
}

func normalizeCLIAgentID(raw string) string {
	return strings.TrimSpace(raw)
}

type agentHeaderTransport struct {
	base    http.RoundTripper
	agentID string
}

func (t agentHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Header.Set(activity.HeaderPTSource, "client")
	if id := normalizeCLIAgentID(t.agentID); id != "" {
		cloned.Header.Set(activity.HeaderAgentID, id)
	}

	return base.RoundTrip(cloned)
}

func optionalArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func stringFlag(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}
