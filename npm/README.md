# Pinchtab npm

Browser control API for AI agents — Node.js SDK + CLI wrapper.

## Installation

```bash
npm install pinchtab
```

or globally:

```bash
npm install -g pinchtab
```

On install, the postinstall script automatically:
1. Detects your OS and CPU architecture (darwin/linux/windows, amd64/arm64)
2. Downloads the precompiled Pinchtab binary from GitHub Releases
   - Example: `pinchtab-darwin-amd64`, `pinchtab-windows-arm64.exe` (Windows)
3. Verifies integrity (SHA256 checksum from `checksums.txt`)
4. Stores it inside the installed package at `.managed-bin/<version>/` (version-specific to avoid conflicts)
5. Makes it executable

The binary lives inside the package directory rather than under `$HOME`, so a
global install resolves the same binary no matter which user runs the CLI —
including `sudo npm install -g pinchtab`, where postinstall runs as root but the
CLI later runs as you. (Binaries placed by older versions under `~/.pinchtab/bin`
are still honored.)

**Requirements:**
- Internet connection on first install (to download binary from GitHub Releases)
- Node.js 16+
- macOS, Linux, or Windows

### Local Development With A Local Binary

If you are testing the npm package from a local checkout, build the canonical
repo-local binary first:

```bash
bash scripts/npm-dev-binary.sh
cd npm
npm install
node bin/pinchtab --version
```

In a source checkout, the npm package now expects the local binary at
`../pinchtab-dev` and will fail clearly if it is missing. It does not download a
release binary in that mode.

### Restricted Networks and Mirrors

`npm install --https-proxy https://proxy.company.com:8080 pinchtab` (or the
`HTTPS_PROXY` environment variable) routes npm's own registry fetches through a
proxy. The postinstall binary download from GitHub Releases goes direct, however,
and does **not** read `HTTP(S)_PROXY`.

For proxied, mirrored, or air-gapped installs, point the binary download at a host
you control with `PINCHTAB_DOWNLOAD_BASE_URL`. It must mirror the GitHub Releases
layout — `<base>/v<version>/<asset>` — serving each platform binary alongside
`checksums.txt`:

```bash
export PINCHTAB_DOWNLOAD_BASE_URL=https://mirror.example.com/pinchtab
npm install -g pinchtab
```

## Quick Start

### Start the server

```bash
pinchtab serve --port 9867
```

### Use the SDK

```typescript
import Pinchtab from 'pinchtab';

const pinch = new Pinchtab({ port: 9867 });

// Start the server
await pinch.start();

// Take a snapshot
const snapshot = await pinch.snapshot({ refs: 'role' });
console.log(snapshot.html);

// Click on an element
await pinch.click({ ref: 'e42' });

// Lock a tab
await pinch.lock({ tabId: 'tab1', timeoutMs: 5000 });

// Stop the server
await pinch.stop();
```

## API

### `new Pinchtab(options)`

Create a Pinchtab client.

**Options:**
- `baseUrl` (string): API base URL. Default: `http://localhost:9867`
- `timeout` (number): Request timeout in ms. Default: `30000`
- `port` (number): Port to run on. Default: `9867`

### `start(binaryPath?)`

Start the Pinchtab server process.

### `stop()`

Stop the Pinchtab server process.

### `snapshot(params?)`

Take a snapshot of the current tab.

**Params:**
- `refs` ('role' | 'aria'): Reference system
- `selector` (string): CSS selector filter
- `maxTokens` (number): Token limit
- `format` ('full' | 'compact'): Response format

### `click(params)`

Click on an element.

**Params:**
- `ref` (string): Element reference
- `targetId` (string): Optional target tab ID

### `lock(params)` / `unlock(params)`

Lock/unlock a tab.

### `createTab(params)`

Create a new tab.

**Params:**
- `url` (string): Tab URL
- `stealth` ('light' | 'full'): Stealth level

## CLI

```bash
pinchtab serve [--port PORT]
pinchtab --version
pinchtab --help
```

### Agent skill

The package bundles the `pinchtab` agent skill, stamped with the release version and a content hash, and syncs it into detected agent homes on install and upgrade, printing what it wrote. `pinchtab skill status` reports each copy against the bundled one (exit 1 if any is stale) and `pinchtab skill update` refreshes them; a copy you edited is kept unless you pass `--force`.

### Shell Completion

After installing the CLI globally, you can generate shell completions:

```bash
# Generate and install zsh completions
pinchtab completion zsh > "${fpath[1]}/_pinchtab"

# Generate bash completions
pinchtab completion bash > /etc/bash_completion.d/pinchtab

# Generate fish completions
pinchtab completion fish > ~/.config/fish/completions/pinchtab.fish
```

### Using a Custom Binary

For development or custom integrations, pass the path explicitly in code:

```typescript
const pinch = new Pinchtab();
const binaryPath = '/custom/path/to/pinchtab';
await pinch.start(binaryPath);
```

## Troubleshooting

**Binary not found or "file not found" error:**

Check if the release has binaries:
```bash
# Should show pinchtab-darwin-arm64, pinchtab-linux-x64, etc.
curl -s https://api.github.com/repos/pinchtab/pinchtab/releases/latest | jq '.assets[].name'
```

If no binaries (only Docker images), rebuild with a newer release:
```bash
npm rebuild pinchtab
```

**Restricted network or mirror:**

The binary download does not use `HTTP(S)_PROXY`. Point it at a reachable mirror
(GitHub Releases layout) and rebuild:
```bash
export PINCHTAB_DOWNLOAD_BASE_URL=https://mirror.example.com/pinchtab
npm rebuild pinchtab
```

## Future: OptionalDependencies Pattern (v1.0)

In a future major version, we plan to migrate to the modern `optionalDependencies` pattern used by esbuild, Biome, Turbo, etc. This will split platform-specific binaries into separate npm packages (@pinchtab/cli-darwin-arm64, etc.) for zero postinstall network overhead and perfect offline support.

## License

MIT
