# Configuration

Complete reference for PinchTab configuration. Supports environment variables, config files (JSON), and CLI commands.

PinchTab has two instance ownership models:
- `launch`: PinchTab starts and manages Chrome
- `attach`: PinchTab connects to an externally managed Chrome instance

The config file defines defaults and policy for those models. It does not define a global instance-specific CDP target.

## Configuration Priority

Values are loaded in this order (highest priority first):

1. **Environment variables**
2. **Config file** — `~/.config/pinchtab/config.json` (or `~/.pinchtab/config.json` legacy)
3. **Built-in defaults**

Only a small operational env surface remains:
- `PINCHTAB_CONFIG`
- `PINCHTAB_BIND`
- `PINCHTAB_PORT`
- `PINCHTAB_TOKEN`
- `CHROME_BIN`

Everything else should be configured in `config.json`.

## Config File

### Location

Default location varies by OS:
- **macOS:** `~/Library/Application Support/pinchtab/config.json`
- **Linux:** `~/.config/pinchtab/config.json` (or `$XDG_CONFIG_HOME/pinchtab/config.json`)
- **Windows:** `%APPDATA%\pinchtab\config.json`

For backward compatibility, `~/.pinchtab/config.json` is used if it exists and the new location doesn't.

Override with `PINCHTAB_CONFIG=/path/to/config.json`.

### Format

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
    "stealthLevel": "light",
    "tabEvictionPolicy": "reject",
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
    "allowUpload": false
  },
  "profiles": {
    "baseDir": "/path/to/profiles",
    "defaultProfile": "default"
  },
  "multiInstance": {
    "strategy": "simple",
    "allocationPolicy": "fcfs",
    "instancePortStart": 9868,
    "instancePortEnd": 9968
  },
  "attach": {
    "enabled": false,
    "allowHosts": ["127.0.0.1", "localhost", "::1"],
    "allowSchemes": ["ws", "wss"]
  },
  "timeouts": {
    "actionSec": 30,
    "navigateSec": 60,
    "shutdownSec": 10,
    "waitNavMs": 1000
  }
}
```

### Section Semantics

- `server`: PinchTab HTTP server settings.
- `browser`: Chrome executable/runtime wiring used when PinchTab launches Chrome.
- `instanceDefaults`: Default launch-time behavior for managed instances.
- `security`: Feature gates for sensitive endpoints.
- `profiles`: Shared profile storage model for both single-instance and multi-instance flows.
- `multiInstance`: Orchestration strategy and instance port allocation.
- `attach`: Policy for whether attach is allowed and which remote CDP targets are acceptable.
- `timeouts`: PinchTab runtime timeouts.

### Legacy Flat Format

Older flat format is still supported for backward compatibility:

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

Run `pinchtab config init` to generate a new config with the nested format.

## Environment Variables

Environment variables always take precedence over config file values.

### Operational Env Vars

| Variable | Default | Description |
|----------|---------|-------------|
| `PINCHTAB_PORT` | `9867` | HTTP server port |
| `PINCHTAB_BIND` | `127.0.0.1` | Bind address |
| `PINCHTAB_TOKEN` | (none) | API authentication token |
| `PINCHTAB_CONFIG` | (OS config dir)/config.json | Config file path |
| `CHROME_BIN` | (auto) | Chrome binary path |

All behavior settings such as display mode, feature gates, profile defaults, attach policy, timeouts, and multi-instance strategy live in `config.json`.

## CLI Commands

### `pinchtab config init`

Create a default config file:

```bash
pinchtab config init
```

### `pinchtab config show`

Show current effective configuration:

```bash
pinchtab config show
```

### `pinchtab config path`

Show config file path:

```bash
pinchtab config path
```

### `pinchtab config validate`

Validate config file:

```bash
pinchtab config validate
```

Checks for:
- Valid port numbers (1-65535)
- Valid enum values (strategy, stealthLevel, tabEvictionPolicy, etc.)
- Valid attach schemes (`ws`, `wss`)
- Valid timeout values (non-negative)
- Valid instance port range (`start <= end`)

## Examples

### Basic Setup

```bash
pinchtab
```

Runs on `localhost:9867`, headless, no authentication.

### With Authentication

```bash
PINCHTAB_TOKEN=my-secret-token pinchtab
```

Or in config file:

```json
{
  "server": {
    "token": "my-secret-token"
  }
}
```

### Network Accessible

```bash
PINCHTAB_BIND=0.0.0.0 PINCHTAB_TOKEN=secret pinchtab
```

Always use a token when binding to `0.0.0.0`.

### Headed Mode for Debugging

```json
{
  "instanceDefaults": {
    "mode": "headed"
  }
}
```

### Attach Policy

Enable attach mode only if you want PinchTab to accept attach requests to externally managed Chrome instances.

```json
{
  "attach": {
    "enabled": true,
    "allowHosts": ["127.0.0.1", "localhost", "chrome.internal"],
    "allowSchemes": ["ws", "wss"]
  }
}
```

This is policy only. The actual `cdpUrl` belongs to the attach request, not global config.

### Custom Ports

```bash
PINCHTAB_PORT=8080 pinchtab server
```

Or in config file:

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

When max tabs is reached:
- `reject` — return error
- `close_oldest` — close oldest tab by creation time
- `close_lru` — close least recently used tab

```json
{
  "instanceDefaults": {
    "maxTabs": 10,
    "tabEvictionPolicy": "close_lru"
  }
}
```

## Validation

All enum fields are validated on load:

| Field | Valid Values |
|-------|--------------|
| `instanceDefaults.mode` | `headless`, `headed` |
| `instanceDefaults.stealthLevel` | `light`, `medium`, `full` |
| `instanceDefaults.tabEvictionPolicy` | `reject`, `close_oldest`, `close_lru` |
| `multiInstance.strategy` | `simple`, `explicit`, `simple-autorestart` |
| `multiInstance.allocationPolicy` | `fcfs`, `round_robin`, `random` |
| `attach.allowSchemes` | `ws`, `wss` |

Run `pinchtab config validate` to check your config file.

## Related Documentation

- [API Reference](endpoints.md)
- [CLI Reference](cli-quick-reference.md)
- [Instance API](instance-api.md)
