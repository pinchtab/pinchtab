package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/pinchtab/pinchtab/internal/config"
)

func loadConfig() *config.RuntimeConfig {
	cfg, err := loadConfigWithMandatoryToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

// loadConfigDeferringDiagnostics is loadConfig for the commands that resolve a log
// level: the returned diagnostics stay unemitted until resolveLogLevel has run,
// so a debug level asked for in the config file can show the load of that file.
// The caller must pass them to config.EmitLoadDiagnostics.
func loadConfigDeferringDiagnostics() (*config.RuntimeConfig, []config.LoadDiagnostic) {
	if err := ensureMandatoryToken(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	return config.LoadDeferringDiagnostics()
}

var emittedLoadDiagnostics sync.Map

func loadLocalConfig() *config.RuntimeConfig {
	cfg, diags := config.LoadDeferringDiagnostics()
	config.EmitLoadDiagnostics(loadDiagnosticsNotYetEmitted(diags))
	return cfg
}

func loadDiagnosticsNotYetEmitted(diags []config.LoadDiagnostic) []config.LoadDiagnostic {
	var fresh []config.LoadDiagnostic
	for _, d := range diags {
		if _, emitted := emittedLoadDiagnostics.LoadOrStore(fmt.Sprint(d.Level, d.Message, d.Attrs), struct{}{}); !emitted {
			fresh = append(fresh, d)
		}
	}
	return fresh
}

func loadConfigWithMandatoryToken() (*config.RuntimeConfig, error) {
	if err := ensureMandatoryToken(); err != nil {
		return nil, err
	}
	return loadLocalConfig(), nil
}

func ensureMandatoryToken() error {
	if strings.TrimSpace(os.Getenv("PINCHTAB_TOKEN")) != "" {
		return nil
	}

	fc, configPath, err := config.LoadFileConfig()
	if err != nil {
		return fmt.Errorf("load config file: %w", err)
	}
	if fc == nil {
		fc = &config.FileConfig{}
	}
	if strings.TrimSpace(fc.Server.Token) != "" {
		return nil
	}

	changed, err := config.ProvisionFileToken(fc, configPath)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	if err := config.SaveFileConfig(fc, configPath); err != nil {
		return fmt.Errorf("save config file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "pinchtab: generated server.token in %s\n", configPath)
	return nil
}
