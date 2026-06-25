package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
	configschema "github.com/pinchtab/pinchtab/internal/schema"
	"github.com/pinchtab/pinchtab/internal/server"
)

func handleConfigTokenCopy() {
	cfg := loadLocalConfig()
	if err := copyConfigToken(cfg.Token); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
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
	return applyPreparedChange(change, "new value", "value",
		fmt.Sprintf("Set %s = %s", path, displayConfigValue(path, value)))
}

func handleConfigPatch(jsonPatch string) error {
	change, err := workflow.PreparePatch(jsonPatch)
	if err != nil {
		return err
	}
	return applyPreparedChange(change, "patch", "patch", "Config patched successfully")
}

// applyPreparedChange reviews validation warnings (gating on confirmSaveAnyway),
// saves the change, prints the success message, and emits the restart hint — the
// shared mutate path for `config set` and `config patch` so they cannot drift.
func applyPreparedChange(change *workflow.PreparedChange, warningNoun, abortNoun, successMessage string) error {
	if errs := change.ValidationErrors; len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s causes validation error(s):\n", warningNoun)
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		if !confirmSaveAnyway() {
			return fmt.Errorf("aborted: %s not saved", abortNoun)
		}
	}

	if err := workflow.SavePreparedChange(change); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println(successMessage)
	hintRestartIfRunning()
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
	if isSensitiveConfigPath(path) {
		return config.MaskToken(value)
	}
	return value
}

// isSensitiveConfigPath reports whether a config path points at a secret
// (token/password/credential) so its value is masked before being printed.
func isSensitiveConfigPath(path string) bool {
	segments := strings.Split(strings.ToLower(strings.TrimSpace(path)), ".")
	last := segments[len(segments)-1]
	switch last {
	case "token", "password", "secret", "apikey", "apisecret":
		return true
	}
	return false
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

func hintRestartIfRunning() {
	cfg := loadLocalConfig()
	if server.CheckPinchTabRunning(cfg.Port, cfg.Token) {
		output.Hint("Server is running — restart it to apply changes: pinchtab server restart")
	}
}
