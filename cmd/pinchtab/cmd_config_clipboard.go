package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var (
	clipboardExecCommand = exec.CommandContext
	clipboardLookPath    = exec.LookPath
	clipboardTimeout     = 2 * time.Second
)

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
		if _, err := clipboardLookPath(candidate.name); err != nil {
			lastErr = err
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
		cmd := clipboardExecCommand(ctx, candidate.name, candidate.args...)
		cmd.Stdin = strings.NewReader(text)
		output, err := cmd.CombinedOutput()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			lastErr = fmt.Errorf("%s timed out after %s", candidate.name, clipboardTimeout)
			continue
		}
		if err != nil {
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
