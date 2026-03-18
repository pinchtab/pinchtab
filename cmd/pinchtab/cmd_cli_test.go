package main

import "testing"

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
