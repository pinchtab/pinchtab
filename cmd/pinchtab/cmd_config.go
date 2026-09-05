package main

import (
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/spf13/cobra"
)

// adviseDefaultConfig is the one printer of the custom-PINCHTAB_CONFIG advisory:
// the text is decided in config, the empty string means it does not apply, and
// output.Advisory owns the switch and the once-per-install dedupe.
func adviseDefaultConfig() {
	if text := config.DefaultConfigAdvisory(); text != "" {
		output.Advisory(text)
	}
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		adviseDefaultConfig()
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
	configTokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Copy the API token to clipboard",
		Long: "Copies the configured server.token to the system clipboard. Nothing is printed to stdout " +
			"unless you pass --stdout, which prints the token and nothing else so it can be captured " +
			"with $(...) on a host with no clipboard tool.",
		Run: func(cmd *cobra.Command, args []string) {
			toStdout, _ := cmd.Flags().GetBool("stdout")
			handleConfigTokenCopy(toStdout)
		},
	}
	configTokenCmd.Flags().Bool("stdout", false, "Print the token to stdout instead of copying it to the clipboard")
	configCmd.AddCommand(configTokenCmd)
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
