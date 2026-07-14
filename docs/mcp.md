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

## Environment

| Variable | Description |
| --- | --- |
| `PINCHTAB_TOKEN` | Auth token for secured servers |

For remote servers, use the root `--server` flag:

```bash
pinchtab --server http://remote:9867 mcp
```

## Available Tools

PinchTab currently exposes 39 tools:

- Navigation: 5
- Interaction: 9
- Keyboard: 4
- Content: 3
- Site: 1
- Tab management: 5
- Wait utilities: 6
- Network: 5
- Dialog: 1

### Navigation

- `pinchtab_navigate`
- `pinchtab_snapshot`
- `pinchtab_frame`
- `pinchtab_screenshot`
- `pinchtab_get_text`

### Interaction

- `pinchtab_click`
- `pinchtab_type`
- `pinchtab_press`
- `pinchtab_hover`
- `pinchtab_focus`
- `pinchtab_select`
- `pinchtab_scroll`
- `pinchtab_scroll_into_view`
- `pinchtab_fill`

### Keyboard

- `pinchtab_keyboard_type`
- `pinchtab_keyboard_inserttext`
- `pinchtab_keydown`
- `pinchtab_keyup`

### Content

- `pinchtab_eval`
- `pinchtab_pdf`
- `pinchtab_find`

### Site

- `pinchtab_scrape` — crawl a site to markdown (HTTP-first, browser-enrich thin/JS pages). Use `preview=true` for a cheap outline, then expand chosen URLs with `only`.

### Tab Management

- `pinchtab_list_tabs`
- `pinchtab_close_tab`
- `pinchtab_health`
- `pinchtab_cookies` (requires `security.allowCookies`)
- `pinchtab_connect_profile`

### Wait Utilities

- `pinchtab_wait`
- `pinchtab_wait_for_selector`
- `pinchtab_wait_for_text`
- `pinchtab_wait_for_url`
- `pinchtab_wait_for_load`
- `pinchtab_wait_for_function`

### Network

- `pinchtab_network`
- `pinchtab_network_detail`
- `pinchtab_network_clear`
- `pinchtab_network_route`
- `pinchtab_network_unroute`

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

`pinchtab_snapshot` supports MCP-safe output controls:

- `compact=true` or `format="compact"` for the most token-efficient text snapshot
- `format="text"` for the full text snapshot
- `noAnimations=true` to reduce animation noise before capture

For full parameter details, see [MCP Tool Reference](./reference/mcp-tools.md).
