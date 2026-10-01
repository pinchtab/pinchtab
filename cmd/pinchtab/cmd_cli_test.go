package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func findCommand(cmd *cobra.Command, use string) *cobra.Command {
	if cmd == nil {
		return nil
	}
	if cmd.Name() == use || cmd.Use == use {
		return cmd
	}
	for _, child := range cmd.Commands() {
		if found := findCommand(child, use); found != nil {
			return found
		}
	}
	return nil
}

func findDirectCommand(cmd *cobra.Command, use string) *cobra.Command {
	if cmd == nil {
		return nil
	}
	for _, child := range cmd.Commands() {
		if child.Name() == use || child.Use == use {
			return child
		}
	}
	return nil
}

func TestMouseCommandGroupRegistered(t *testing.T) {
	m := findCommand(rootCmd, "mouse")
	if m == nil {
		t.Fatal("expected mouse command to be registered")
		return
	}
	if m.GroupID != "browser" {
		t.Fatalf("expected mouse command group browser, got %q", m.GroupID)
	}
}

func TestMouseSubCommandsRegistered(t *testing.T) {
	m := findCommand(rootCmd, "mouse")
	if m == nil {
		t.Fatal("expected mouse command to be registered")
		return
	}
	for _, name := range []string{"move", "down", "up", "wheel"} {
		if findCommand(m, name) == nil {
			t.Fatalf("expected mouse subcommand %q to be registered", name)
		}
	}
}

func TestDragCommandRegistered(t *testing.T) {
	if findCommand(rootCmd, "drag") == nil {
		t.Fatal("expected drag command to be registered")
	}
}

func TestTabManagementCommandsRegistered(t *testing.T) {
	for _, name := range []string{"close <id>", "handoff [id]", "resume [id]", "handoff-status [id]"} {
		if findDirectCommand(rootCmd, name) == nil {
			t.Fatalf("expected top-level command %q to be registered", name)
		}
	}

	tabCmd := findDirectCommand(rootCmd, "tab [id]")
	if tabCmd == nil {
		t.Fatal("expected tab command to be registered")
		return
	}
	for _, name := range []string{"close <id>", "handoff [id]", "resume [id]", "handoff-status [id]"} {
		if findDirectCommand(tabCmd, name) == nil {
			t.Fatalf("expected tab subcommand %q to be registered", name)
		}
	}

	rootClose := findDirectCommand(rootCmd, "close <id>")
	tabClose := findDirectCommand(tabCmd, "close <id>")
	if rootClose == tabClose {
		t.Fatal("top-level close and tab close should be separate cobra commands")
	}
	if got := rootClose.CommandPath(); got != "pinchtab close" {
		t.Fatalf("root close path = %q, want pinchtab close", got)
	}
	if got := tabClose.CommandPath(); got != "pinchtab tab close" {
		t.Fatalf("tab close path = %q, want pinchtab tab close", got)
	}
}
