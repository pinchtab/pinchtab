package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cloakImage is the prebuilt image the cloak provider expects to find locally.
// It is built by tests/tools/docker/cloakbrowser-smoke.Dockerfile and is
// intentionally never auto-built by the e2e runner — it is too heavy and the
// Cloak binary license requires explicit opt-in.
const cloakImage = "pinchtab-cloakbrowser:test"
const dryRunCloakComposeOverride = "<generated-cloak-compose-override>"

// pinchtabServices enumerates every pinchtab variant present in the e2e
// compose files. Anything not listed here is left untouched by the cloak
// override (notably the runner-* services and fixtures).
var pinchtabServices = []string{
	"pinchtab",
	"pinchtab-secure",
	"pinchtab-autoclose",
	"pinchtab-medium",
	"pinchtab-full",
	"pinchtab-lite",
	"pinchtab-bridge",
}

// providerOverrides bundles the override compose file + any generated cloak
// configs the runner mounts in place of the chrome configs.
type providerOverrides struct {
	provider     string
	tmpDir       string
	composeFiles []string
}

// preparePoviderOverrides creates a temp directory with cloak-flavoured copies
// of every tests/e2e/config/*.json the suite mounts, plus a compose override
// file that swaps each pinchtab service's image and config bind mount.
//
// For provider=chrome it is a no-op and returns nil; the runner runs the suite
// byte-identical to today.
func (r *Runner) prepareProviderOverrides() (*providerOverrides, error) {
	if r.args.Provider == "" || r.args.Provider == "chrome" {
		return nil, nil
	}
	if r.args.Provider != "cloak" {
		return nil, fmt.Errorf("unsupported provider: %s", r.args.Provider)
	}

	if r.args.DryRun {
		return &providerOverrides{
			provider:     "cloak",
			composeFiles: []string{dryRunCloakComposeOverride},
		}, nil
	}

	if err := assertCloakImagePresent(); err != nil {
		return nil, err
	}

	tmp, err := os.MkdirTemp("", "pinchtab-e2e-cloak-*")
	if err != nil {
		return nil, fmt.Errorf("create cloak tmp dir: %w", err)
	}

	configDir := filepath.Join(r.repoRoot, "tests/e2e/config")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("read e2e config dir: %w", err)
	}

	cloakConfigs := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		src := filepath.Join(configDir, name)
		dst := filepath.Join(tmp, name)
		if err := writeCloakConfig(src, dst); err != nil {
			_ = os.RemoveAll(tmp)
			return nil, fmt.Errorf("rewrite config %s: %w", name, err)
		}
		cloakConfigs[name] = dst
	}

	overridePath := filepath.Join(tmp, "docker-compose.cloak.yml")
	fixturesDir := filepath.Join(r.repoRoot, "tests/e2e/fixtures")
	if err := writeCloakComposeOverride(overridePath, cloakConfigs, fixturesDir); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("write cloak compose override: %w", err)
	}

	return &providerOverrides{
		provider:     "cloak",
		tmpDir:       tmp,
		composeFiles: []string{overridePath},
	}, nil
}

func (o *providerOverrides) cleanup() {
	if o == nil || o.tmpDir == "" {
		return
	}
	_ = os.RemoveAll(o.tmpDir)
}

func assertCloakImagePresent() error {
	cmd := exec.Command("docker", "image", "inspect", cloakImage)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"%s image not found; build via tests/tools/docker/cloakbrowser-smoke.Dockerfile or `./dev smoke cloakbrowser` first",
			cloakImage,
		)
	}
	return nil
}

// writeCloakConfig reads a chrome-flavoured pinchtab config and writes a
// cloak-flavoured copy: it preserves every other field and injects the
// browser.provider + browser.binary + browser.cloak block. The fingerprint
// seed mirrors scripts/lib/smoke-config.sh for consistency.
func writeCloakConfig(src, dst string) error {
	raw, err := os.ReadFile(src) // #nosec G304 -- src is constrained to tests/e2e/config/*.json.
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	browser, _ := cfg["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{}
	}
	browser["provider"] = "cloak"
	browser["binary"] = "/opt/cloakbrowser/chrome"
	browser["cloak"] = map[string]any{
		"fingerprintSeed":           "42069",
		"platform":                  "linux",
		"locale":                    "en-US",
		"timezone":                  "UTC",
		"disableDefaultStealthArgs": true,
	}
	cfg["browser"] = browser

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644) // #nosec G306 -- read-only config fixture.
}

// writeCloakComposeOverride writes a compose file that targets each pinchtab
// service: swaps the image to the prebuilt cloak image, drops the build
// directive (via image-only + pull_policy:never), remaps the config bind mount
// onto the cloak-flavoured copy we just generated, and preserves extension
// fixture mounts needed by extension-parity E2E tests.
//
// The override is intentionally written as YAML by hand (no third-party deps)
// since the structure is small and stable.
func writeCloakComposeOverride(path string, cloakConfigs map[string]string, fixturesDir string) error {
	type extensionMount struct {
		sourceDir string
		target    string
	}
	type serviceOverrideConfig struct {
		configName      string
		extensionMounts []extensionMount
	}

	// Map service name to original config basename and extension bind mounts.
	// The config bind-mount target inside the container is
	// /fixture-config/<basename>:ro, which the entrypoint command copies into
	// /data/e2e-config/<basename>.
	serviceConfig := map[string]serviceOverrideConfig{
		"pinchtab": {
			configName: "pinchtab.json",
			extensionMounts: []extensionMount{
				{sourceDir: "test-extension", target: "/extensions/test-extension"},
				{sourceDir: "test-extension-api", target: "/extensions/test-extension-api"},
			},
		},
		"pinchtab-secure": {
			configName: "pinchtab-secure.json",
		},
		"pinchtab-autoclose": {
			configName: "pinchtab-autoclose.json",
			extensionMounts: []extensionMount{
				{sourceDir: "test-extension", target: "/extensions/test-extension"},
			},
		},
		"pinchtab-medium": {
			configName: "pinchtab-medium-permissive.json",
			extensionMounts: []extensionMount{
				{sourceDir: "test-extension", target: "/extensions/test-extension"},
			},
		},
		"pinchtab-full": {
			configName: "pinchtab-full-permissive.json",
			extensionMounts: []extensionMount{
				{sourceDir: "test-extension", target: "/extensions/test-extension"},
			},
		},
		"pinchtab-lite": {
			configName: "pinchtab-lite.json",
		},
		"pinchtab-bridge": {
			configName: "pinchtab-bridge.json",
		},
	}

	var b strings.Builder
	b.WriteString("# Generated by tests/tools/runner — provider=cloak override.\n")
	b.WriteString("# Swaps every pinchtab* service onto the prebuilt cloak image and\n")
	b.WriteString("# mounts the cloak-flavoured runtime config in place of the chrome one.\n")
	b.WriteString("services:\n")

	for _, svc := range pinchtabServices {
		cfg, ok := serviceConfig[svc]
		if !ok {
			continue
		}
		baseName := cfg.configName
		cloakPath, ok := cloakConfigs[baseName]
		if !ok {
			// No config generated (probably renamed); skip silently — the
			// service will fail at startup and surface a clear error.
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", svc)
		fmt.Fprintf(&b, "    image: %s\n", cloakImage)
		fmt.Fprintf(&b, "    pull_policy: never\n")
		// Keep the override self-contained: every provider=cloak service gets
		// the rewritten config plus the same extension fixtures its Chrome
		// counterpart exposes in the base compose files.
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/fixture-config/%s:ro\n", cloakPath, baseName)
		for _, mount := range cfg.extensionMounts {
			source := filepath.Join(fixturesDir, mount.sourceDir)
			fmt.Fprintf(&b, "      - %s:%s:ro\n", source, mount.target)
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644) // #nosec G306 -- consumed read-only by docker compose.
}

// assertStealthStatus runs the one-shot pre-suite check against the running
// PinchTab container. We hit /stealth/status on the host-exposed port (9999)
// with the well-known e2e token; for cloak we assert the full claim set; for
// chrome we sanity-check the provider field only.
func (r *Runner) assertStealthStatus(composeFile string) error {
	if r.args.DryRun {
		return nil
	}
	host, port, err := r.resolvePinchtabHostPort(composeFile)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%s/stealth/status", host, port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build stealth status request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer e2e-token")

	client := &http.Client{Timeout: 15 * time.Second}

	var resp *http.Response
	deadline := time.Now().Add(45 * time.Second)
	for {
		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("stealth status check failed: %w", err)
			}
			return fmt.Errorf("stealth status check returned %d", resp.StatusCode)
		}
		time.Sleep(500 * time.Millisecond)
	}
	defer func() { _ = resp.Body.Close() }()

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode stealth status: %w", err)
	}

	provider, _ := status["provider"].(string)

	switch r.args.Provider {
	case "cloak":
		native, _ := status["native"].(bool)
		overlays, _ := status["pinchtabOverlaysDisabled"].(bool)
		seed, _ := status["fingerprintSeed"].(string)
		if provider != "cloak" || !native || !overlays || seed != "42069" {
			return fmt.Errorf(
				"cloak stealth assertion failed (want provider=cloak, native=true, pinchtabOverlaysDisabled=true, fingerprintSeed=42069); got: %s",
				formatStealthStatus(status),
			)
		}
		_, _ = fmt.Fprintf(
			r.stdout,
			"  /stealth/status: provider=cloak native=true overlaysDisabled=true seed=%s\n",
			seed,
		)
	default:
		// chrome — sanity check only.
		if provider != "chrome" {
			return fmt.Errorf("chrome stealth assertion failed (want provider=chrome); got: %s", formatStealthStatus(status))
		}
		_, _ = fmt.Fprintln(r.stdout, "  /stealth/status: provider=chrome")
	}
	return nil
}

func formatStealthStatus(status map[string]any) string {
	pretty, err := json.MarshalIndent(status, "  ", "  ")
	if err != nil {
		return fmt.Sprintf("%v", status)
	}
	return string(pretty)
}

// resolvePinchtabHostPort returns the host-side bind address+port that maps to
// the `pinchtab` service inside the compose project. We prefer the published
// fixed port (9999 in both compose files) but fall back to `compose port` for
// resilience against ephemeral port mappings.
func (r *Runner) resolvePinchtabHostPort(composeFile string) (string, string, error) {
	args := r.composeArgs(composeFile, "port", "pinchtab", "9999")
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- compose args are runner-controlled.
	cmd.Dir = r.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "127.0.0.1", "9999", nil
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "127.0.0.1", "9999", nil
	}
	// `compose port` prints e.g. "0.0.0.0:9999" — split on last colon.
	idx := strings.LastIndex(line, ":")
	if idx <= 0 || idx == len(line)-1 {
		return "127.0.0.1", "9999", nil
	}
	host := line[:idx]
	port := line[idx+1:]
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return host, port, nil
}
