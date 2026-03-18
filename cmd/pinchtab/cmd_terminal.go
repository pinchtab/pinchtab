package main

import "os"

func isInteractiveTerminal() bool {
	in, err := os.Stdin.Stat()
	if err != nil || (in.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	out, err := os.Stdout.Stat()
	if err != nil || (out.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	return true
}
