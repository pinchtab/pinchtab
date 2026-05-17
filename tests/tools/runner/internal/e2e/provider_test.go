package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCloakComposeOverrideMountsExtensionFixtures(t *testing.T) {
	tmp := t.TempDir()
	fixturesDir := filepath.Join(tmp, "fixtures")
	outPath := filepath.Join(tmp, "docker-compose.cloak.yml")

	configs := map[string]string{
		"pinchtab.json":                   filepath.Join(tmp, "pinchtab.json"),
		"pinchtab-secure.json":            filepath.Join(tmp, "pinchtab-secure.json"),
		"pinchtab-autoclose.json":         filepath.Join(tmp, "pinchtab-autoclose.json"),
		"pinchtab-medium-permissive.json": filepath.Join(tmp, "pinchtab-medium-permissive.json"),
		"pinchtab-full-permissive.json":   filepath.Join(tmp, "pinchtab-full-permissive.json"),
		"pinchtab-lite.json":              filepath.Join(tmp, "pinchtab-lite.json"),
		"pinchtab-bridge.json":            filepath.Join(tmp, "pinchtab-bridge.json"),
	}

	if err := writeCloakComposeOverride(outPath, configs, fixturesDir); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)

	for _, svc := range []string{"pinchtab", "pinchtab-autoclose", "pinchtab-medium", "pinchtab-full"} {
		block := serviceBlock(t, compose, svc)
		want := filepath.Join(fixturesDir, "test-extension") + ":/extensions/test-extension:ro"
		if !strings.Contains(block, want) {
			t.Fatalf("%s override missing extension mount %q:\n%s", svc, want, block)
		}
	}

	pinchtabBlock := serviceBlock(t, compose, "pinchtab")
	wantAPIExtension := filepath.Join(fixturesDir, "test-extension-api") + ":/extensions/test-extension-api:ro"
	if !strings.Contains(pinchtabBlock, wantAPIExtension) {
		t.Fatalf("pinchtab override missing extension API mount %q:\n%s", wantAPIExtension, pinchtabBlock)
	}

	for _, svc := range []string{"pinchtab-secure", "pinchtab-lite", "pinchtab-bridge"} {
		block := serviceBlock(t, compose, svc)
		if strings.Contains(block, "/extensions/test-extension") {
			t.Fatalf("%s override should not add extension mounts:\n%s", svc, block)
		}
	}

	for svc, configName := range map[string]string{
		"pinchtab":           "pinchtab.json",
		"pinchtab-secure":    "pinchtab-secure.json",
		"pinchtab-autoclose": "pinchtab-autoclose.json",
		"pinchtab-medium":    "pinchtab-medium-permissive.json",
		"pinchtab-full":      "pinchtab-full-permissive.json",
		"pinchtab-lite":      "pinchtab-lite.json",
		"pinchtab-bridge":    "pinchtab-bridge.json",
	} {
		block := serviceBlock(t, compose, svc)
		want := configs[configName] + ":/fixture-config/" + configName + ":ro"
		if !strings.Contains(block, want) {
			t.Fatalf("%s override missing cloak config mount %q:\n%s", svc, want, block)
		}
		if !strings.Contains(block, "image: "+cloakImage) {
			t.Fatalf("%s override missing cloak image:\n%s", svc, block)
		}
		if !strings.Contains(block, "pull_policy: never") {
			t.Fatalf("%s override missing pull_policy: never:\n%s", svc, block)
		}
	}
}

func TestWriteCloakConfigPreservesExtensionPaths(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "pinchtab.json")
	dst := filepath.Join(tmp, "pinchtab.cloak.json")

	input := `{
  "browser": {
    "extensionPaths": ["/extensions/test-extension"]
  },
  "security": {
    "allowedDomains": ["fixtures"]
  }
}`
	if err := os.WriteFile(src, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeCloakConfig(src, dst); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"provider": "cloak"`,
		`"binary": "/opt/cloakbrowser/chrome"`,
		`"extensionPaths": [`,
		`"/extensions/test-extension"`,
		`"allowedDomains": [`,
		`"fixtures"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloak config missing %q:\n%s", want, out)
		}
	}
}

func serviceBlock(t *testing.T, compose, service string) string {
	t.Helper()

	marker := "  " + service + ":\n"
	start := strings.Index(compose, marker)
	if start < 0 {
		t.Fatalf("service %s not found in compose:\n%s", service, compose)
	}
	var lines []string
	for _, line := range strings.Split(compose[start+len(marker):], "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
