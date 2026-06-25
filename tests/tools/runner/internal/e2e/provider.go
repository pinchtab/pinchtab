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
const dryRunGhostChromeComposeOverride = "<generated-ghost-chrome-compose-override>"
const stockPinchtabImage = "e2e-pinchtab:latest"

type extensionMount struct {
	sourceDir string
	target    string
}

// pinchtabServiceTable enumerates every pinchtab variant present in the e2e
// compose files, with the attributes a provider override must preserve from
// docker-compose-multi.yml: the config it mounts, the subcommand it execs
// (bridge services must stay bridges), and its extension fixture mounts.
// Anything not listed here is left untouched by overrides (notably the
// runner-* services and fixtures).
var pinchtabServiceTable = []struct {
	name            string
	configName      string
	subcommand      string // "server" or "bridge" — must match docker-compose-multi.yml
	extensionMounts []extensionMount
	// keepStockProvider pins the service to the stock image and its own
	// config flavor in every provider lane: pinchtab-ghostchrome is the
	// dedicated ghost-chrome comparison server and must not become cloak.
	keepStockProvider bool
}{
	{
		name:       "pinchtab",
		configName: "pinchtab.json",
		subcommand: "server",
		extensionMounts: []extensionMount{
			{sourceDir: "test-extension", target: "/extensions/test-extension"},
			{sourceDir: "test-extension-api", target: "/extensions/test-extension-api"},
		},
	},
	{
		name:       "pinchtab-secure",
		configName: "pinchtab-secure.json",
		subcommand: "server",
	},
	{
		name:       "pinchtab-autoclose",
		configName: "pinchtab-autoclose.json",
		subcommand: "server",
		extensionMounts: []extensionMount{
			{sourceDir: "test-extension", target: "/extensions/test-extension"},
		},
	},
	{
		name:       "pinchtab-medium",
		configName: "pinchtab-medium-permissive.json",
		subcommand: "server",
		extensionMounts: []extensionMount{
			{sourceDir: "test-extension", target: "/extensions/test-extension"},
		},
	},
	{
		name:       "pinchtab-full",
		configName: "pinchtab-full-permissive.json",
		subcommand: "server",
		extensionMounts: []extensionMount{
			{sourceDir: "test-extension", target: "/extensions/test-extension"},
		},
	},
	{
		name:       "pinchtab-retain",
		configName: "pinchtab-network-retain-bodies.json",
		subcommand: "server",
		extensionMounts: []extensionMount{
			{sourceDir: "test-extension", target: "/extensions/test-extension"},
		},
	},
	{
		name:              "pinchtab-ghostchrome",
		configName:        "pinchtab-ghostchrome.json",
		subcommand:        "bridge",
		keepStockProvider: true,
	},
	{
		name:       "pinchtab-bridge",
		configName: "pinchtab-bridge.json",
		subcommand: "bridge",
	},
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

	configs, err := r.rewriteE2EConfigs(tmp, func(src, dst string) error {
		// The dedicated ghost-chrome comparison server keeps its ghost-chrome
		// flavor in every provider lane; only the primary services go cloak.
		if filepath.Base(src) == "pinchtab-ghostchrome.json" {
			return writeGhostChromeConfig(src, dst)
		}
		return writeCloakConfig(src, dst)
	})
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
			composeFiles: []string{dryRunGhostChromeComposeOverride},
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
	cmd := exec.Command("docker", "image", "inspect", image) // #nosec G204 -- fixed docker command; image name is runner-controlled, not user input
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

// rewriteProviderConfig reads a chrome-flavoured pinchtab config, applies the
// provider-specific mutate, forces configVersion (so the startup wizard won't
// rewrite the file) and browsers.default, then writes the result to dst.
func rewriteProviderConfig(src, dst, defaultBrowser string, mutate func(cfg map[string]any)) error {
	raw, err := os.ReadFile(src) // #nosec G304 -- src is constrained to tests/e2e/config/*.json.
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if mutate != nil {
		mutate(cfg)
	}

	cfg["configVersion"] = "0.8.0"
	browsers, _ := cfg["browsers"].(map[string]any)
	if browsers == nil {
		browsers = map[string]any{}
	}
	browsers["default"] = defaultBrowser
	cfg["browsers"] = browsers

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644) // #nosec G306 -- read-only config fixture.
}

// writeCloakConfig reads a chrome-flavoured pinchtab config and writes a
// cloak-flavoured copy: it sets browsers.default, browser.binary, and the
// browser.cloak block. The fingerprint seed mirrors scripts/lib/smoke-config.sh.
func writeCloakConfig(src, dst string) error {
	return rewriteProviderConfig(src, dst, "cloak", func(cfg map[string]any) {
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
	})
}

// writeGhostChromeConfig reads a chrome-flavoured pinchtab config and writes a
// ghost-chrome-flavoured copy: it sets browsers.default to "ghost-chrome" and
// configVersion to prevent the startup wizard from overwriting it. No binary or
// cloak block changes — ghost-chrome uses Chrome under the hood.
func writeGhostChromeConfig(src, dst string) error {
	return rewriteProviderConfig(src, dst, "ghost-chrome", nil)
}

// writeConfigOnlyComposeOverride writes a compose override that mounts
// rewritten config files into each pinchtab service without swapping the image.
// Used by ghost-chrome where the same Chrome image is used but the config
// selects a different browsers.default.
func writeConfigOnlyComposeOverride(path string, configs map[string]string, fixturesDir string) error {
	return writeProviderComposeOverride(path, configs, fixturesDir, "ghost-chrome", "")
}

// writeCloakComposeOverride writes a compose file that swaps each pinchtab
// service onto the cloak provider image (except keepStockProvider services)
// and remaps the config bind mount onto the cloak-flavoured copy.
func writeCloakComposeOverride(path string, cloakConfigs map[string]string, fixturesDir, image string) error {
	return writeProviderComposeOverride(path, cloakConfigs, fixturesDir, "cloak", image)
}

// writeProviderComposeOverride emits the per-provider compose override. It
// preserves each service's base-compose subcommand (bridge services stay
// bridges) and pins keepStockProvider services to the stock image. The
// override is intentionally written as YAML by hand (no third-party deps)
// since the structure is small and stable.
func writeProviderComposeOverride(path string, configs map[string]string, fixturesDir, provider, image string) error {
	// Mount configs at a provider-distinct path to avoid volume merge
	// ambiguity with the base compose's /fixture-config mount; the command
	// copies from this path instead.
	mountDir := "/fixture-config-ghost"
	if provider == "cloak" {
		mountDir = "/fixture-config-cloak"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by tests/tools/runner — provider=%s override.\n", provider)
	fmt.Fprintf(&b, "# Mounts %s-flavoured runtime config in place of the chrome one.\n", provider)
	b.WriteString("services:\n")

	for _, svc := range pinchtabServiceTable {
		configPath, ok := configs[svc.configName]
		if !ok {
			return fmt.Errorf("provider %q override: service %q has no generated config %q (renamed or removed fixture in tests/e2e/config?)", provider, svc.name, svc.configName)
		}
		svcImage := stockPinchtabImage
		if provider == "cloak" && !svc.keepStockProvider {
			svcImage = image
		}
		fmt.Fprintf(&b, "  %s:\n", svc.name)
		fmt.Fprintf(&b, "    image: %s\n", svcImage)
		fmt.Fprintf(&b, "    pull_policy: never\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:%s/%s:ro\n", configPath, mountDir, svc.configName)
		for _, mount := range svc.extensionMounts {
			source := filepath.Join(fixturesDir, mount.sourceDir)
			fmt.Fprintf(&b, "      - %s:%s:ro\n", source, mount.target)
		}
		fmt.Fprintf(&b, "    command: [\"/bin/sh\", \"-lc\", \"mkdir -p /data/e2e-config && cp %s/%s /data/e2e-config/%s && exec /usr/local/bin/docker-entrypoint.sh pinchtab %s\"]\n",
			mountDir, svc.configName, svc.configName, svc.subcommand)
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
