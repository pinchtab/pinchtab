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

func TestNormalizeRequiredURL(t *testing.T) {
	t.Run("normalizes bare hostname", func(t *testing.T) {
		got := normalizeRequiredURL("pinchtab.com")
		if got != "https://pinchtab.com" {
			t.Fatalf("normalizeRequiredURL() = %q, want %q", got, "https://pinchtab.com")
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got := normalizeRequiredURL("  https://pinchtab.com  ")
		if got != "https://pinchtab.com" {
			t.Fatalf("normalizeRequiredURL() = %q, want %q", got, "https://pinchtab.com")
		}
	})
}

func TestMouseCommandGroupRegistered(t *testing.T) {
	m := findCommand(rootCmd, "mouse")
	if m == nil {
		t.Fatal("expected mouse command to be registered")
	}
	if m.GroupID != "browser" {
		t.Fatalf("expected mouse command group browser, got %q", m.GroupID)
	}
}

func TestMouseSubCommandsRegistered(t *testing.T) {
	m := findCommand(rootCmd, "mouse")
	if m == nil {
		t.Fatal("expected mouse command to be registered")
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

func TestTabHandoffCommandsRegistered(t *testing.T) {
	for _, name := range []string{"handoff [id]", "resume [id]", "handoff-status [id]"} {
		if findCommand(rootCmd, name) == nil {
			t.Fatalf("expected top-level command %q to be registered", name)
		}
	}

	tabCmd := findCommand(rootCmd, "tab [id]")
	if tabCmd == nil {
		t.Fatal("expected tab command to be registered")
	}
	for _, name := range []string{"handoff [id]", "resume [id]", "handoff-status [id]"} {
		if findCommand(tabCmd, name) == nil {
			t.Fatalf("expected tab subcommand %q to be registered", name)
		}
	}
}
