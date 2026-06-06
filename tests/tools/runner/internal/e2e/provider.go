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

const defaultCloakImage = "pinchtab-cloakbrowser:test"
const defaultCloakDockerfile = "tests/tools/docker/cloakbrowser-smoke.Dockerfile"
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
	"pinchtab-ghostchrome",
	"pinchtab-bridge",
}

// providerOverrides bundles the override compose file + any generated cloak
// configs the runner mounts in place of the chrome configs.
type providerOverrides struct {
	provider     string
	image        string
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
	switch r.args.Provider {
	case "cloak":
		return r.prepareCloakOverrides()
	case "ghost-chrome":
		return r.prepareGhostChromeOverrides()
	default:
		return nil, fmt.Errorf("unsupported provider: %s", r.args.Provider)
	}
}

func (r *Runner) prepareCloakOverrides() (*providerOverrides, error) {
	image := cloakProviderImage()

	if r.args.DryRun {
		return &providerOverrides{
			provider:     "cloak",
			image:        image,
			composeFiles: []string{dryRunCloakComposeOverride},
		}, nil
	}

	if err := r.ensureCloakImage(image); err != nil {
		return nil, err
	}

	tmp, err := os.MkdirTemp("", "pinchtab-e2e-cloak-*")
	if err != nil {
		return nil, fmt.Errorf("create cloak tmp dir: %w", err)
	}

	configs, err := r.rewriteE2EConfigs(tmp, writeCloakConfig)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	overridePath := filepath.Join(tmp, "docker-compose.cloak.yml")
	fixturesDir := filepath.Join(r.repoRoot, "tests/e2e/fixtures")
	if err := writeCloakComposeOverride(overridePath, configs, fixturesDir, image); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("write cloak compose override: %w", err)
	}

	return &providerOverrides{
		provider:     "cloak",
		image:        image,
		tmpDir:       tmp,
		composeFiles: []string{overridePath},
	}, nil
}

func (r *Runner) prepareGhostChromeOverrides() (*providerOverrides, error) {
	if r.args.DryRun {
		return &providerOverrides{
			provider:     "ghost-chrome",
			composeFiles: []string{dryRunCloakComposeOverride},
		}, nil
	}

	tmp, err := os.MkdirTemp("", "pinchtab-e2e-ghost-chrome-*")
	if err != nil {
		return nil, fmt.Errorf("create ghost-chrome tmp dir: %w", err)
	}

	configs, err := r.rewriteE2EConfigs(tmp, writeGhostChromeConfig)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	overridePath := filepath.Join(tmp, "docker-compose.ghost-chrome.yml")
	fixturesDir := filepath.Join(r.repoRoot, "tests/e2e/fixtures")
	if err := writeConfigOnlyComposeOverride(overridePath, configs, fixturesDir); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("write ghost-chrome compose override: %w", err)
	}

	return &providerOverrides{
		provider:     "ghost-chrome",
		tmpDir:       tmp,
		composeFiles: []string{overridePath},
	}, nil
}

// rewriteE2EConfigs reads every JSON file in tests/e2e/config/ and writes a
// transformed copy using rewriteFn. Returns a map from basename to rewritten path.
func (r *Runner) rewriteE2EConfigs(tmpDir string, rewriteFn func(src, dst string) error) (map[string]string, error) {
	configDir := filepath.Join(r.repoRoot, "tests/e2e/config")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("read e2e config dir: %w", err)
	}

	configs := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		src := filepath.Join(configDir, name)
		dst := filepath.Join(tmpDir, name)
		if err := rewriteFn(src, dst); err != nil {
			return nil, fmt.Errorf("rewrite config %s: %w", name, err)
		}
		configs[name] = dst
	}
	return configs, nil
}

func (o *providerOverrides) cleanup() {
	if o == nil || o.tmpDir == "" {
		return
	}
	_ = os.RemoveAll(o.tmpDir)
}

func cloakProviderImage() string {
	for _, env := range []string{"PINCHTAB_E2E_CLOAK_IMAGE", "PINCHTAB_PARITY_CLOAK_IMAGE"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v
		}
	}
	return defaultCloakImage
}

func cloakProviderDockerfile() string {
	if v := strings.TrimSpace(os.Getenv("PINCHTAB_CLOAKBROWSER_DOCKERFILE")); v != "" {
		return v
	}
	return defaultCloakDockerfile
}

func (r *Runner) ensureCloakImage(image string) error {
	if strings.TrimSpace(os.Getenv("SKIP_BUILD")) == "1" {
		present, err := dockerImagePresent(image)
		if err != nil {
			return err
		}
		if present {
			_, _ = fmt.Fprintf(r.stdout, "  provider image: reusing %s (SKIP_BUILD=1)\n", image)
			return nil
		}
		return fmt.Errorf("%s image not found and SKIP_BUILD=1 is set; build with %s or unset SKIP_BUILD", image, cloakProviderDockerfile())
	}

	dockerfile := cloakProviderDockerfile()
	dockerfilePath := dockerfile
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(r.repoRoot, dockerfilePath)
	}
	if _, err := os.Stat(dockerfilePath); err != nil {
		return fmt.Errorf("cloak Dockerfile not found at %s: %w", dockerfile, err)
	}

	code := r.runLoggedCommand(
		"building Cloak provider image",
		stackOutput,
		[]string{"docker", "build", "--load", "-f", dockerfilePath, "-t", image, r.repoRoot},
	)
	if code != 0 {
		return fmt.Errorf("build Cloak provider image failed (exit %d)", code)
	}
	return nil
}

func dockerImagePresent(image string) (bool, error) {
	cmd := exec.Command("docker", "image", "inspect", image)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	msg := strings.TrimSpace(string(out))
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "no such image") || strings.Contains(lower, "not found") {
		return false, nil
	}
	if msg == "" {
		return false, fmt.Errorf("inspect Docker image %s: %w", image, err)
	}
	return false, fmt.Errorf("inspect Docker image %s: %w: %s", image, err, msg)
}

// writeCloakConfig reads a chrome-flavoured pinchtab config and writes a
// cloak-flavoured copy: it sets browsers.default, browser.binary, and the
// browser.cloak block. The fingerprint seed mirrors scripts/lib/smoke-config.sh.
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
	browser["binary"] = "/opt/cloakbrowser/chrome"
	browser["cloak"] = map[string]any{
		"fingerprintSeed":           "42069",
		"platform":                  "linux",
		"locale":                    "en-US",
		"timezone":                  "UTC",
		"disableDefaultStealthArgs": true,
	}
	cfg["browser"] = browser

	// Set configVersion so the startup wizard doesn't rewrite the file,
	// and browsers.default to select the cloak browser provider.
	cfg["configVersion"] = "0.8.0"
	browsers, _ := cfg["browsers"].(map[string]any)
	if browsers == nil {
		browsers = map[string]any{}
	}
	browsers["default"] = "cloak"
	cfg["browsers"] = browsers

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644) // #nosec G306 -- read-only config fixture.
}

// writeGhostChromeConfig reads a chrome-flavoured pinchtab config and writes a
// ghost-chrome-flavoured copy: it sets browsers.default to "ghost-chrome" and
// configVersion to prevent the startup wizard from overwriting it. No binary or
// cloak block changes — ghost-chrome uses Chrome under the hood.
func writeGhostChromeConfig(src, dst string) error {
	raw, err := os.ReadFile(src) // #nosec G304 -- src is constrained to tests/e2e/config/*.json.
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	cfg["configVersion"] = "0.8.0"
	browsers, _ := cfg["browsers"].(map[string]any)
	if browsers == nil {
		browsers = map[string]any{}
	}
	browsers["default"] = "ghost-chrome"
	cfg["browsers"] = browsers

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644) // #nosec G306 -- read-only config fixture.
}

// writeConfigOnlyComposeOverride writes a compose override that mounts
// rewritten config files into each pinchtab service without swapping the image.
// Used by ghost-chrome where the same Chrome image is used but the config
// selects a different browsers.default.
func writeConfigOnlyComposeOverride(path string, configs map[string]string, fixturesDir string) error {
	type extensionMount struct {
		sourceDir string
		target    string
	}
	type serviceOverrideConfig struct {
		configName      string
		extensionMounts []extensionMount
	}

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
		"pinchtab-ghostchrome": {
			configName: "pinchtab-ghostchrome.json",
		},
		"pinchtab-bridge": {
			configName: "pinchtab-bridge.json",
		},
	}

	var b strings.Builder
	b.WriteString("# Generated by tests/tools/runner — provider=ghost-chrome override.\n")
	b.WriteString("# Mounts ghost-chrome-flavoured runtime config in place of the chrome one.\n")
	b.WriteString("services:\n")

	for _, svc := range pinchtabServices {
		cfg, ok := serviceConfig[svc]
		if !ok {
			continue
		}
		baseName := cfg.configName
		configPath, ok := configs[baseName]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", svc)
		fmt.Fprintf(&b, "    image: e2e-pinchtab:latest\n")
		fmt.Fprintf(&b, "    pull_policy: never\n")
		b.WriteString("    environment:\n")
		b.WriteString("      PINCHTAB_RATE_LIMIT_MAX: \"3000\"\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/fixture-config-ghost/%s:ro\n", configPath, baseName)
		for _, mount := range cfg.extensionMounts {
			source := filepath.Join(fixturesDir, mount.sourceDir)
			fmt.Fprintf(&b, "      - %s:%s:ro\n", source, mount.target)
		}
		fmt.Fprintf(&b, "    command: [\"/bin/sh\", \"-lc\", \"mkdir -p /data/e2e-config && cp /fixture-config-ghost/%s /data/e2e-config/%s && exec /usr/local/bin/docker-entrypoint.sh pinchtab server\"]\n",
			baseName, baseName)
	}

	return os.WriteFile(path, []byte(b.String()), 0o644) // #nosec G306 -- consumed read-only by docker compose.
}

// writeCloakComposeOverride writes a compose file that targets each pinchtab
// service: swaps the image to the cloak provider image, drops the build
// directive (via image-only + pull_policy:never), remaps the config bind mount
// onto the cloak-flavoured copy we just generated, and preserves extension
// fixture mounts needed by extension-parity E2E tests.
//
// The override is intentionally written as YAML by hand (no third-party deps)
// since the structure is small and stable.
func writeCloakComposeOverride(path string, cloakConfigs map[string]string, fixturesDir, image string) error {
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
		"pinchtab-ghostchrome": {
			configName: "pinchtab-ghostchrome.json",
		},
		"pinchtab-bridge": {
			configName: "pinchtab-bridge.json",
		},
	}

	var b strings.Builder
	b.WriteString("# Generated by tests/tools/runner — provider=cloak override.\n")
	b.WriteString("# Swaps every pinchtab* service onto the cloak provider image and\n")
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
		fmt.Fprintf(&b, "    image: %s\n", image)
		fmt.Fprintf(&b, "    pull_policy: never\n")
		// Mount the cloak config at a distinct path to avoid volume merge
		// ambiguity with the base compose's /fixture-config mount. Override
		// the command to copy from this path instead.
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/fixture-config-cloak/%s:ro\n", cloakPath, baseName)
		for _, mount := range cfg.extensionMounts {
			source := filepath.Join(fixturesDir, mount.sourceDir)
			fmt.Fprintf(&b, "      - %s:%s:ro\n", source, mount.target)
		}
		fmt.Fprintf(&b, "    command: [\"/bin/sh\", \"-lc\", \"mkdir -p /data/e2e-config && cp /fixture-config-cloak/%s /data/e2e-config/%s && exec /usr/local/bin/docker-entrypoint.sh pinchtab server\"]\n",
			baseName, baseName)
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
	case "ghost-chrome":
		if provider != "ghost-chrome" {
			return fmt.Errorf("ghost-chrome stealth assertion failed (want provider=ghost-chrome); got: %s", formatStealthStatus(status))
		}
		_, _ = fmt.Fprintln(r.stdout, "  /stealth/status: provider=ghost-chrome")
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
