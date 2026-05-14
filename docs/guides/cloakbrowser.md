# CloakBrowser

Use this guide to run PinchTab with a CloakBrowser Chromium binary. PinchTab
stays the API and automation control plane. CloakBrowser supplies the browser
executable and native fingerprint patches.

```text
agent -> PinchTab API -> PinchTab bridge -> CloakBrowser Chromium
```

## What PinchTab Supports

PinchTab can launch a user-installed CloakBrowser binary with:

- `browser.provider=cloak`
- `browser.binary=/absolute/path/to/cloakbrowser/chrome`
- structured fingerprint settings under `browser.cloak`
- PinchTab profile directories, tab lifecycle, screenshots, snapshots, actions,
  downloads, uploads, clipboard, and evaluate endpoints
- `/stealth/status` reporting for the active provider, native Cloak mode, and
  PinchTab overlay status

When CloakBrowser native stealth is enabled, PinchTab disables overlapping
PinchTab JavaScript stealth overlays and automation-hiding launch flags by
default. PinchTab still controls browser lifecycle, user data directories,
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
pinchtab config set browser.provider cloak
pinchtab config set browser.binary /absolute/path/to/cloakbrowser/chrome
pinchtab config set browser.cloak.fingerprintSeed 42069
pinchtab config set browser.cloak.platform windows
pinchtab config set browser.cloak.timezone Europe/London
pinchtab config set browser.cloak.locale en-GB
```

Example config fragment:

```json
{
  "browser": {
    "provider": "cloak",
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
  "browser": {
    "provider": "cloak",
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
  "browser": {
    "provider": "cloak",
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
./dev smoke cloakbrowser
```

That command builds the same dedicated local image, starts PinchTab with
`browser.provider=cloak`, verifies `/stealth/status`, and runs the
Cloak-compatible API scenario set against the container.

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
