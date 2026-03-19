# Config

`pinchtab config` is the CLI entry point for creating, inspecting, validating, and editing PinchTab's config file.

Use this page as the command and schema reference. Broader deployment patterns can move to a separate guide later.

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
- a `Copy token` action for clipboard/manual copy

```bash
pinchtab config
```

### `pinchtab config init`

Creates a default config file at the standard user config location.

```bash
pinchtab config init
```

Current behavior note:

- `config init` writes to the default config path
- it does not currently switch to a custom `PINCHTAB_CONFIG` target path

### `pinchtab config show`

Shows the effective runtime configuration after applying:

```text
env vars -> config file -> built-in defaults
```

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

Reads a single dotted-path value from the config file.

```bash
pinchtab config get server.port
pinchtab config get instanceDefaults.mode
pinchtab config get security.attach.allowHosts
```

### `pinchtab config set`

Sets a single dotted-path value in the config file.

```bash
pinchtab config set server.port 8080
pinchtab config set instanceDefaults.mode headed
pinchtab config set multiInstance.strategy explicit
```

### `pinchtab config patch`

Applies a JSON patch object to the config file.

```bash
pinchtab config patch '{"server":{"port":"8080"}}'
pinchtab config patch '{"instanceDefaults":{"mode":"headed","maxTabs":50}}'
```

## Config Priority

PinchTab loads configuration in this order:

1. environment variables
2. config file
3. built-in defaults

The supported env vars are:

- `PINCHTAB_CONFIG` — path to config file
- `PINCHTAB_TOKEN` — auth token (overrides config file)

Everything else is configured in `config.json`. For CLI targeting a remote server, use the `--server` flag.

For the common local workflow, prefer the interactive `pinchtab config` screen for everyday changes and use `get`, `set`, or `patch` when you need an exact scripted edit.

## Config File Location

Default location by OS:

- macOS: `~/Library/Application Support/pinchtab/config.json`
- Linux: `~/.config/pinchtab/config.json` (respects `$XDG_CONFIG_HOME` if set)
- Windows: `%APPDATA%\pinchtab\config.json`

Legacy fallback:

- if `~/.pinchtab/config.json` exists and the newer location does not, PinchTab still uses the legacy location

Override the read path with:

```bash
export PINCHTAB_CONFIG=/path/to/config.json
```

## Config Shape

Current nested config shape:

```json
{
  "server": {
    "port": "9867",
    "bind": "127.0.0.1",
    "token": "your-secret-token",
    "stateDir": "/path/to/state"
  },
  "browser": {
    "version": "144.0.7559.133",
    "binary": "/path/to/chrome",
    "extraFlags": "",
    "extensionPaths": []
  },
  "instanceDefaults": {
    "mode": "headless",
    "maxTabs": 20,
    "maxParallelTabs": 0,
    "stealthLevel": "light",
    "tabEvictionPolicy": "close_lru",
    "blockAds": false,
    "blockImages": false,
    "blockMedia": false,
    "noRestore": false,
    "noAnimations": false
  },
  "security": {
    "allowEvaluate": false,
    "allowMacro": false,
    "allowScreencast": false,
    "allowDownload": false,
    "allowUpload": false,
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
      "customPatterns": []
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
  }
}
```

## Sections

| Section | Purpose |
| --- | --- |
| `server` | HTTP server settings |
| `browser` | Chrome executable and launch wiring |
| `instanceDefaults` | Default behavior for managed instances |
| `security` | Sensitive feature gates, attach policy, and IDPI |
| `profiles` | Profile storage defaults |
| `multiInstance` | Strategy, allocation, instance port range, and restart policy |
| `timeouts` | Runtime timeouts |
| `scheduler` | Optional task queue (see [Scheduler](./scheduler.md)) |

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
pinchtab
```

### Custom Instance Port Range

```json
{
  "server": {
    "port": "8080"
  },
  "multiInstance": {
    "instancePortStart": 8100,
    "instancePortEnd": 8200
  }
}
```

### Tab Eviction Policy

```json
{
  "instanceDefaults": {
    "maxTabs": 10,
    "tabEvictionPolicy": "close_lru"
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
      "allowSchemes": ["ws", "wss"]
    }
  }
}
```

### IDPI Policy

```json
{
  "security": {
    "idpi": {
      "enabled": true,
      "allowedDomains": ["example.com", "*.example.com"],
      "strictMode": true,
      "scanContent": true,
      "wrapContent": true,
      "customPatterns": []
    }
  }
}
```

This is policy only. The actual `cdpUrl` is provided in the attach request, not in global config.

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

Use `pinchtab config init` to create a nested config file.

## Validation

`pinchtab config validate` currently checks, among other things:

- valid server port values
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

Valid enum values:

| Field | Values |
| --- | --- |
| `instanceDefaults.mode` | `headless`, `headed` |
| `instanceDefaults.stealthLevel` | `light`, `medium`, `full` |
| `instanceDefaults.tabEvictionPolicy` | `reject`, `close_oldest`, `close_lru` (default) |
| `multiInstance.strategy` | `simple`, `explicit`, `simple-autorestart`, `always-on` (default), `no-instance` |
| `multiInstance.allocationPolicy` | `fcfs`, `round_robin`, `random` |
| `security.attach.allowSchemes` | `ws`, `wss`, `http`, `https` |

## Notes

- `config show` reports effective runtime values, not just raw file contents.
- `config get`, `set`, and `patch` operate on the file config model, not on transient env overrides.
- Most operational behavior now belongs in `config.json`, not in startup env vars.
