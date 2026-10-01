# CLI Overview

`pinchtab` is driven by direct commands. Running it with no subcommand prints a
status summary and next-step hints.

When you target a remote server with `--server`, the CLI is exercising the same privileged control plane as the dashboard and HTTP API. Do not use it as an access path for untrusted users or untrusted systems. For deployment guidance, see [Security](../guides/security.md).

## Bare `pinchtab`

Running `pinchtab` with no subcommand does not start the server. When the config file
is new or its `configVersion` is older than this build, it first runs the security
setup (generating `server.token` if missing; the prompts only appear in an interactive
terminal). It then prints the server state, where logs go, `allowedDomains`, IDPI
state, and suggested next commands, for example:

```text
PinchTab dev

  server               protected listener
  logs                 stdout/stderr of the terminal running `pinchtab server`
  allowedDomains       a.com, b.com
  idpi                 enabled

Next steps:
  pinchtab config token --stdout               # print the API token (capture with $(...))
  pinchtab health --json                       # retry health with the current token
  pinchtab config show                         # inspect configured port and token
```

Advisory hints about a steady state you may have chosen print once per run; set
`PINCHTAB_HINTS=off` to silence them.

## Direct Commands

Use direct commands when you already know the action you want:

```bash
pinchtab server
pinchtab bridge
pinchtab mcp
pinchtab config
pinchtab --agent-id agent-main nav https://pinchtab.com
pinchtab nav https://pinchtab.com
pinchtab snap -i -c
pinchtab click e5
pinchtab find "login button"
pinchtab network --limit 20
```

`pinchtab nav <url>` auto-starts the local PinchTab server when it is not already running. Explicit `--server` and `PINCHTAB_SERVER` targets are used as-is and are not auto-started. The default config uses a headless browser, so `nav` may succeed without opening a visible window. To navigate and snapshot in one command after install, run:

```bash
pinchtab nav https://pinchtab.com --snap
```

Global flags such as `--server` and `--agent-id` apply to direct command mode. `--agent-id` is recorded in activity logs and dashboard agent views so multiple CLI-driven agents are distinguishable.

## Agent Attribution

CLI requests carry agent identity over the `X-Agent-Id` request header.

- `--agent-id <value>` sets the header explicitly for that command
- `PINCHTAB_AGENT_ID` sets the default agent ID for the current shell or script
- if neither is set, the CLI sends no `X-Agent-Id`; a request authenticated with an
  agent session (`PINCHTAB_SESSION`) is attributed to that session's agent ID

That agent ID is what appears as `agentId` in `/api/activity`, the Agents page, and scheduler-driven activity.

Example:

```bash
PINCHTAB_AGENT_ID=agent-crawl-01 pinchtab nav https://pinchtab.com
curl 'http://127.0.0.1:9867/api/activity?agentId=agent-crawl-01'
```

## Output Format

Most commands output human-readable text by default. Use `--json` for structured output:

```bash
pinchtab tab                  # *abc123  https://...  Title
pinchtab tab --json           # {"tabs":[...]}
pinchtab frame                # main
pinchtab network              # GET  200  https://...
```

**For scripts**: Always use `--json` when piping or parsing programmatically. Human-readable output may change between versions. JSON is the stable contract.

## Exit Codes

A mistyped verb is an error everywhere: an unrecognised command or subcommand exits `1` and
names the valid subcommands, at the top level and inside every group. `pinchtab cache clera`
does not share an exit code with `pinchtab cache clear`, so `set -e` and `&&` chains stop on
a typo instead of continuing as though the state had been reset.

```bash
$ pinchtab cache clera ; echo exit=$?
unknown command "clera" for "pinchtab cache"
Valid subcommands: clear, status
exit=1
```

Two commands take an argument rather than a subcommand and are unaffected: `pinchtab tab
<id>` focuses a tab, and `pinchtab network <requestId>` inspects one captured request (filters use
`--filter`, `--method`, `--status`, `--type`). An unknown
value there is data the server rejects, not a typo the CLI can catch.

## Core Commands

| Command | Purpose |
| --- | --- |
| `pinchtab server` | Start the full server and dashboard |
| `pinchtab server stop` | Stop the running server (foreground or background) |
| `pinchtab server restart` | Stop + restart in background (applies config changes) |
| `pinchtab bridge` | Start the single-instance bridge runtime |
| `pinchtab mcp` | Start the stdio MCP server |
| `pinchtab daemon` | Show daemon status and manage the background service |
| `pinchtab config` | Print the config overview (read-only) |
| `pinchtab security` | Print the runtime security posture |
| `pinchtab completion <shell>` | Generate shell completion scripts |
| `pinchtab version` | Print the PinchTab version (same as `--version`) |

### Server Flags

```bash
pinchtab server [flags]
```

| Flag | Short | Purpose |
| --- | --- | --- |
| `--yolo` | `-y` | Apply the guards-down preset for this run only; the config file is unchanged (every capability gate except state export on, attach on, IDPI off) |
| `--headed` | `-H` | Start the default instance in headed (visible) mode |
| `--extension <path>` | `-e` | Load browser extension (repeatable) |
| `--browser <name>` | | `chrome`, `cloak`, or `ghost-chrome` (overrides config) |
| `--bind <addr>` | | HTTP bind address (overrides `server.bind`) |
| `--port <port>` | | HTTP port (overrides `server.port`) |
| `--log-level <level>` | | `debug`, `info` (default), `warn`, or `error` |
| `--verbose` | `-v` | Full startup banner; logs at debug only when neither `--log-level` nor `server.logLevel` is set |
| `--background` | `-b` | Spawn the server detached and print JSON with pid/url/token |

Examples:

```bash
pinchtab server -y                  # guards down for local dev
pinchtab server -H                  # visible browser for debugging
pinchtab server -yH                 # both combined
pinchtab server -e ./my-extension   # load extension
```

**Note:** Use `--headed` only when you need visual feedback (debugging, manual testing). Headless mode is more resource-efficient for automation.

## Browser Commands

The browser control surface is top-level. `tab` is only for list/focus/close.

Common commands:

| Command | Purpose |
| --- | --- |
| `pinchtab nav <url>` | Navigate current tracked tab, or create one if needed |
| `pinchtab nav <url> --timeout 90` | Override the navigation timeout in seconds (maximum 120) |
| `pinchtab nav <url> --snap` | Navigate and output an interactive compact snapshot |
| `pinchtab snap [selector]` | Accessibility snapshot for the current tab, optionally scoped |
| `pinchtab frame [target\|main]` | Show or set selector frame scope |
| `pinchtab click <selector>` | Click an element |
| `pinchtab mouse move <x> <y>` | Move the pointer to coordinates |
| `pinchtab mouse down [selector]` | Press a mouse button at the current pointer or a fresh target |
| `pinchtab mouse up [selector]` | Release a mouse button at the current pointer or a fresh target |
| `pinchtab mouse wheel [dy\|selector]` | Dispatch wheel deltas at the current pointer or a fresh target |
| `pinchtab drag <from> <to>` | Drag from one target to another |
| `pinchtab type <selector> <text>` | Type via key events |
| `pinchtab fill <selector> <text>` | Fill directly |
| `pinchtab text` | Extract page text (`--full`, `--raw`, `--frame <frameId>`) |
| `pinchtab find <query>` | Semantic element search |
| `pinchtab extract --schema <file\|->` | Schema-typed JSON from the page (`--scope`, `--max-items`, `--fields`, `--explain`) |
| `pinchtab screenshot` | Save a screenshot (`-s/--selector` captures a specific element, `--scale <f>` rescales the bitmap, `--beyond-viewport` captures the full scrollable document) |
| `pinchtab capture` | Paired screenshot + accessibility snapshot from the same DOM epoch (`--scale`, `--beyond-viewport`, `--require-pair`, `--with-bounds`) |
| `pinchtab pdf` | Export the page as PDF |
| `pinchtab network` | Inspect captured network requests |
| `pinchtab wait ...` | Wait for selector, text, URL, network idle, JS, or time |
| `pinchtab console` | Show browser console logs |
| `pinchtab errors` | Show browser error logs |

Many browser commands accept `--tab <id>` to target an existing tab instead of the active one.

Selector lookup is explicit by frame. Unscoped selectors stay in the main document unless you set a frame first with `pinchtab frame`. Same-origin iframe scopes are supported; cross-origin iframe descendants are not currently exposed.

`pinchtab text` follows that frame model too: it uses the active frame scope
unless you override it with `--frame`.

`pinchtab eval` is separate from that model and does not inherit current frame scope.

Selector-based actions fail fast when a selector does not match. If you expect
dynamic content to appear shortly, use `pinchtab wait` first.

Manual handoff is available via the `tab` command:

```bash
pinchtab tab handoff <tabId> --reason captcha --timeout-ms 120000
pinchtab tab handoff-status <tabId>
pinchtab tab resume <tabId> --status completed
```

API equivalents:

Paused handoff state blocks action execution routes (`/action`, `/actions`, `/macro`) with `409 tab_paused_handoff`
until resumed or expired via timeout.

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/handoff \
  -H "Content-Type: application/json" \
  -d '{"reason":"captcha"}'
curl http://localhost:9867/tabs/<tabId>/handoff
curl -X POST http://localhost:9867/tabs/<tabId>/resume \
  -H "Content-Type: application/json" \
  -d '{"status":"completed"}'
```

## Tab Command

`pinchtab tab` is intentionally small:

```bash
pinchtab tab
pinchtab tab <id>
pinchtab tab close <id>
pinchtab tab handoff <id>
pinchtab tab handoff-status <id>
pinchtab tab resume <id>
```

For tab-scoped actions, use the normal top-level command with `--tab`:

```bash
pinchtab click --tab <id> e5
pinchtab pdf --tab <id> -o page.pdf
```

## Config From The CLI

`pinchtab config` prints a read-only overview:

- `multiInstance.strategy`
- `instanceDefaults.stealthLevel`
- `instanceDefaults.tabEvictionPolicy`
- `instanceDefaults.tabPolicy.lifecycle`
- the active config file path
- the masked server token
- the dashboard URL when the server is running
- hints for `config get/set/show`, `config token` and `pinchtab security`

For file schema details and `config get/set/patch`, see [Config](./config.md).

## Security From The CLI

`pinchtab security` prints the runtime security posture and recommended defaults.

Direct subcommands:

```bash
pinchtab security up
pinchtab security down
```

`pinchtab security down` applies the documented, non-default, security-reducing preset for local operator workflows. It is not the baseline security posture.

For broader security guidance, see [Security Guide](../guides/security.md).

## Daemon

`pinchtab daemon` supports:

- macOS via `launchd`
- Linux via user `systemd`

Windows binaries exist, but daemon workflows are not currently supported there. Use `pinchtab server` or `pinchtab bridge` directly.

For operational details, see [Background Service (Daemon)](../guides/daemon.md).

## Full Command Tree

Use built-in help for the live command tree:

```bash
pinchtab --help
```

For per-command pages, start at [Reference Index](./index.md).
