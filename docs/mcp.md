# MCP Server

PinchTab includes a native [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that lets AI agents control the browser through MCP over stdio.

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

PinchTab currently exposes 34 tools:

- Navigation: 4
- Interaction: 8
- Keyboard: 4
- Content: 3
- Tab management: 5
- Wait utilities: 6
- Network: 3
- Dialog: 1

### Navigation

- `pinchtab_navigate`
- `pinchtab_snapshot`
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

### Tab Management

- `pinchtab_list_tabs`
- `pinchtab_close_tab`
- `pinchtab_health`
- `pinchtab_cookies`
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

1. `pinchtab_navigate`
2. `pinchtab_snapshot`
3. `pinchtab_click` / `pinchtab_type` / other action tools
4. `pinchtab_wait_*` or `pinchtab_network` when needed

For full parameter details, see [MCP Tool Reference](./reference/mcp-tools.md).
