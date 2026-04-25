# CLI Overview

`pinchtab` has two normal usage styles:

- interactive menu mode
- direct command mode

Use the menu when you want a guided local control surface. Use direct commands when you want shell history, scripts, or remote targeting with `--server`.

When you target a remote server with `--server`, the CLI is exercising the same privileged control plane as the dashboard and HTTP API. Do not use it as an access path for untrusted users or untrusted systems. For deployment guidance, see [Security](../guides/security.md).

## Interactive Menu

Running `pinchtab` with no subcommand in an interactive terminal opens the menu. It does not immediately start the server.

Typical flow:

```text
listen    running  127.0.0.1:9867
str,plc   simple,fcfs
daemon    ok
security  [■■■■■■■■■■]  LOCKED

Main Menu
  1. Start server
  2. Daemon
  3. Start bridge
  4. Start MCP server
  5. Config
  6. Security
  7. Help
  8. Exit
```

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

Global flags such as `--server` and `--agent-id` apply to direct command mode. `--agent-id` is recorded in activity logs and dashboard agent views so multiple CLI-driven agents are distinguishable.

## Agent Attribution

CLI requests carry agent identity over the `X-Agent-Id` request header.

- `--agent-id <value>` sets the header explicitly for that command
- `PINCHTAB_AGENT_ID` sets the default agent ID for the current shell or script
- if neither is set, the CLI uses `cli`

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

## Core Commands

| Command | Purpose |
| --- | --- |
| `pinchtab server` | Start the full server and dashboard |
| `pinchtab bridge` | Start the single-instance bridge runtime |
| `pinchtab mcp` | Start the stdio MCP server |
| `pinchtab daemon` | Show daemon status and manage the background service |
| `pinchtab config` | Open the interactive config overview/editor |
| `pinchtab security` | Open the interactive security overview |
| `pinchtab completion <shell>` | Generate shell completion scripts |

### Server Flags

```bash
pinchtab server [flags]
```

| Flag | Short | Purpose |
| --- | --- | --- |
| `--yolo` | `-y` | Apply guards down preset (enables evaluate, macro, download) |
| `--headed` | `-H` | Start browser instances in headed (visible) mode |
| `--extension <path>` | `-e` | Load browser extension (repeatable) |

Examples:

```bash
pinchtab server -y                  # guards down for local dev
pinchtab server -H                  # visible browser for debugging
pinchtab server -yH                 # both combined
pinchtab server -e ./my-extension   # load extension
```

**Note:** Use `--headed` only when you need visual feedback (debugging, manual testing). Headless mode is more resource-efficient for automation.

## Browser Commands

The browser control surface is top-level. `tab` is only for list/focus/new/close.

Common commands:

| Command | Purpose |
| --- | --- |
| `pinchtab nav <url>` | Open a new tab and navigate it |
| `pinchtab quick <url>` | Navigate and snapshot |
| `pinchtab snap` | Accessibility snapshot |
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
| `pinchtab screenshot` | Save a screenshot (`-s/--selector` captures a specific element, `--css-1x` exports selector shots at CSS size) |
| `pinchtab pdf` | Export the page as PDF |
| `pinchtab network` | Inspect captured network requests |
| `pinchtab wait ...` | Wait for selector, text, URL, JS, or time |
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
pinchtab tab new [url]
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

`pinchtab config` shows:

- `multiInstance.strategy`
- `multiInstance.allocationPolicy`
- `instanceDefaults.stealthLevel`
- `instanceDefaults.tabEvictionPolicy`
- the active config file path
- the dashboard URL when the server is running
- the masked server token
- a `Copy token` action

For file schema details and `config get/set/patch`, see [Config](./config.md).

## Security From The CLI

`pinchtab security` is the interactive security screen.

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
