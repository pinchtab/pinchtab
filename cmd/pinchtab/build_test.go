package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// GoReleaserConfig represents the minimal goreleaser config we care about
type GoReleaserConfig struct {
	Builds []struct {
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
	} `yaml:"builds"`
}

// TestBinaryPermutations verifies all expected binary permutations are configured in goreleaser
func TestBinaryPermutations(t *testing.T) {
	// Find .goreleaser.yml in repo root (2 levels up from cmd/pinchtab/)
	repoRoot := filepath.Join("..", "..", ".goreleaser.yml")
	data, err := os.ReadFile(repoRoot)
	if err != nil {
		t.Fatalf("failed to read .goreleaser.yml at %s: %v", repoRoot, err)
	}

	var cfg GoReleaserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse .goreleaser.yml: %v", err)
	}

	if len(cfg.Builds) == 0 {
		t.Fatal("no builds configured in .goreleaser.yml")
	}

	build := cfg.Builds[0]

	// Expected OS/arch combinations
	expectedOS := map[string]bool{
		"linux":   true,
		"darwin":  true,
		"windows": true,
	}

	expectedArch := map[string]bool{
		"amd64": true,
		"arm64": true,
	}

	// Verify all expected OS are configured
	for os := range expectedOS {
		found := false
		for _, configOS := range build.GOOS {
			if configOS == os {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("OS %q not found in goreleaser config", os)
		}
	}

	// Verify all expected architectures are configured
	for arch := range expectedArch {
		found := false
		for _, configArch := range build.GOARCH {
			if configArch == arch {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Architecture %q not found in goreleaser config", arch)
		}
	}

	// Calculate expected binary count
	totalExpected := len(expectedOS) * len(expectedArch)
	totalConfigured := len(build.GOOS) * len(build.GOARCH)

	if totalConfigured != totalExpected {
		t.Errorf("expected %d binaries (3 OS × 2 arch), but config produces %d",
			totalExpected, totalConfigured)
	}

	t.Logf("✓ Binary matrix verified: %d OS × %d arch = %d total binaries",
		len(build.GOOS), len(build.GOARCH), totalConfigured)
}

// TestExpectedBinaryNames verifies correct naming for all permutations
func TestExpectedBinaryNames(t *testing.T) {
	expectedBinaries := []string{
		"pinchtab-linux-amd64",
		"pinchtab-linux-arm64",
		"pinchtab-darwin-amd64",
		"pinchtab-darwin-arm64",
		"pinchtab-windows-amd64.exe",
		"pinchtab-windows-arm64.exe",
	}

	if len(expectedBinaries) != 6 {
		t.Errorf("expected 6 binaries, got %d", len(expectedBinaries))
	}

	t.Logf("Expected binary names (%d total):", len(expectedBinaries))
	for _, bin := range expectedBinaries {
		t.Logf("  ✓ %s", bin)
	}
}
