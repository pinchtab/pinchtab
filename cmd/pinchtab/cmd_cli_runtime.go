package main

import (
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
	if envURL := os.Getenv("PINCHTAB_SERVER"); envURL != "" {
		return strings.TrimRight(envURL, "/")
	}
	return "http://127.0.0.1:9867"
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
