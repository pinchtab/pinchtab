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
- `instanceDefaults.tabPolicy.lifecycle`

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

Generated config files include a `$schema` URL for IDE completion and validation.

### `pinchtab config schema`

Prints the JSON Schema URL for this PinchTab build. Source builds, development
builds, and versions without a published schema use the `main` schema URL.
When a matching release schema is known, PinchTab uses that release tag; when a
newer matching schema is known, PinchTab uses the closest newer tag.

```bash
pinchtab config schema
```

Print the bundled schema JSON:

```bash
pinchtab config schema --print
```

### `pinchtab config show`

Shows the effective runtime configuration.

```bash
pinchtab config show
```

Secret values such as `server.token` remain masked in this output.
The Security section also includes `security.trustLoopbackProxy` as `Trust Loopback Proxy` so proxy trust posture is explicit.

### `pinchtab config token`

Copies the configured `server.token` to the system clipboard. Nothing is written
to stdout, so a shell capture returns empty rather than a message.

```bash
pinchtab config token             # copy to clipboard; status goes to stderr
TOKEN=$(pinchtab config token --stdout)   # print the token and nothing else
```

`--stdout` is the supported way to read the token on a host with no clipboard
tool. Without it the token is never printed.

If clipboard access is unavailable the command exits non-zero and names both
`--stdout` and the config file path on stderr, so a script sees the failure
instead of a success that copied nothing.

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

Reads a single dotted-path value and reports the **value in effect**: what the file
sets, and otherwise the value the runtime resolves — a shipped default, or a value
derived from another key. `profiles.baseDir` is the clearest case: leave it out of the
file and `config get` reports `<server.stateDir>/profiles`, the directory profiles are
actually kept in, rather than nothing. A key that is genuinely unset — a secret nobody
configured, an optional override such as `browser.binary` — still answers blank.

```bash
pinchtab config get server.port
pinchtab config get instanceDefaults.mode
pinchtab config get security.attach.allowHosts
pinchtab config get profiles.baseDir      # derived from server.stateDir when unset
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
- `PINCHTAB_RATE_LIMIT_MAX`: per-client request cap per 10-second window
  (default 3000, sized for agent-driven snapshot/action bursts). Lower it
  (e.g. to 300) when exposing the port beyond localhost. Child instances
  inherit it from the orchestrator's environment.

For remote CLI targeting, use the root `--server` flag instead of config.

## Config File Location

Default location by OS:

- macOS: `~/.pinchtab/config.json`
- Linux: `~/.pinchtab/config.json`
- Windows: `%APPDATA%\pinchtab\config.json`

On macOS and Linux, PinchTab defaults to `~/.pinchtab` so the CLI, npm-managed binary, and config file all use the same base directory.

If you are upgrading from an older macOS setup and still have a config at `~/Library/Application Support/pinchtab/config.json`, treat that as a legacy location and move or merge it into `~/.pinchtab/config.json`.

Override the config path with:

```bash
export PINCHTAB_CONFIG=/path/to/config.json
```

## Config Shape

Current nested file-config shape:

```json
{
  "$schema": "https://raw.githubusercontent.com/pinchtab/pinchtab/main/schema/config.json",
  "configVersion": "0.8.0",
  "server": {
    "port": "9867",
    "bind": "127.0.0.1",
    "token": "your-secret-token",
    "stateDir": "/path/to/state",
    "logLevel": "info",
    "networkBufferSize": 100,
    "retainNetworkBodies": false,
    "retainNetworkBodyMaxBytes": 262144,
    "trustProxyHeaders": false,
    "cookieSecure": null
  },
  "browser": {
    "version": "144.0.7559.133",
    "binary": "/path/to/chrome",
    "remoteDebuggingPort": null,
    "extraFlags": "--disable-gpu",
    "cloak": {
      "fingerprintSeed": "42069",
      "platform": "windows",
      "locale": "en-GB",
      "timezone": "Europe/London",
      "webrtcIP": "auto",
      "fontsDir": "/path/to/fonts",
      "storageQuotaMB": 2048,
      "disableDefaultStealthArgs": true
    },
    "extensionPaths": ["/path/to/pinchtab/extensions"]
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
    "captureAllowActivation": true,
    "humanize": false,
    "stealthLevel": "light",
    "tabEvictionPolicy": "close_lru",
    "tabPolicy": {
      "lifecycle": "keep",
      "closeDelaySec": 300,
      "restore": false
    },
    "dialogAutoAccept": false
  },
  "security": {
    "allowEvaluate": false,
    "allowMacro": false,
    "allowScreencast": false,
    "allowDownload": false,
    "allowCookies": false,
    "allowFileScheme": false,
    "allowedDomains": ["127.0.0.1", "localhost", "::1"],
    "downloadAllowedDomains": [],
    "downloadMaxBytes": 20971520,
    "allowUpload": false,
    "allowClipboard": false,
    "uploadMaxRequestBytes": 10485760,
    "uploadMaxFiles": 8,
    "uploadMaxFileBytes": 5242880,
    "uploadMaxTotalBytes": 10485760,
    "maxRedirects": -1,
    "trustedProxyCIDRs": [],
    "trustedResolveCIDRs": [],
    "attach": {
      "enabled": false,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "wss", "http", "https"],
      "forwardProxyAuth": false
    },
    "idpi": {
      "enabled": true,
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
    "defaultProfile": "default",
    "quarantineKeep": 1
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
  "autoSolver": {
    "enabled": false,
    "autoTrigger": true,
    "triggerOnNavigate": true,
    "triggerOnAction": true,
    "maxAttempts": 8,
    "solverTimeoutSec": 30,
    "retryBaseDelayMs": 500,
    "retryMaxDelayMs": 10000,
    "solvers": ["cloudflare", "semantic"],
    "llmProvider": "",
    "llmFallback": false,
    "external": {
      "capsolverKey": "",
      "twoCaptchaKey": ""
    }
  },
  "scheduler": {
    "enabled": false,
    "strategy": "fair-fifo",
    "maxQueueSize": 1000,
    "maxPerAgent": 100,
    "maxInflight": 20,
    "maxPerAgentInflight": 10,
    "resultTTLSec": 300,
    "workerCount": 4,
    "maxBatchSize": 50
  },
  "observability": {
    "activity": {
      "enabled": true,
      "sessionIdleSec": 1800,
      "retentionDays": 30,
      "events": {
        "dashboard": false,
        "server": false,
        "bridge": false,
        "orchestrator": false,
        "scheduler": false,
        "mcp": false,
        "other": false
      }
    }
  }
}
```

`autoSolver.external` is config-file-only. Capsolver and 2Captcha credentials
are stored there.

### Semantic Flow Credentials

The semantic-first autosolver flow injects credential values into recognised
login/signup/form fields. Configure them under `autoSolver.credentials` in
the config file:

```json
{
  "autoSolver": {
    "credentials": {
      "login":  { "user": "you@example.com", "password": "..." },
      "signup": { "name": "Jane Doe", "email": "you@example.com", "password": "..." },
      "form":   { "field1": "...", "field2": "...", "email": "you@example.com" }
    }
  }
}
```

Notes:

- Edit credentials by writing the config file directly. The dashboard config API
  redacts them on read (GET returns blanks) and preserves on-disk values when a
  PUT comes in with empty fields, so secrets never round-trip through the UI.
- The form solver step 2 falls back to `form.email` when `form.field2` is empty.
- Steps without a configured value fall through to a click-only flow (e.g. a
  login attempt with no password becomes a "click submit" attempt).

The dashboard Settings page exposes the non-secret AutoSolver settings and
shows the active config file path. Provider keys remain managed directly in the
config file.

### Browser Selection

The CLI uses `--browser <name>` to select a browser. In the config file the
equivalent field is `browsers.default`:

```json
{
  "browsers": { "default": "cloak" }
}
```

`browsers.default` explicitly selects the local browser backend:

- `chrome` uses the normal Chrome/Chromium launch path.
- `ghost-chrome` serves static-friendly reads from a lightweight fetcher and
  escalates to Chrome when a page needs rendering.
- `cloak` uses a discovered or configured local CloakBrowser Chromium binary.

When `browsers.default` is omitted, PinchTab prefers a discovered local
CloakBrowser installation and otherwise falls back to Chrome. Newly generated
config files make the same choice, while existing configs that explicitly set
`browsers.default` to `chrome` continue to use Chrome.

`browsers.available` optionally restricts which browsers requests may select.
For multiple named configurations of the same provider (different binaries,
proxies, or fingerprints), use `browser.targets` with `browser.defaultTarget`
and `browser.fallbackOrder` — see [Terminology](../architecture/terminology.md).
The legacy `browser.provider` field is no longer supported and is rejected at
validation time.

When the selected browser is `cloak`, PinchTab uses `browser.binary` when set
or searches its normal local CloakBrowser paths. A named CloakBrowser target
must set that target's `binary` explicitly. PinchTab does not download, bundle,
or redistribute the CloakBrowser binary.

On **macOS**, prefer a dedicated automation browser over your daily Google
Chrome. Launching `/Applications/Google Chrome.app` headless makes macOS treat
Chrome as already running, so opening your normal Chrome from the Dock just
activates the windowless automation process and no window appears (issue #583).
PinchTab's discovery now prefers Google Chrome for Testing, Chromium, then Chrome
Canary, and only falls back to your primary Chrome as a last resort. If only your
daily Chrome is installed, install [Google Chrome for
Testing](https://developer.chrome.com/blog/chrome-for-testing) or set
`browser.binary` to a separate Chrome/Chromium build. `pinchtab doctor browsers`
warns when automation would use your primary Chrome.

`browser.cloak` maps supported CloakBrowser fingerprint settings to native launch
flags:

- `fingerprintSeed` -> `--fingerprint`
- `platform` -> `--fingerprint-platform`
- `locale` -> `--fingerprint-locale`
- `timezone` -> `--fingerprint-timezone`
- `webrtcIP` -> `--fingerprint-webrtc-ip`
- `fontsDir` -> `--fingerprint-fonts-dir`
- `storageQuotaMB` -> `--fingerprint-storage-quota`

`disableDefaultStealthArgs` defaults to true for CloakBrowser targets. When set,
PinchTab keeps its process, profile, tab, extension, and action-control behavior,
but does not add its own JS stealth overlays or automation-hiding launch flags.
Set it to false only when you intentionally want PinchTab's legacy stealth layer
on top of CloakBrowser's native patches.

Advanced CloakBrowser flags can still go through `browser.extraFlags` when they
are not PinchTab-owned lifecycle flags.

`browser.proxy.server` is the prerequisite for the rest of the block:
`browser.proxy.username`, `password`, `bypassList` and every `browser.proxy.geo.*`
key do nothing without it, because there is no proxy to route through,
authenticate to, or align geo with. The values are still kept — `config set`
writes them, `config get` returns them, and setting the server afterwards makes
them take effect — and PinchTab reports the incomplete block on each config read
or write until a server is set. Clearing `browser.proxy.server` turns the proxy
off and leaves the other values on disk for the next server.

`browser.proxy.username` and `password` are the exception, and it is not about
the server: each requires the other, so `config set` aborts on either one alone
whichever order you try. Send the pair in one write instead:

```bash
pinchtab config patch '{"browser":{"proxy":{"username":"bob","password":"s3cret"}}}'
```

`browser.proxy.geo` is a CloakBrowser fingerprint-alignment hint. When a proxy
server and geo block are configured for a CloakBrowser target, PinchTab maps the
geo values into native CloakBrowser fingerprint flags unless the target already
sets the corresponding `browser.cloak` field. The stock `chrome` browser does
not derive `--lang`, `TZ`, or WebRTC launch settings from proxy geo data.

### Browser Extra Flags

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

For Linux container compatibility, use the runtime-managed path instead of `browser.extraFlags`. PinchTab enables `--no-sandbox` automatically when needed.

By default, PinchTab looks for unpacked Chrome extensions in `<server.stateDir>/extensions`. On a normal local install that means the OS-specific PinchTab config directory plus `extensions/`, for example:

- macOS: `~/.pinchtab/extensions`
- Linux: `~/.pinchtab/extensions`
- Windows: `%APPDATA%\\pinchtab\\extensions`

You can change or clear that default with `browser.extensionPaths`.

### Tab Policy

`instanceDefaults.tabPolicy` groups tab lifecycle behavior:

```json
{
  "instanceDefaults": {
    "tabPolicy": {
      "eviction": "close_lru",
      "lifecycle": "keep",
      "closeDelaySec": 300,
      "restore": false
    }
  }
}
```

- `eviction` controls what happens when `maxTabs` is reached: `close_lru`, `close_oldest`, or `reject`.
- `lifecycle` controls idle lifecycle behavior: `keep` disables lifecycle auto-close and is the default; `close_idle` auto-closes a tab after it handles an authorized `/text`, `/snapshot`, or `/action` request.
- `closeDelaySec` is the idle delay for `close_idle`. The default is `300` seconds when auto-close is enabled.
- `restore` controls whether session tabs are restored on startup. The default is `false`.

`instanceDefaults.tabEvictionPolicy` is still accepted for compatibility. New configs should use `instanceDefaults.tabPolicy.eviction`.

### Humanized Input

`instanceDefaults.humanize` controls whether click and typing actions use the slower humanized path by default. The default is `false`, which keeps automation fast and deterministic by using raw CDP input.

Callers can override the instance default per action with the JSON field `humanize`:

- `{"kind":"click","selector":"#submit","humanize":true}` opts a single action into bezier mouse movement and human-like delays.
- `{"kind":"type","selector":"#name","text":"Ada","humanize":true}` opts a single type action into the slower per-character path.

Rationale: humanized input is useful for compatibility with pages that react poorly to raw input, but it adds sleeps and multi-step pointer movement. Keeping it opt-in prevents accidental seconds of overhead in default E2E and agent runs.

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
| `observability` | Activity logging, source selection, and retention |

## `config get` And `config set` Support

`pinchtab config get` and `pinchtab config set` only support these top-level sections:

- `server`
- `browser`
- `instanceDefaults`
- `security`
- `profiles`
- `multiInstance`
- `timeouts`
- `scheduler`
- `observability`

They do not expose every field in those sections.

Use `pinchtab config patch` or edit `config.json` directly for fields such as:

- `server.networkBufferSize`
- `browser.extensionPaths`
- `instanceDefaults.dialogAutoAccept`
- `instanceDefaults.tabPolicy.*`
- `security.allowClipboard`
- `security.idpi.scanTimeoutSec`
- `security.idpi.shieldThreshold`
- `observability.activity.events.*`

## Common Examples

### Headed Mode

```json
{
  "instanceDefaults": {
    "mode": "headed"
  }
}
```

### Log Level For A Daemon Or Auto-Started Server

`pinchtab server --log-level` only reaches a server you start by hand. A
daemon-installed server (`pinchtab daemon install`) and the server a bare
`pinchtab nav` auto-starts both launch `pinchtab server` with no flags, so
`server.logLevel` is the only way to set their threshold — and it needs no change
to the unit file.

```bash
pinchtab config set server.logLevel warn    # Warnings and errors only
pinchtab config set server.logLevel debug   # Full debug detail while diagnosing
pinchtab config get server.logLevel
```

Accepted values are `debug`, `info` (the default), `warn` and `error`; an
unparseable value fails when the config loads, naming the accepted values.
`--log-level` still wins over the configured value. `-v` keeps its startup banner
either way, but it only raises the level when neither `--log-level` nor
`server.logLevel` is set — so a persisted `warn` survives `pinchtab server -v`, and
`--log-level debug` is how you override it for one run. `pinchtab bridge` reads the
same key and accepts the same flag.

### Network Bind With Token

```bash
pinchtab config set server.bind 0.0.0.0
pinchtab config set server.token secret
pinchtab server
```

Changing `server.bind` away from loopback is a documented, non-default, security-reducing deployment change. Use it only when remote reachability is intentional, keep a token set, and review the outer network boundary explicitly.

If the dashboard is served over plain HTTP on a non-loopback bind, PinchTab
shows an in-product warning because session cookies are no longer transport
encrypted. Prefer HTTPS or localhost when possible.

### Dashboard Cookie Transport

`server.cookieSecure` controls whether the dashboard session cookie must use the
`Secure` flag:

- `null` / unset / `auto`: default behavior. Session cookies are `Secure` on
  HTTPS and non-`Secure` on plain HTTP.
- `true`: always require `Secure`. Dashboard login works only over HTTPS.
- `false`: always omit `Secure`, even on HTTPS. Use only for operator-managed
  edge cases.

Examples:

```bash
pinchtab config set server.cookieSecure true
pinchtab config set server.cookieSecure false
pinchtab config set server.cookieSecure auto
```

When `server.cookieSecure = true`, plain-HTTP dashboard login fails explicitly
with an HTTPS-required error instead of appearing to succeed and looping.

If TLS terminates in front of PinchTab, also set `server.trustProxyHeaders=true`
only when the proxy is trusted and rewrites `Forwarded` / `X-Forwarded-*`
headers correctly.

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
      "allowSchemes": ["ws", "wss", "http", "https"],
      "forwardProxyAuth": false
    }
  }
}
```

`security.attach.allowHosts` is an allowlist. If you set it to `["*"]`, PinchTab accepts any reachable attach host with an allowed scheme. That is a documented, non-default, security-reducing override: it removes host allowlisting entirely and should only be used on isolated, operator-controlled networks.

`security.attach.forwardProxyAuth` controls whether PinchTab may send configured proxy authentication credentials over remote CDP attach. It defaults to `false`; enable it only when the attached browser process and CDP transport are trusted.

### Quarantined Profile Retention

When a browser profile turns out to be unusable, PinchTab renames it aside as
`<profile>.quarantine-<unix>` and starts fresh. PinchTab keeps the most recent
quarantined copy of each profile and prunes the older ones at the moment a new quarantine
is created — the newest is the copy most likely to relate to a problem being investigated
now, and nothing in the product reads the older ones. Every removal is logged with the
path and the bytes reclaimed.

```json
{
  "profiles": {
    "quarantineKeep": 1
  }
}
```

`profiles.quarantineKeep` defaults to `1`. Set it to `0` to keep every quarantined
profile, which is exactly the behaviour before this setting existed. Pruning only ever
runs when a new quarantine is created, never on startup or a timer, so quarantined copies
of a profile that never fails again are left alone — clearing that backlog is a separate,
explicit operation.

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

Activity logs are always written to `<server.stateDir>/activity`, so two instances cannot
end up sharing one log directory. `observability.activity.stateDir` is therefore not
settable: `config set` refuses it, and `config get observability.activity.stateDir` reports
the derived directory in effect. Move the logs with `server.stateDir`.

A file that already carries the key keeps loading and keeps working. The key is **ignored**,
and saying so is an *advisory*, not a validation error: PinchTab reports it — at load, and
on stderr when you run `config set`, `config patch` or `config validate` — and it never
blocks anything. There is nothing to fix and nothing to remove; the value simply has no
effect. Validation errors, such as an out-of-range `server.port`, still block a save.

`server.trustProxyHeaders` should stay `false` unless PinchTab is behind a trusted reverse proxy that overwrites `Forwarded` and `X-Forwarded-*` headers. Do not enable it on direct-exposure deployments or behind proxies that pass client-supplied forwarding headers through unchanged. When it is enabled, the client-most forwarded address is also the client identity: rate-limit buckets, the concurrent-stream cap and every audit line are keyed on it rather than on the proxy's address.

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
- valid `instanceDefaults.tabPolicy.eviction`
- valid `instanceDefaults.tabPolicy.lifecycle`
- non-negative `instanceDefaults.tabPolicy.closeDelaySec`
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
| `instanceDefaults.tabPolicy.eviction` | `reject`, `close_oldest`, `close_lru` |
| `instanceDefaults.tabPolicy.lifecycle` | `keep`, `close_idle` |
| `multiInstance.strategy` | `simple`, `explicit`, `simple-autorestart`, `always-on`, `no-instance` |
| `multiInstance.allocationPolicy` | `fcfs`, `round_robin`, `random` |
| `security.attach.allowSchemes` | `ws`, `wss`, `http`, `https` |
| `security.attach.forwardProxyAuth` | `true`, `false` |

## Notes

- `config show` reports effective runtime values, not just raw file contents.
- `config get` reports the value in effect, including shipped defaults and values derived from another key. `config set` and `patch` write the file config model and do not carry transient runtime overrides.
- the dashboard config API treats `server.token` as write-only; use the CLI or file editing to manage it.
