package main

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var bridgeEngine string

var bridgeCmd = &cobra.Command{
	Use:   "bridge",
	Short: "Start single-instance bridge-only server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		engineMode, err := resolveBridgeEngine(bridgeEngine, cfg.Engine)
		if err != nil {
			return err
		}
		cfg.Engine = engineMode
		server.RunBridgeServer(cfg)
		return nil
	},
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
	rootCmd.AddCommand(bridgeCmd)
}
