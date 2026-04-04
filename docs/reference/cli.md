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

## Browser Commands

The browser control surface is top-level. `tab` is only for list/focus/new/close.

Common commands:

| Command | Purpose |
| --- | --- |
| `pinchtab nav <url>` | Open a new tab and navigate it |
| `pinchtab quick <url>` | Navigate and snapshot |
| `pinchtab snap` | Accessibility snapshot |
| `pinchtab click <selector>` | Click an element |
| `pinchtab type <selector> <text>` | Type via key events |
| `pinchtab fill <selector> <text>` | Fill directly |
| `pinchtab text` | Extract page text |
| `pinchtab find <query>` | Semantic element search |
| `pinchtab screenshot` | Save a screenshot |
| `pinchtab pdf` | Export the page as PDF |
| `pinchtab network` | Inspect captured network requests |
| `pinchtab wait ...` | Wait for selector, text, URL, JS, or time |
| `pinchtab console` | Show browser console logs |
| `pinchtab errors` | Show browser error logs |

Many browser commands accept `--tab <id>` to target an existing tab instead of the active one.

## Tab Command

`pinchtab tab` is intentionally small:

```bash
pinchtab tab
pinchtab tab <id>
pinchtab tab new [url]
pinchtab tab close <id>
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
