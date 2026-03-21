package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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
		client: &http.Client{Timeout: 60 * time.Second},
		base:   resolveCLIBase(cfg),
		token:  resolveCLIToken(cfg),
	})
}

func resolveCLIBase(cfg *config.RuntimeConfig) string {
	if serverURL != "" {
		return strings.TrimRight(serverURL, "/")
	}
	if envURL := os.Getenv("PINCHTAB_URL"); envURL != "" {
		return strings.TrimRight(envURL, "/")
	}

	port := cfg.Port
	if port == "" {
		port = "9867"
	}

	bind := cfg.Bind
	if bind == "127.0.0.1" || bind == "localhost" {
		return fmt.Sprintf("http://%s:%s", bind, port)
	}
	if bind == "::1" || bind == "[::1]" {
		return fmt.Sprintf("http://[::1]:%s", port)
	}

	// For empty, 0.0.0.0, ::, or any other non-loopback bind,
	// fall back to 127.0.0.1 for implicit CLI targeting.
	return fmt.Sprintf("http://127.0.0.1:%s", port)
}

func resolveCLIToken(cfg *config.RuntimeConfig) string {
	token := cfg.Token
	if envToken := os.Getenv("PINCHTAB_TOKEN"); envToken != "" {
		token = envToken
	}
	return token
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
