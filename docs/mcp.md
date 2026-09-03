# MCP Server

PinchTab includes a native [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that lets AI agents control the browser through MCP over stdio.

> [!WARNING]
> The MCP server is part of PinchTab's privileged control plane. It is intended for trusted operators and trusted agent systems only. Do not expose it to untrusted users, untrusted client systems, or the public internet. If you are unsure how to secure a non-local deployment, review [Security](guides/security.md) and use the private security contact path in `SECURITY.md` before exposing the service.

> [!CAUTION]
> By default, PinchTab's IDPI posture is meant to keep MCP browsing local-only until you deliberately widen it. Expanding MCP use to non-local or non-trusted domains is a security-reducing choice.
>
> When MCP tools read page content from wider domains, treat `pinchtab_snapshot` and `pinchtab_get_text` output as untrusted data, not instructions. Hostile pages can contain prompt-injection content, poisoned text, or other material that should never be treated as operator guidance. Review [Security](guides/security.md#idpi) before relaxing domain restrictions.

## Quick Start

1. Start PinchTab in server or bridge mode:
   ```bash
   pinchtab server
   # or
   pinchtab bridge
   ```
2. Start the MCP server in another terminal or from your MCP client config:
   ```bash
   pinchtab mcp
   ```

The MCP server communicates over stdio using JSON-RPC, which is the standard MCP transport.

## Client Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"]
    }
  }
}
```

### VS Code / GitHub Copilot

```json
{
  "servers": {
    "pinchtab": {
      "type": "stdio",
      "command": "pinchtab",
      "args": ["mcp"]
    }
  }
}
```

### Cursor

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"]
    }
  }
}
```

### Grok Build

After PinchTab is listed in the official xAI marketplace, install the plugin with:

```bash
grok plugin install pinchtab --trust
```

Until then, or to install from the PinchTab repository marketplace:

```bash
grok plugin marketplace add pinchtab/pinchtab
grok plugin install pinchtab --trust
```

You can also install the plugin directory directly from GitHub:

```bash
grok plugin install pinchtab/pinchtab#plugins/grok --trust
```

From the root of a local checkout, use `grok plugin install ./plugins/grok --trust`. `--trust` is required before MCP starts. The plugin does not install the `pinchtab` binary; install PinchTab separately and run `pinchtab server` or the local daemon.

To wire MCP yourself instead of the plugin, add to `~/.grok/config.toml`:

```toml
[mcp_servers.pinchtab]
command = "pinchtab"
args = ["mcp"]
```

For first use, verification, domain authorization, and troubleshooting, see the [Grok plugin install and usage guide](../plugins/grok/README.md).

## Environment

| Variable | Description |
| --- | --- |
| `PINCHTAB_TOKEN` | Auth token for secured servers |

For remote servers, use the root `--server` flag with that host's credential — a non-loopback host requires `PINCHTAB_TOKEN` (or `PINCHTAB_SESSION`), since the CLI refuses to send the local config's `server.token` off the machine:

```bash
PINCHTAB_TOKEN=<that-host-token> pinchtab --server http://remote:9867 mcp
```

## Available Tools

PinchTab currently exposes 38 tools:

- Navigation: 9
- Interaction: 8
- Keyboard: 1
- Content: 3
- Recording: 1
- Site: 1
- Tab management: 5
- Wait utilities: 1
- Network: 5
- Diagnostics: 2
- Dialog: 1

### Navigation

- `pinchtab_navigate`
- `pinchtab_back` — go back one history entry; returns the tab ID and the URL landed on
- `pinchtab_forward` — go forward one history entry; returns the tab ID and the URL landed on
- `pinchtab_reload` — reload the current page; returns the tab ID and the URL landed on
- `pinchtab_snapshot`
- `pinchtab_frame`
- `pinchtab_screenshot`
- `pinchtab_capture` — paired screenshot + snapshot from one DOM epoch
- `pinchtab_get_text`

### Interaction

- `pinchtab_click`
- `pinchtab_type`
- `pinchtab_hover`
- `pinchtab_focus`
- `pinchtab_select`
- `pinchtab_scroll`
- `pinchtab_scroll_into_view`
- `pinchtab_fill`

### Keyboard

- `pinchtab_key` — `action` is `press`, `down`, `up`, `type` or `insert`

### Content

- `pinchtab_eval`
- `pinchtab_pdf`
- `pinchtab_find`

### Recording

- `pinchtab_record` — `action` is `start`, `stop` or `status`

### Site

- `pinchtab_scrape` — crawl a site to markdown (HTTP-first, browser-enrich thin/JS pages). Use `preview=true` for a cheap outline, then expand chosen URLs with `only`.

### Tab Management

- `pinchtab_list_tabs`
- `pinchtab_close_tab`
- `pinchtab_health`
- `pinchtab_cookies` (requires `security.allowCookies`)
- `pinchtab_cookies_set` (requires `security.allowCookies`)
- `pinchtab_connect_profile`

### Wait Utilities

- `pinchtab_wait` — `for` is `ms`, `selector`, `text`, `url`, `load` or `function`, and `value` carries the condition

### Network

- `pinchtab_network`
- `pinchtab_network_detail`
- `pinchtab_network_clear`
- `pinchtab_network_route`
- `pinchtab_network_unroute`

### Diagnostics

- `pinchtab_console` — read (or `clear`) the tab's browser console logs
- `pinchtab_errors` — read (or `clear`) the tab's uncaught JavaScript exceptions; check this when a snapshot looks healthy but actions do nothing

### Dialog

- `pinchtab_dialog`

## Selector Model

For selector-based interaction tools, prefer `selector`. `ref` is still accepted as a deprecated fallback on the element-action tools.

Common selector forms:

- `e5`
- `#login`
- `xpath://button`
- `text:Submit`
- `find:login button`

## Practical Flow

The normal MCP browser loop is:

1. Call `pinchtab_navigate` with a `url`
2. Call `pinchtab_snapshot` to inspect page structure and collect refs
3. Call `pinchtab_click`, `pinchtab_type`, or other action tools with structured arguments
4. Call `pinchtab_wait_*` or `pinchtab_network` when needed
5. Call `pinchtab_back` to leave a dead end, or `pinchtab_reload` to retry the page

`pinchtab_back`, `pinchtab_forward` and `pinchtab_reload` take an optional `tabId`
and `snap`, so `snap: true` returns the page after the navigation in one
round-trip.

`pinchtab_snapshot` supports MCP-safe output controls:

- `compact=true` or `format="compact"` for the most token-efficient text snapshot
- `format="text"` for the full text snapshot
- `noAnimations=true` to reduce animation noise before capture

For full parameter details, see [MCP Tool Reference](./reference/mcp-tools.md).
