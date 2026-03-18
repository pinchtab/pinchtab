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
	port := cfg.Port
	if port == "" {
		port = "9867"
	}
	base := fmt.Sprintf("http://127.0.0.1:%s", port)
	if serverURL != "" {
		base = strings.TrimRight(serverURL, "/")
	}
	return base
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
