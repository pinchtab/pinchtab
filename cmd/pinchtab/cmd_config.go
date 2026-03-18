package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var clipboardExecCommand = exec.Command

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Run: func(cmd *cobra.Command, args []string) {
		handleConfigOverview(loadLocalConfig())
	},
}

func init() {
	configCmd.GroupID = "config"
	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadLocalConfig()
			cli.HandleConfigShow(cfg)
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize a new config file",
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigInit()
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Show config file path",
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigPath()
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate config file",
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigValidate()
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "get <path>",
		Short: "Get a config value (e.g., server.port)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigGet(args[0])
		},
	})
	configSetCmd := &cobra.Command{
		Use:   "set <path> <val>",
		Short: "Set a config value (e.g., server.port 8080)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigSet(args[0], args[1])
		},
	}
	// Allow values like "--no-sandbox --disable-gpu" after the config path.
	configSetCmd.Flags().SetInterspersed(false)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(&cobra.Command{
		Use:   "patch <json>",
		Short: "Merge JSON into config",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigPatch(args[0])
		},
	})

	rootCmd.AddCommand(configCmd)
}

func handleConfigOverview(cfg *config.RuntimeConfig) {
	_, configPath, err := config.LoadFileConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("Error loading config path: %v", err)))
		os.Exit(1)
	}

	dashPort := cfg.Port
	if dashPort == "" {
		dashPort = "9870"
	}
	dashboardURL := fmt.Sprintf("http://localhost:%s", dashPort)
	running := server.CheckPinchTabRunning(dashPort, cfg.Token)

	for {
		fmt.Print(renderConfigOverview(cfg, configPath, dashboardURL, running))

		if !isInteractiveTerminal() {
			return
		}

		nextCfg, changed, done, err := promptConfigEdit(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			fmt.Println()
			continue
		}
		if done {
			return
		}
		if !changed {
			fmt.Println()
			continue
		}

		cfg = nextCfg
		dashPort = cfg.Port
		if dashPort == "" {
			dashPort = "9870"
		}
		dashboardURL = fmt.Sprintf("http://localhost:%s", dashPort)
		running = server.CheckPinchTabRunning(dashPort, cfg.Token)
		fmt.Println()
	}
}

func renderConfigOverview(cfg *config.RuntimeConfig, configPath, dashboardURL string, running bool) string {
	out := ""
	out += cli.StyleStdout(cli.HeadingStyle, "Config") + "\n\n"
	out += fmt.Sprintf("  1. %-18s %s\n", "Strategy", cli.StyleStdout(cli.ValueStyle, cfg.Strategy))
	out += fmt.Sprintf("  2. %-18s %s\n", "Allocation policy", cli.StyleStdout(cli.ValueStyle, cfg.AllocationPolicy))
	out += fmt.Sprintf("  3. %-18s %s\n", "Stealth level", cli.StyleStdout(cli.ValueStyle, cfg.StealthLevel))
	out += fmt.Sprintf("  4. %-18s %s\n", "Tab eviction", cli.StyleStdout(cli.ValueStyle, cfg.TabEvictionPolicy))
	out += fmt.Sprintf("  5. %-18s %s\n", "Copy token", cli.StyleStdout(cli.MutedStyle, "clipboard"))
	out += "\n"
	out += cli.StyleStdout(cli.HeadingStyle, "More") + "\n\n"
	out += fmt.Sprintf("  %s %s\n", cli.StyleStdout(cli.MutedStyle, "File:"), cli.StyleStdout(cli.ValueStyle, configPath))
	out += fmt.Sprintf("  %s %s\n", cli.StyleStdout(cli.MutedStyle, "Token:"), cli.StyleStdout(cli.ValueStyle, config.MaskToken(cfg.Token)))
	if running {
		out += fmt.Sprintf("  %s %s\n", cli.StyleStdout(cli.MutedStyle, "Dashboard:"), cli.StyleStdout(cli.ValueStyle, dashboardURL))
	} else {
		out += fmt.Sprintf("  %s %s\n", cli.StyleStdout(cli.MutedStyle, "Dashboard:"), cli.StyleStdout(cli.MutedStyle, "not running"))
	}
	if isInteractiveTerminal() {
		out += "\n"
		out += cli.StyleStdout(cli.MutedStyle, "Edit item (1-5, blank to exit):") + " "
	}
	out += "\n"
	return out
}

func promptConfigEdit(cfg *config.RuntimeConfig) (*config.RuntimeConfig, bool, bool, error) {
	choice, err := promptInput("", "")
	if err != nil {
		return nil, false, false, err
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return nil, false, true, nil
	}

	switch choice {
	case "1":
		nextCfg, changed, err := editConfigSelection("Instance strategy", "multiInstance.strategy", cfg.Strategy, config.ValidStrategies())
		return nextCfg, changed, false, err
	case "2":
		nextCfg, changed, err := editConfigSelection("Allocation policy", "multiInstance.allocationPolicy", cfg.AllocationPolicy, config.ValidAllocationPolicies())
		return nextCfg, changed, false, err
	case "3":
		nextCfg, changed, err := editConfigSelection("Default stealth level", "instanceDefaults.stealthLevel", cfg.StealthLevel, config.ValidStealthLevels())
		return nextCfg, changed, false, err
	case "4":
		nextCfg, changed, err := editConfigSelection("Default tab eviction", "instanceDefaults.tabEvictionPolicy", cfg.TabEvictionPolicy, config.ValidEvictionPolicies())
		return nextCfg, changed, false, err
	case "5":
		if err := copyConfigToken(cfg.Token); err != nil {
			return nil, false, false, err
		}
		return nil, false, false, nil
	default:
		return nil, false, false, fmt.Errorf("invalid selection %q", choice)
	}
}

func editConfigSelection(title, path, current string, values []string) (*config.RuntimeConfig, bool, error) {
	options := make([]menuOption, 0, len(values)+1)
	for _, value := range values {
		label := value
		if value == current {
			label += " (current)"
		}
		options = append(options, menuOption{label: label, value: value})
	}
	options = append(options, menuOption{label: "Cancel", value: "cancel"})

	picked, err := promptSelect(title, options)
	if err != nil {
		return nil, false, err
	}
	if picked == "" || picked == "cancel" {
		return nil, false, nil
	}

	nextCfg, changed, err := workflow.UpdateValue(path, picked)
	if err != nil {
		return nil, false, err
	}
	if changed {
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Updated %s to %s", path, picked)))
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "Restart PinchTab to apply file-based changes."))
	}
	return nextCfg, changed, nil
}

func copyConfigToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("server token is empty")
	}

	if err := copyToClipboard(token); err == nil {
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Token copied to clipboard."))
		return nil
	}

	fmt.Println(cli.StyleStdout(cli.WarningStyle, "Clipboard unavailable; copy the token manually:"))
	fmt.Println(cli.StyleStdout(cli.ValueStyle, token))
	return nil
}

func copyToClipboard(text string) error {
	candidates := clipboardCommands()
	var lastErr error

	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate.name); err != nil {
			lastErr = err
			continue
		}
		cmd := clipboardExecCommand(candidate.name, candidate.args...)
		cmd.Stdin = strings.NewReader(text)
		if output, err := cmd.CombinedOutput(); err != nil {
			if len(strings.TrimSpace(string(output))) > 0 {
				lastErr = fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
			} else {
				lastErr = err
			}
			continue
		}
		return nil
	}

	if lastErr == nil {
		return fmt.Errorf("no clipboard command available")
	}
	return lastErr
}

type clipboardCommand struct {
	name string
	args []string
}

func clipboardCommands() []clipboardCommand {
	switch runtime.GOOS {
	case "darwin":
		return []clipboardCommand{{name: "pbcopy"}}
	case "windows":
		return []clipboardCommand{{name: "clip"}}
	default:
		return []clipboardCommand{
			{name: "wl-copy"},
			{name: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "xsel", args: []string{"--clipboard", "--input"}},
		}
	}
}

func handleConfigInit() {
	configPath := workflow.CurrentConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return
		}
	}

	if err := workflow.InitDefaultConfig(configPath); err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file created at %s\n", configPath)
}

func handleConfigPath() {
	fmt.Println(workflow.CurrentConfigPath())
}

func handleConfigGet(path string) {
	value, err := workflow.GetValue(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(value)
}

func handleConfigSet(path, value string) {
	change, err := workflow.PrepareSetValue(path, value)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if errs := change.ValidationErrors; len(errs) > 0 {
		fmt.Printf("Warning: new value causes validation error(s):\n")
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		fmt.Print("Save anyway? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return
		}
	}

	if err := workflow.SavePreparedChange(change); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Set %s = %s\n", path, value)
}

func handleConfigPatch(jsonPatch string) {
	change, err := workflow.PreparePatch(jsonPatch)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if errs := change.ValidationErrors; len(errs) > 0 {
		fmt.Printf("Warning: patch causes validation error(s):\n")
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		fmt.Print("Save anyway? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return
		}
	}

	if err := workflow.SavePreparedChange(change); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Config patched successfully")
}

func handleConfigValidate() {
	configPath, errs, err := workflow.ValidateCurrentFile()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(errs) > 0 {
		fmt.Printf("Config file has %d error(s):\n", len(errs))
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("Config file is valid: %s\n", configPath)
}
