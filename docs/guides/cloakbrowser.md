# CloakBrowser

Use this guide to run PinchTab with a CloakBrowser Chromium binary. PinchTab
stays the API and automation control plane. CloakBrowser supplies the browser
executable and native fingerprint patches.

```text
agent -> PinchTab API -> PinchTab bridge -> CloakBrowser Chromium
```

## What PinchTab Supports

PinchTab can launch a user-installed CloakBrowser binary with:

- `browsers.default=cloak`
- `browser.binary=/absolute/path/to/cloakbrowser/chrome`
- structured fingerprint settings under `browser.cloak`
- PinchTab profile directories, tab lifecycle, screenshots, snapshots, actions,
  downloads, uploads, clipboard, and evaluate endpoints
- `/stealth/status` reporting for the active browser, native Cloak mode, and
  PinchTab overlay status

When `browser.cloak.disableDefaultStealthArgs=true`, PinchTab disables its
overlapping JavaScript stealth overlays and automation-hiding launch flags.
PinchTab still controls browser lifecycle, user data directories,
headless/headed mode, remote debugging, tab limits, and action humanization.

PinchTab does not bundle CloakBrowser in release binaries, npm packages,
Homebrew formulae, or the default published Docker image.

## Licensing

CloakBrowser has two relevant pieces:

- wrapper packages and source code
- the compiled CloakBrowser Chromium binary

The wrapper source is MIT licensed. The compiled Chromium binary has a separate
license. Do not vendor, copy, or redistribute that binary in PinchTab release
artifacts or public Docker images unless the license explicitly permits that
use.

PinchTab's distribution policy for CloakBrowser is:

- PinchTab does **not** bundle any CloakBrowser binary in released artifacts —
  not in `./dev binaries`, not in the npm package, not in the Homebrew formula,
  not in the published `pinchtab/pinchtab` or `ghcr.io/pinchtab/pinchtab`
  Docker images.
- Users supply their own CloakBrowser binary on disk and point
  `browser.binary` (or `browser.targets.<name>.binary`) at it.
- The local Docker file
  `tests/tools/docker/cloakbrowser-smoke.Dockerfile` exists for local
  smoke/test purposes only. It is not pushed to any registry and is not a
  release artifact.
- There is no public CloakBrowser-bundled PinchTab release image and no
  hosted service. None are planned until redistribution, OEM, or SaaS terms
  are clear with the upstream CloakBrowser maintainers.

The safe default is: install or download CloakBrowser yourself, then point
PinchTab at that local binary path.

Review:

- [CloakBrowser repository](https://github.com/CloakHQ/CloakBrowser)
- [CloakBrowser binary license](https://github.com/CloakHQ/CloakBrowser/blob/main/BINARY-LICENSE.md)

## Install CloakBrowser

Install CloakBrowser through one of its official package paths, then use the
reported Chromium binary path as PinchTab's `browser.binary`.

Python:

```bash
pip install cloakbrowser
python -m cloakbrowser install
python -m cloakbrowser info
```

JavaScript:

```bash
npm install cloakbrowser playwright-core
node -e "import('cloakbrowser').then(async m => { await m.ensureBinary(); console.log(m.binaryInfo()); })"
```

Use an absolute path to the `chrome` executable reported by the installer.

## Configure PinchTab

Create or update your PinchTab config:

```bash
pinchtab config init
pinchtab config set browsers.default cloak      # CLI: --browser=cloak
pinchtab config set browser.binary /absolute/path/to/cloakbrowser/chrome
pinchtab config set browser.cloak.fingerprintSeed 42069
pinchtab config set browser.cloak.platform windows
pinchtab config set browser.cloak.timezone Europe/London
pinchtab config set browser.cloak.locale en-GB
```

Example config fragment:

```json
{
  "browsers": {
    "default": "cloak"
  },
  "browser": {
    "binary": "/absolute/path/to/cloakbrowser/chrome",
    "cloak": {
      "fingerprintSeed": "42069",
      "platform": "windows",
      "timezone": "Europe/London",
      "locale": "en-GB",
      "webrtcIP": "auto",
      "disableDefaultStealthArgs": true
    }
  },
  "instanceDefaults": {
    "mode": "headless",
    "humanize": true
  }
}
```

Use a fixed `browser.cloak.fingerprintSeed` when revisiting the same site from
the same profile. Keep `browser.cloak.disableDefaultStealthArgs` set to `true`
unless you intentionally want to layer PinchTab's legacy stealth behavior over
CloakBrowser's native patches.

## Cloak Options

PinchTab maps these `browser.cloak` fields to CloakBrowser launch flags:

| Config field | Launch flag |
| --- | --- |
| `fingerprintSeed` | `--fingerprint=<seed>` |
| `platform` | `--fingerprint-platform=<windows|macos|linux>` |
| `timezone` | `--fingerprint-timezone=<iana timezone>` |
| `locale` | `--fingerprint-locale=<locale>` |
| `webrtcIP` | `--fingerprint-webrtc-ip=<ip|auto>` |
| `fontsDir` | `--fingerprint-fonts-dir=<path>` |
| `storageQuotaMB` | `--fingerprint-storage-quota=<mb>` |

Use `browser.extraFlags` only for advanced CloakBrowser flags that do not have a
structured PinchTab field.

```json
{
  "browsers": {
    "default": "cloak"
  },
  "browser": {
    "binary": "/absolute/path/to/cloakbrowser/chrome",
    "cloak": {
      "fingerprintSeed": "42069",
      "timezone": "Europe/London",
      "locale": "en-GB"
    },
    "extraFlags": "--fingerprint-brand=Chrome"
  }
}
```

Be conservative with raw flags. PinchTab owns lifecycle flags such as remote
debugging port, user data dir, headless mode, window size, extension paths, and
user agent.

## Start PinchTab

Start the server:

```bash
pinchtab server
```

In another shell, check that the server is healthy and can drive a page:

```bash
pinchtab health
pinchtab nav https://example.com --snap
```

If your security policy blocks public navigation, add the exact host to
`security.allowedDomains` before using a public URL.

## Verify Configuration With `pinchtab doctor`

Before starting the server, run the read-only diagnostic command. It checks the
configured binary, executes it briefly, and validates fingerprint flags
without mutating any state:

```bash
pinchtab doctor                       # human-readable report
pinchtab doctor --json                # machine-readable JSON
pinchtab doctor browser cloak-eu      # scope to one browser.targets entry
pinchtab doctor --check binary_exists # run a single check by name
```

Exit codes: `0` all checks passed or skipped, `1` at least one check failed,
`2` config or usage error. See the [Troubleshooting](#troubleshooting) section
below for how to react to specific failures.

## Verify CloakBrowser Is Active

Check `/stealth/status` after the managed browser instance starts:

```bash
TOKEN="$(pinchtab config get server.token)"
curl -sS -H "Authorization: Bearer ${TOKEN}" \
  http://127.0.0.1:9867/stealth/status
```

Look for these fields:

```json
{
  "provider": "cloak",
  "native": true,
  "pinchtabOverlaysDisabled": true,
  "fingerprintSeed": "42069"
}
```

If launch fails, inspect the instance:

```bash
pinchtab instances
pinchtab instance logs <instance-id>
```

Common causes:

- the configured binary path does not exist
- the binary is not executable
- the CloakBrowser binary was downloaded for a different operating system
- the profile directory is locked by another browser process
- a raw extra flag conflicts with PinchTab-owned lifecycle behavior

## Docker

The published `pinchtab/pinchtab` and `ghcr.io/pinchtab/pinchtab` images include
Alpine Chromium. They do not include CloakBrowser.

For local Docker use with CloakBrowser, build the dedicated local image:

```bash
docker build \
  -f tests/tools/docker/cloakbrowser-smoke.Dockerfile \
  -t pinchtab-cloakbrowser:local \
  .
```

Create a config that points PinchTab at the CloakBrowser binary inside that
image:

```json
{
  "server": {
    "bind": "0.0.0.0",
    "port": "9867",
    "token": "replace-me",
    "stateDir": "/data"
  },
  "browsers": {
    "default": "cloak"
  },
  "browser": {
    "binary": "/opt/cloakbrowser/chrome",
    "cloak": {
      "fingerprintSeed": "42069",
      "platform": "linux",
      "timezone": "UTC",
      "locale": "en-US",
      "disableDefaultStealthArgs": true
    }
  },
  "instanceDefaults": {
    "mode": "headless",
    "humanize": true
  },
  "profiles": {
    "baseDir": "/data/profiles",
    "defaultProfile": "default"
  }
}
```

Run the container with the config mounted read-only:

```bash
docker run -d \
  --name pinchtab-cloak \
  -p 127.0.0.1:9867:9867 \
  --shm-size=2g \
  -v pinchtab-cloak-data:/data \
  -v "$PWD/pinchtab-cloak.json":/config/pinchtab.json:ro \
  -e PINCHTAB_CONFIG=/config/pinchtab.json \
  pinchtab-cloakbrowser:local
```

Do not publish or push a CloakBrowser-bundled image unless the CloakBrowser
binary license explicitly permits redistribution for your use case.

## Maintainer Validation

Project maintainers can validate the Docker integration with:

```bash
./dev e2e --browser=cloak
```

That command swaps the runtime browser to `browsers.default=cloak`
(`/opt/cloakbrowser/chrome`, `fingerprintSeed=42069`,
`disableDefaultStealthArgs=true`), reuses the prebuilt
`pinchtab-cloakbrowser:test` image (build it once via
`tests/tools/docker/cloakbrowser-smoke.Dockerfile` — the runner does not
auto-build it), asserts `/stealth/status` reports
`provider=cloak, native=true, pinchtabOverlaysDisabled=true,
fingerprintSeed=42069`, and runs the full E2E suite against the cloak
container.

Use `./dev smoke --browser=cloak` for the full local CloakBrowser smoke set.
Use `./dev smoke cloakbrowser` when you only need the specialized parity legs,
including `--multi-target`, `--profile-persistence`, and
`--profile-lock-recovery`.

## Attaching a running CloakBrowser

If CloakBrowser is already running with remote debugging enabled (for example,
under CloakBrowser Manager), you can attach it to PinchTab through the normal
CDP attach path. PinchTab spawns a child `pinchtab bridge --cdp-attach ...` wrapper
around the external endpoint and exposes the standard API on a local port.

```bash
curl -X POST http://localhost:9867/instances/attach \
  -H "Content-Type: application/json" \
  -d '{
    "name": "cloak-manager-profile",
    "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/abc123",
    "provider": "cloak",
    "browserTarget": "cloak-1"
  }'
```

For a remote CloakBrowser attach:

- `browserTarget` should name a CloakBrowser target when `browser.targets` is configured; if omitted, the configured default target is used
- `provider` in the JSON body must be `cloak` when no browser target is configured, or must match the selected target when both fields are present
- `/stealth/status` reports `provider=cloak`, `launchMode=remote-cdp`,
  `native=true`, and `pinchtabOverlaysDisabled=true` — PinchTab does not inject
  its JS fingerprint overlays, on the assumption the external browser owns
  native fingerprint behavior
- stopping the attached PinchTab instance shuts down the wrapper bridge only;
  the external CloakBrowser process is left running

The opt-in smoke `./dev smoke cdp-attach` exercises this path against a
locally-built CloakBrowser image.

## Troubleshooting

Most CloakBrowser launch failures fall into one of the buckets below. Run
`pinchtab doctor` first — it isolates the failure to a specific named check
and removes the need to guess.

### Missing binary

Symptom: `pinchtab doctor` reports `binary_exists: FAIL`, or the server logs
`stat /path/to/cloakbrowser/chrome: no such file or directory` when starting an
instance.

Fix: set `browser.binary` (or `browser.targets.<name>.binary` for named
targets) to an absolute path that exists and is executable:

```bash
pinchtab config set browser.binary /absolute/path/to/cloakbrowser/chrome
# or, for a named target:
pinchtab config set browser.targets.cloak-eu.binary /absolute/path/to/cloakbrowser/chrome
```

PinchTab does not search `$PATH` for CloakBrowser. Use the absolute path
reported by `python -m cloakbrowser info` or `cloakbrowser.binaryInfo()`.

### Unsupported platform build

Symptom: `pinchtab doctor` reports `binary_starts: FAIL` with a non-zero
exit code, a segfault, or an `exec format error`.

Fix: the CloakBrowser binary is platform-specific (CPU architecture and
operating system). A Linux x86_64 binary will not run on macOS arm64 and vice
versa. Reinstall CloakBrowser for the platform you are actually running on,
and refer to the upstream CloakBrowser release notes for the matrix of
supported builds.

The PinchTab smoke image
(`tests/tools/docker/cloakbrowser-smoke.Dockerfile`) is built for the host
architecture; if you build it on arm64 and try to run the resulting image on
x86_64 (or vice versa) the inner CloakBrowser binary will fail the same way.

### Profile lock conflicts

Symptom: launch fails with `profile in use`, or PinchTab logs a warning that
the profile lock is held.

PinchTab's stale-`SingletonLock` recovery (P3b) automatically removes the
lock when the owning PID no longer exists:

- if the lock points to a live non-PinchTab process, PinchTab refuses to
  steal the profile and logs `chrome profile lock appears active and owned by another pinchtab; leaving singleton files in place`
- if the lock points to a dead process, PinchTab logs `chrome profile lock appears active but pinchtab owner is dead` and cleans up the stale files
- if the lock points to a live process owned by another PinchTab, PinchTab
  refuses to steal it

If recovery does not happen automatically, check the container or host
logs for the `chrome profile lock appears active but pinchtab owner is dead`
line:

```bash
docker logs pinchtab-cloak | grep "profile lock"
```

Manual recovery (only when you are sure no real CloakBrowser process is
running against that profile):

```bash
rm -f /path/to/profile/{SingletonLock,SingletonCookie,SingletonSocket}
```

### Bad fingerprint / proxy / timezone combinations

Symptom: target site flags the session as automated even though
`/stealth/status` reports `provider=cloak, native=true`.

CloakBrowser's fingerprint flags and proxy geo must be **coherent**. A US
residential proxy paired with `cloak.timezone=Europe/London` and
`cloak.locale=en-GB` looks suspicious to any detector that cross-checks
client locale against egress IP. Align them:

- match `browser.proxy.geo.timezone` to the proxy's actual location
- match `browser.proxy.geo.locale` and `browser.cloak.locale`
- match `browser.cloak.platform` to a believable device for the locale
- if you set `browser.cloak.webrtcIP`, ensure it is not leaking the host's
  real LAN range

If you cannot set the proxy's geo by hand, leave the `browser.proxy.geo`
block out and let PinchTab use the proxy without geo overrides — better to
have a missing signal than a contradictory one.

Proxy geo alignment is CloakBrowser-only. Plain Chrome targets keep their normal
Chrome launch behavior and do not receive proxy-derived `--lang`, `TZ`, or
WebRTC flags.

### Detection sites still flagging automation

Symptom: bot-detection demo pages (CreepJS, BrowserScan, Sannysoft, etc.)
report a non-zero detection score even with CloakBrowser native stealth
enabled.

This is expected to vary. Detection score is a moving target: detector
heuristics change outside PinchTab releases, CloakBrowser ships new patches,
Chromium versions drift, and per-site signals evolve independently of any
single component.

To investigate, re-run the live-detection smoke against the configured
browser:

```bash
./dev smoke live-detection                       # Chrome leg (baseline)
./dev smoke live-detection --browser=chrome
./dev smoke live-detection --browser=cloak
```

The smoke captures per-site screenshots and an extracted summary into
`./tests/e2e/results/live-detection/<browser>-<timestamp>/`. Compare the cloak run
against the chrome baseline: a non-zero cloak score that matches the
Chrome baseline indicates a generic Chromium signal, not a CloakBrowser
regression. Re-run the smoke after upstream CloakBrowser or detector
updates rather than treating one run as authoritative.

Results from the live-detection smoke are advisory and never gate CI.

## Related Guides

- [docker.md](docker.md) — the local CloakBrowser smoke image and persistent
  profile volume
- [attach-chrome.md](attach-chrome.md) — attach to an externally managed
  CloakBrowser via CDP
- [headed-mode.md](headed-mode.md) — manual headed setup (the bundled and
  smoke images are headless-only)
- [security.md](security.md) — the underlying security model, attach policy,
  IDPI, and token handling

## Security

CloakBrowser changes browser fingerprint behavior. It does not change PinchTab's
security model.

Keep these rules:

- leave PinchTab bound to localhost unless you intentionally operate a secured
  remote deployment
- keep `server.token` set when reachable outside localhost
- keep `security.allowedDomains` narrow
- do not expose CDP endpoints publicly
- do not use automation against systems you do not own or have authorization to
  test

If a target site blocks automation, treat that as a policy and reliability
signal, not just a technical obstacle.
