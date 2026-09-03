package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
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
// It shares runtimekit.ResolveEffectiveBrowser with bridge/runtime.InitBrowser,
// so it cannot diverge from what actually runs.
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
			"       Install one (%s) or set a browser binary in your config.\n"+
			"       Run `pinchtab doctor` for the full diagnosis",
			browserID, browsers.InstallHint(runtime.GOOS))
	}
	return nil
}

func newCLIRuntime(cfg *config.RuntimeConfig) cliRuntime {
	base := resolveCLIBase(cfg)
	return cliRuntime{
		client: newCLIHTTPClient(resolveCLIAgentID()),
		base:   base,
		token:  tokenForBaseOrExit(cfg, base),
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
			output.Advisory("--server " + resolved + " is the default and can be omitted")
		} else if os.Getenv("PINCHTAB_SERVER") != "" {
			output.Advisory("PINCHTAB_SERVER=" + resolved + " is the default and can be omitted")
		}
	}
	return resolved
}

func resolveDefaultCLIBase(cfg *config.RuntimeConfig) string {
	return fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
}

// resolveTabStateEndpoint resolves the server the cached current tab belongs to,
// with the SAME precedence the command being guarded uses: --server/PINCHTAB_SERVER,
// else the configured port; token from PINCHTAB_SESSION/PINCHTAB_TOKEN, else the
// config file's. The tab probe used the env-only resolvers with a hardcoded 9867
// fallback, so with a configured port it probed a different server than the command
// — found nothing listening, assumed the stale tab valid, and refreshed its own
// freshness window. Same target, so the probe can self-heal.
//
// The credential is the same EXCEPT against a non-loopback base with only the config
// file's server.token: there the probe DEGRADES to an unauthenticated request rather
// than exiting, because it runs on every command and must not kill one that would
// refuse properly at its own site. Either way the config token does not leave the
// machine; the probe simply learns less.
var resolveTabStateEndpoint = func() (base, token string) {
	cfg := config.Load()
	base = resolveBaseURL(resolveDefaultCLIBase(cfg))
	token, err := resolveCLIToken(cfg, base)
	if err != nil {
		return base, ""
	}
	return base, token
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

func canAutoStartServerForCLI(cfg *config.RuntimeConfig, baseURL string) bool {
	if serverURL != "" || os.Getenv("PINCHTAB_SERVER") != "" {
		return false
	}
	return strings.TrimRight(baseURL, "/") == resolveDefaultCLIBase(cfg)
}

// resolveCLIToken is the ONE owner pairing a credential with its destination.
// The config file's server.token is a secret for the LOCAL server: it is only
// ever sent to a loopback base, so a typo'd or hostile --server host cannot
// receive it — an explicit env credential travels anywhere the caller says.
func resolveCLIToken(cfg *config.RuntimeConfig, base string) (string, error) {
	if s := os.Getenv("PINCHTAB_SESSION"); s != "" {
		apiclient.UseTokenSource("the PINCHTAB_SESSION environment variable")
		return s, nil
	}
	if t := os.Getenv("PINCHTAB_TOKEN"); t != "" {
		apiclient.UseTokenSource("the PINCHTAB_TOKEN environment variable")
		return t, nil
	}
	// The emptiness test comes FIRST: with no server.token there is nothing to withhold, so
	// refusing would name a secret that does not exist and would break a legitimate flow —
	// a tokenless server on a LAN or in Docker — for no security gain. That is the same test
	// the loopback predicate itself was chosen by. With no credential the remote answers its
	// own honest 401.
	if cfg.Token == "" {
		return "", nil
	}
	if !loopbackBase(base) {
		return "", fmt.Errorf("refusing to send the local config's server.token to %s: set PINCHTAB_TOKEN (or PINCHTAB_SESSION) with the credential for that host", base)
	}
	apiclient.UseTokenSource("server.token in " + cliTokenConfigPath())
	return cfg.Token, nil
}

// tokenForBaseOrExit is the terminal wrapper for command paths: the refusal
// happens here, before any client or request exists.
func tokenForBaseOrExit(cfg *config.RuntimeConfig, base string) string {
	token, err := resolveCLIToken(cfg, base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pinchtab: %v\n", err)
		osExit(1)
	}
	return token
}

var osExit = os.Exit

func loopbackBase(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func cliTokenConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("PINCHTAB_CONFIG")); p != "" {
		return p
	}
	return config.DefaultConfigPath()
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
