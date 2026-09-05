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

func handleConfigTokenCopy(toStdout bool) {
	cfg := loadLocalConfig()
	if err := emitConfigToken(cfg.Token, toStdout); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
}

// emitConfigToken puts the token where the caller asked for it. Every human
// message goes to stderr: stdout is the machine channel, so
// TOKEN=$(pinchtab config token) captures the token or nothing, never prose
// that would then be sent as a bearer credential.
//
// A clipboard failure is an error, not a notice. It used to print and return
// nil, so on the headless hosts where agents actually run the command exited 0
// having done nothing and named no other way to get the token.
func emitConfigToken(token string, toStdout bool) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("server token is empty")
	}

	if toStdout {
		fmt.Println(token)
		return nil
	}

	if err := copyToClipboard(token); err != nil {
		return fmt.Errorf("clipboard unavailable (%v); run `pinchtab config token --stdout` to print it, or read server.token from %s",
			err, workflow.CurrentConfigPath())
	}

	fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.SuccessStyle, "Token copied to clipboard."))
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
	fmt.Fprintf(os.Stderr, "pinchtab: generated server.token in %s\n", configPath)
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
//
// Advisories are printed and never gated. They were gated once, when the validator had a
// single severity, and one inert key in a file then aborted every later write — off a TTY,
// where confirmSaveAnyway cannot ask, so agents could not save anything at all. Gating
// still applies to real validation errors, which is what stops an out-of-range port
// slipping through unnoticed.
func applyPreparedChange(change *workflow.PreparedChange, warningNoun, abortNoun, successMessage string) error {
	printConfigAdvisories(config.FileConfigAdvisories(change.FileConfig))

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

// printConfigAdvisories reports settings the file carries that PinchTab does not act on.
// stderr, so it never lands in piped output a caller is parsing, and no exit code: an
// advisory describes something inert, and there is nothing for the reader to do about it.
func printConfigAdvisories(advisories []string) {
	for _, advisory := range advisories {
		fmt.Fprintf(os.Stderr, "Note: %s\n", advisory)
	}
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
	if config.IsSensitiveConfigPath(path) {
		return config.MaskToken(value)
	}
	return value
}

func handleConfigValidate() {
	configPath, errs, advisories, err := workflow.ValidateCurrentFile()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	printConfigAdvisories(advisories)

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
