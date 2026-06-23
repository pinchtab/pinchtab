package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var clipboardExecCommand = exec.Command

type clipboardCommand struct {
	name string
	args []string
}

// clipboardCommands returns the platform-appropriate clipboard tools to try, in
// preference order. Linux has several depending on the display server, so it
// returns multiple candidates.
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
