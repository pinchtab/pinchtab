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
	configschema "github.com/pinchtab/pinchtab/internal/schema"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/spf13/cobra"
)

var clipboardExecCommand = exec.Command

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		config.EmitDefaultConfigHint()
	},
	Run: func(cmd *cobra.Command, args []string) {
		printConfigOverview(loadLocalConfig())
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
	configSchemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Print config JSON Schema information",
		Run: func(cmd *cobra.Command, args []string) {
			printSchema, _ := cmd.Flags().GetBool("print")
			handleConfigSchema(printSchema)
		},
	}
	configSchemaCmd.Flags().Bool("print", false, "Print the bundled schema JSON instead of the schema URL")
	configCmd.AddCommand(configSchemaCmd)
	configCmd.AddCommand(&cobra.Command{
		Use:   "token",
		Short: "Copy the API token to clipboard",
		Long:  "Copies the configured server.token to the system clipboard. The token is never printed to stdout.",
		Run: func(cmd *cobra.Command, args []string) {
			handleConfigTokenCopy()
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleConfigSet(args[0], args[1])
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Allow values like "--no-sandbox --disable-gpu" after the config path.
	configSetCmd.Flags().SetInterspersed(false)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(&cobra.Command{
		Use:   "patch <json>",
		Short: "Merge JSON into config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleConfigPatch(args[0])
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	})

	rootCmd.AddCommand(configCmd)
}

func printConfigOverview(cfg *config.RuntimeConfig) {
	_, configPath, err := config.LoadFileConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("Error loading config path: %v", err)))
		os.Exit(1)
	}

	dashPort := cfg.Port
	dashboardURL := fmt.Sprintf("http://localhost:%s", dashPort)
	running := server.CheckPinchTabRunning(dashPort, cfg.Token)

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "Config"))
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "strategy", cli.StyleStdout(cli.ValueStyle, cfg.Strategy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "allocation policy", cli.StyleStdout(cli.ValueStyle, cfg.AllocationPolicy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "stealth level", cli.StyleStdout(cli.ValueStyle, cfg.StealthLevel))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "tab eviction", cli.StyleStdout(cli.ValueStyle, cfg.TabEvictionPolicy))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "tab lifecycle", cli.StyleStdout(cli.ValueStyle, formatTabLifecycle(cfg)))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "file", cli.StyleStdout(cli.ValueStyle, configPath))
	_, _ = fmt.Fprintf(out, "  %-20s %s\n", "token", cli.StyleStdout(cli.ValueStyle, config.MaskToken(cfg.Token)))
	if running {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "dashboard", cli.StyleStdout(cli.ValueStyle, dashboardURL))
	} else {
		_, _ = fmt.Fprintf(out, "  %-20s %s\n", "dashboard", cli.StyleStdout(cli.MutedStyle, "not running"))
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, cli.StyleStdout(cli.HeadingStyle, "Change config:"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config get <path>"), cli.StyleStdout(cli.MutedStyle, "# read a value (e.g. server.port)"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config set <path> <value>"), cli.StyleStdout(cli.MutedStyle, "# update a value"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config show"), cli.StyleStdout(cli.MutedStyle, "# print labelled config summary"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab config token"), cli.StyleStdout(cli.MutedStyle, "# copy API token to clipboard"))
	_, _ = fmt.Fprintf(out, "  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab security"), cli.StyleStdout(cli.MutedStyle, "# review security posture"))
	_, _ = fmt.Fprintf(out, "  %s %s\n", cli.StyleStdout(cli.MutedStyle, "Or edit the file directly:"), cli.StyleStdout(cli.ValueStyle, configPath))
}

func formatTabLifecycle(cfg *config.RuntimeConfig) string {
	policy := cfg.TabLifecyclePolicy
	if policy == "" {
		policy = "keep"
	}
	if policy == "close_idle" && cfg.TabCloseDelay > 0 {
		return fmt.Sprintf("%s (%s)", policy, cfg.TabCloseDelay)
	}
	return policy
}

func copyConfigToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("server token is empty")
	}

	if err := copyToClipboard(token); err == nil {
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "Token copied to clipboard."))
		return nil
	}

	fmt.Println(cli.StyleStdout(cli.WarningStyle, "Clipboard unavailable; token not shown for safety."))
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

func handleConfigTokenCopy() {
	cfg := loadLocalConfig()
	if err := copyConfigToken(cfg.Token); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(displayConfigValue(path, value))
}

func handleConfigSet(path, value string) error {
	change, err := workflow.PrepareSetValue(path, value)
	if err != nil {
		return err
	}

	if errs := change.ValidationErrors; len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: new value causes validation error(s):\n")
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		if !confirmSaveAnyway() {
			return fmt.Errorf("aborted: value not saved")
		}
	}

	if err := workflow.SavePreparedChange(change); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", path, displayConfigValue(path, value))
	return nil
}

// confirmSaveAnyway prompts on a TTY; on non-TTY (agent / pipe) it returns
// false so the validation warning blocks the save by default.
func confirmSaveAnyway() bool {
	if !isInteractiveTerminal() {
		return false
	}
	fmt.Print("Save anyway? (y/N): ")
	var response string
	_, _ = fmt.Scanln(&response)
	return response == "y" || response == "Y"
}

func displayConfigValue(path, value string) string {
	if strings.EqualFold(strings.TrimSpace(path), "server.token") {
		return config.MaskToken(value)
	}
	return value
}

func handleConfigPatch(jsonPatch string) error {
	change, err := workflow.PreparePatch(jsonPatch)
	if err != nil {
		return err
	}

	if errs := change.ValidationErrors; len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: patch causes validation error(s):\n")
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		if !confirmSaveAnyway() {
			return fmt.Errorf("aborted: patch not saved")
		}
	}

	if err := workflow.SavePreparedChange(change); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println("Config patched successfully")
	return nil
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

func handleConfigSchema(printSchema bool) {
	schemaURL := config.CurrentConfigSchemaURL()
	if printSchema {
		schemaJSON, err := configschema.ConfigJSONForURL(schemaURL)
		if err != nil {
			fmt.Printf("Error rendering schema: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(schemaJSON); err != nil {
			fmt.Printf("Error writing schema: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println(schemaURL)
}
