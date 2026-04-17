package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
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

func runCLIWith(cfg *config.RuntimeConfig, fn func(cliRuntime)) {
	fn(cliRuntime{
		client: newCLIHTTPClient(resolveCLIAgentID()),
		base:   resolveCLIBase(cfg),
		token:  resolveCLIToken(cfg),
	})
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
	if serverURL != "" {
		return strings.TrimRight(serverURL, "/")
	}
	if envURL := os.Getenv("PINCHTAB_SERVER"); envURL != "" {
		return strings.TrimRight(envURL, "/")
	}
	// Default to the main server port so requests go through the
	// orchestrator. This ensures activity is recorded in the shared
	// store and visible in the dashboard.
	return fmt.Sprintf("http://127.0.0.1:%s", cfg.Port)
}

func resolveCLIToken(cfg *config.RuntimeConfig) string {
	if sessionToken := os.Getenv("PINCHTAB_SESSION"); sessionToken != "" {
		return sessionToken
	}
	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}
	return token
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
