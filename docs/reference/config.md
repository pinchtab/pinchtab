# Config

`pinchtab config` is the CLI entry point for creating, inspecting, validating, and editing PinchTab's config file.

For security posture, token usage, sensitive endpoint policy, and IDPI guidance, see [Security](../guides/security.md).

## Commands

### `pinchtab config`

Opens the interactive config overview/editor.

It currently exposes these high-signal settings directly:

- `multiInstance.strategy`
- `multiInstance.allocationPolicy`
- `instanceDefaults.stealthLevel`
- `instanceDefaults.tabEvictionPolicy`

It also shows:

- the active config file path
- the dashboard URL when the server is running
- the masked server token
- a `Copy token` action

```bash
pinchtab config
```

### `pinchtab config init`

Creates a default config file at the current config path.

```bash
pinchtab config init
```

`config init` respects `PINCHTAB_CONFIG`. If that environment variable is set, the file is created there.

### `pinchtab config show`

Shows the effective runtime configuration.

```bash
pinchtab config show
```

### `pinchtab config path`

Prints the config file path PinchTab will read.

```bash
pinchtab config path
```

### `pinchtab config validate`

Validates the current config file.

```bash
pinchtab config validate
```

### `pinchtab config get`

Reads a single dotted-path value from the file config.

```bash
pinchtab config get server.port
pinchtab config get instanceDefaults.mode
pinchtab config get security.attach.allowHosts
```

### `pinchtab config set`

Sets a single dotted-path value in the file config.

```bash
pinchtab config set server.port 8080
pinchtab config set instanceDefaults.mode headed
pinchtab config set multiInstance.strategy explicit
```

### `pinchtab config patch`

Merges a JSON object into the config file.

```bash
pinchtab config patch '{"server":{"port":"8080"}}'
pinchtab config patch '{"instanceDefaults":{"mode":"headed","maxTabs":50}}'
pinchtab config patch '{"observability":{"activity":{"retentionDays":14}}}'
```

## Load Order

PinchTab applies configuration in this order:

1. built-in defaults
2. the config file selected by `PINCHTAB_CONFIG` or the default path
3. `PINCHTAB_TOKEN`, if set, overriding `server.token` at runtime

Supported environment variables:

- `PINCHTAB_CONFIG`: choose the config file path
- `PINCHTAB_TOKEN`: override the API token at runtime

For remote CLI targeting, use the root `--server` flag instead of config.

## Config File Location

Default location by OS:

- macOS: `~/Library/Application Support/pinchtab/config.json`
- Linux: `~/.config/pinchtab/config.json` or `$XDG_CONFIG_HOME/pinchtab/config.json`
- Windows: `%APPDATA%\pinchtab\config.json`

Legacy fallback:

- if `~/.pinchtab/config.json` exists and the newer location does not, PinchTab still uses the legacy location

Override the config path with:

```bash
export PINCHTAB_CONFIG=/path/to/config.json
```

## Config Shape

Current nested file-config shape:

```json
{
  "configVersion": "0.8.0",
  "server": {
    "port": "9867",
    "bind": "127.0.0.1",
    "token": "your-secret-token",
    "stateDir": "/path/to/state",
    "engine": "chrome",
    "networkBufferSize": 100,
    "trustProxyHeaders": false
  },
  "browser": {
    "version": "144.0.7559.133",
    "binary": "/path/to/chrome",
    "extraFlags": "--disable-gpu",
    "extensionPaths": []
  },
  "instanceDefaults": {
    "mode": "headless",
    "noRestore": false,
    "timezone": "Europe/Rome",
    "blockImages": false,
    "blockMedia": false,
    "blockAds": false,
    "maxTabs": 20,
    "maxParallelTabs": 0,
    "userAgent": "",
    "noAnimations": false,
    "stealthLevel": "light",
    "tabEvictionPolicy": "close_lru",
    "dialogAutoAccept": false
  },
  "security": {
    "allowEvaluate": false,
    "allowMacro": false,
    "allowScreencast": false,
    "allowDownload": false,
    "downloadAllowedDomains": [],
    "downloadMaxBytes": 20971520,
    "allowUpload": false,
    "allowClipboard": false,
    "uploadMaxRequestBytes": 10485760,
    "uploadMaxFiles": 8,
    "uploadMaxFileBytes": 5242880,
    "uploadMaxTotalBytes": 10485760,
    "maxRedirects": -1,
    "attach": {
      "enabled": false,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "wss"]
    },
    "idpi": {
      "enabled": true,
      "allowedDomains": ["127.0.0.1", "localhost", "::1"],
      "strictMode": true,
      "scanContent": true,
      "wrapContent": true,
      "customPatterns": [],
      "scanTimeoutSec": 5,
      "shieldThreshold": 30
    }
  },
  "profiles": {
    "baseDir": "/path/to/profiles",
    "defaultProfile": "default"
  },
  "multiInstance": {
    "strategy": "always-on",
    "allocationPolicy": "fcfs",
    "instancePortStart": 9868,
    "instancePortEnd": 9968,
    "restart": {
      "maxRestarts": 20,
      "initBackoffSec": 2,
      "maxBackoffSec": 60,
      "stableAfterSec": 300
    }
  },
  "timeouts": {
    "actionSec": 30,
    "navigateSec": 60,
    "shutdownSec": 10,
    "waitNavMs": 1000
  },
  "scheduler": {
    "enabled": false,
    "strategy": "fair-fifo",
    "maxQueueSize": 1000,
    "maxPerAgent": 100,
    "maxInflight": 20,
    "maxPerAgentInflight": 10,
    "resultTTLSec": 300,
    "workerCount": 4
  },
  "observability": {
    "activity": {
      "enabled": true,
      "sessionIdleSec": 1800,
      "retentionDays": 1
    }
  }
}
```

`browser.extraFlags` is validated and sanitized. It is only for user-safe Chrome flags that do not weaken browser security and do not override PinchTab-owned launch behavior.

Rejected examples include:

- `--no-sandbox`
- `--disable-web-security`
- `--ignore-certificate-errors`
- `--user-agent=...`
- `--enable-automation=...`
- `--disable-blink-features=...`

Use the dedicated config fields instead:

- `instanceDefaults.userAgent` for UA overrides
- `instanceDefaults.mode` for headed/headless
- `instanceDefaults.timezone` for timezone
- `browser.extensionPaths` for extension loading
- `browser.remoteDebuggingPort` for the remote debugging port

For Linux container compatibility, use the runtime-managed path instead of `browser.extraFlags`. PinchTab enables `--no-sandbox` automatically when needed, and you can force that compatibility mode with `PINCHTAB_CHROME_NO_SANDBOX=1`.

## Sections

| Section | Purpose |
| --- | --- |
| `server` | HTTP server settings, engine selection, proxy trust, and network buffer defaults |
| `browser` | Chrome executable, version pin, extra flags, and extension paths |
| `instanceDefaults` | Default behavior for managed instances |
| `security` | Sensitive feature gates, transfer limits, attach policy, and IDPI |
| `profiles` | Profile storage defaults |
| `multiInstance` | Orchestrator strategy, allocation, port range, and restart policy |
| `timeouts` | Action, navigation, shutdown, and navigation wait delays |
| `scheduler` | Optional task queue |
| `observability` | Activity logging and retention |

## `config get` And `config set` Support

`pinchtab config get` and `pinchtab config set` only support these top-level sections:

- `server`
- `browser`
- `instanceDefaults`
- `security`
- `profiles`
- `multiInstance`
- `timeouts`

They do not expose every field in those sections, and they do not support `scheduler.*` or `observability.*`.

Use `pinchtab config patch` or edit `config.json` directly for fields such as:

- `server.engine`
- `server.networkBufferSize`
- `browser.extensionPaths`
- `instanceDefaults.dialogAutoAccept`
- `security.allowClipboard`
- `security.idpi.scanTimeoutSec`
- `security.idpi.shieldThreshold`
- `scheduler.*`
- `observability.*`

## Common Examples

### Headed Mode

```json
{
  "instanceDefaults": {
    "mode": "headed"
  }
}
```

### Network Bind With Token

```bash
pinchtab config set server.bind 0.0.0.0
pinchtab config set server.token secret
pinchtab server
```

Changing `server.bind` away from loopback is a documented, non-default, security-reducing deployment change. Use it only when remote reachability is intentional, keep a token set, and review the outer network boundary explicitly.

### Custom Instance Port Range

```json
{
  "multiInstance": {
    "instancePortStart": 8100,
    "instancePortEnd": 8200
  }
}
```

### Attach Policy

```json
{
  "security": {
    "attach": {
      "enabled": true,
      "allowHosts": ["127.0.0.1", "localhost", "chrome.internal"],
      "allowSchemes": ["ws", "wss", "http", "https"]
    }
  }
}
```

`security.attach.allowHosts` is an allowlist. If you set it to `["*"]`, PinchTab accepts any reachable attach host with an allowed scheme. That is a documented, non-default, security-reducing override: it removes host allowlisting entirely and should only be used on isolated, operator-controlled networks.

### Activity Retention

```json
{
  "observability": {
    "activity": {
      "retentionDays": 14,
      "sessionIdleSec": 1800
    }
  }
}
```

`server.trustProxyHeaders` should stay `false` unless PinchTab is behind a trusted reverse proxy that overwrites `Forwarded` and `X-Forwarded-*` headers. Do not enable it on direct-exposure deployments or behind proxies that pass client-supplied forwarding headers through unchanged.

## Legacy Flat Format

Older flat config is still accepted for backward compatibility:

```json
{
  "port": "9867",
  "headless": true,
  "maxTabs": 20,
  "allowEvaluate": false,
  "timeoutSec": 30,
  "navigateSec": 60
}
```

Use `pinchtab config init` to create the current nested format.

## Validation

`pinchtab config validate` checks, among other things:

- valid `instanceDefaults.mode`
- valid `instanceDefaults.stealthLevel`
- valid `instanceDefaults.tabEvictionPolicy`
- `instanceDefaults.maxTabs >= 1`
- `instanceDefaults.maxParallelTabs >= 0`
- valid `multiInstance.strategy`
- valid `multiInstance.allocationPolicy`
- valid `multiInstance.restart.*` values
- valid `security.attach.allowSchemes`
- `multiInstance.instancePortStart <= multiInstance.instancePortEnd`
- `multiInstance.restart.initBackoffSec <= multiInstance.restart.maxBackoffSec`
- non-negative timeout values
- non-negative `server.networkBufferSize`
- non-negative `security.idpi.scanTimeoutSec`
- positive `observability.activity.sessionIdleSec` and `retentionDays`

Valid enum values:

| Field | Values |
| --- | --- |
| `instanceDefaults.mode` | `headless`, `headed` |
| `instanceDefaults.stealthLevel` | `light`, `medium`, `full` |
| `instanceDefaults.tabEvictionPolicy` | `reject`, `close_oldest`, `close_lru` |
| `multiInstance.strategy` | `simple`, `explicit`, `simple-autorestart`, `always-on`, `no-instance` |
| `multiInstance.allocationPolicy` | `fcfs`, `round_robin`, `random` |
| `security.attach.allowSchemes` | `ws`, `wss`, `http`, `https` |

## Notes

- `config show` reports effective runtime values, not just raw file contents.
- `config get`, `set`, and `patch` operate on the file config model, not transient runtime overrides.
- the dashboard config API treats `server.token` as write-only; use the CLI or file editing to manage it.
