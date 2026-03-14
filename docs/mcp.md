# MCP Server

PinchTab includes a native [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that lets AI agents control the browser directly through the standardized MCP interface.

## Quick Start

1. **Start PinchTab** in any mode (server or bridge):
   ```bash
   pinchtab server
   #or
   pinchtab daemon install
   ```

2. **Start the MCP server** (in a separate terminal or from your MCP client config):
   ```bash
   pinchtab mcp
   ```

The MCP server communicates over **stdio** (stdin/stdout with JSON-RPC 2.0), which is the standard transport for MCP tools.

## Client Configuration

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"],
      "env": {
        "PINCHTAB_URL": "http://127.0.0.1:9867"
      }
    }
  }
}
```

### VS Code / GitHub Copilot

Create `.vscode/mcp.json` in your workspace:

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

Add to your Cursor MCP settings:

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

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PINCHTAB_URL` | `http://127.0.0.1:9867` | PinchTab server URL |
| `PINCHTAB_TOKEN` | *(from config)* | Auth token for secured servers |

## Available Tools

### Navigation (4 tools)

| Tool | Description |
|------|-------------|
| `pinchtab_navigate` | Navigate to a URL |
| `pinchtab_snapshot` | Get accessibility tree snapshot |
| `pinchtab_screenshot` | Take a screenshot (base64 PNG) |
| `pinchtab_get_text` | Extract readable text content |

### Interaction (8 tools)

| Tool | Description |
|------|-------------|
| `pinchtab_click` | Click an element by ref |
| `pinchtab_type` | Type text into an input |
| `pinchtab_press` | Press a keyboard key |
| `pinchtab_hover` | Hover over an element |
| `pinchtab_focus` | Focus an element |
| `pinchtab_select` | Select a dropdown option |
| `pinchtab_scroll` | Scroll page or element |
| `pinchtab_fill` | Fill input via JS dispatch |

### Content (3 tools)

| Tool | Description |
|------|-------------|
| `pinchtab_eval` | Execute JavaScript |
| `pinchtab_pdf` | Export page as PDF |
| `pinchtab_find` | Find elements by text or CSS |

### Tab Management (4 tools)

| Tool | Description |
|------|-------------|
| `pinchtab_list_tabs` | List open browser tabs |
| `pinchtab_close_tab` | Close a tab |
| `pinchtab_health` | Check server health |
| `pinchtab_cookies` | Get page cookies |

### Utility (2 tools)

| Tool | Description |
|------|-------------|
| `pinchtab_wait` | Wait for N milliseconds |
| `pinchtab_wait_for_selector` | Wait for a CSS selector to appear |

## Tool Reference

### pinchtab_navigate

Navigate the browser to a URL.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | URL to navigate to |
| `tabId` | string | No | Target tab (uses current if empty) |

### pinchtab_snapshot

Get an accessibility tree snapshot of the page. This is the primary way agents understand page structure.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Target tab |
| `interactive` | boolean | No | Only interactive elements |
| `compact` | boolean | No | Compact format (fewer tokens) |
| `diff` | boolean | No | Only changes since last snapshot |
| `selector` | string | No | CSS selector to scope |

### pinchtab_screenshot

Capture a screenshot of the page.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Target tab |
| `quality` | number | No | JPEG quality 0-100 |

### pinchtab_get_text

Extract readable text from the page.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Target tab |
| `raw` | boolean | No | Raw text without formatting |

### pinchtab_click

Click an element identified by its ref from a snapshot.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Element ref (e.g., `e5`) |
| `tabId` | string | No | Target tab |

### pinchtab_type

Type text into an input element.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Element ref |
| `text` | string | Yes | Text to type |
| `tabId` | string | No | Target tab |

### pinchtab_press

Press a keyboard key.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | Yes | Key name (Enter, Tab, Escape, etc.) |
| `tabId` | string | No | Target tab |

### pinchtab_hover

Hover over an element.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Element ref |
| `tabId` | string | No | Target tab |

### pinchtab_focus

Focus an element.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Element ref |
| `tabId` | string | No | Target tab |

### pinchtab_select

Select a dropdown option.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Select element ref |
| `value` | string | Yes | Option value |
| `tabId` | string | No | Target tab |

### pinchtab_scroll

Scroll the page or a specific element.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | No | Element ref (omit for page scroll) |
| `pixels` | number | No | Pixels to scroll (positive=down) |
| `tabId` | string | No | Target tab |

### pinchtab_fill

Fill an input using JavaScript dispatch (works with React/Vue/Angular).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ref` | string | Yes | Element ref or CSS selector |
| `value` | string | Yes | Value to fill |
| `tabId` | string | No | Target tab |

### pinchtab_eval

Execute JavaScript in the browser. Requires `security.allowEvaluate: true` in config.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `expression` | string | Yes | JavaScript expression |
| `tabId` | string | No | Target tab |

### pinchtab_pdf

Export the page as a PDF document.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Target tab |
| `landscape` | boolean | No | Landscape orientation |
| `scale` | number | No | Print scale 0.1-2.0 |
| `pageRanges` | string | No | Pages (e.g., "1-3,5") |

### pinchtab_find

Find elements by text content or CSS selector using semantic matching.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Text or CSS selector |
| `tabId` | string | No | Target tab |

### pinchtab_list_tabs

List all open browser tabs. No parameters.

### pinchtab_close_tab

Close a browser tab.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Tab to close (current if empty) |

### pinchtab_health

Check server health. No parameters.

### pinchtab_cookies

Get cookies for the current page.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tabId` | string | No | Target tab |

### pinchtab_wait

Wait for a specified duration.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ms` | number | Yes | Milliseconds (max 30000) |

### pinchtab_wait_for_selector

Wait for a CSS selector to appear on the page. Polls every 250ms.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `selector` | string | Yes | CSS selector |
| `timeout` | number | No | Timeout ms (default 10000, max 30000) |
| `tabId` | string | No | Target tab |

## Typical Agent Workflow

```
1. pinchtab_navigate → go to URL
2. pinchtab_snapshot  → understand page structure
3. pinchtab_click     → interact with elements
4. pinchtab_type      → fill in forms
5. pinchtab_snapshot  → verify changes
6. pinchtab_get_text  → extract results
```

## Architecture

The MCP server is a thin stdio-based JSON-RPC layer that translates MCP tool calls into HTTP requests to a running PinchTab instance:

```
LLM Client ──stdio──▶ pinchtab mcp ──HTTP──▶ PinchTab Server
```

This architecture means:
- The MCP server works with any PinchTab deployment (local, remote, Docker)
- No direct Chrome dependency — the server delegates to PinchTab
- Built with [mcp-go](https://github.com/mark3labs/mcp-go) SDK (MCP spec 2025-11-25)

## Code Layout

```
internal/mcp/
├── server.go      # MCP server setup and stdio transport
├── tools.go       # Tool definitions with JSON schemas
├── handlers.go    # Tool handler implementations
└── client.go      # HTTP client for PinchTab API

cmd/pinchtab/
└── cmd_mcp.go     # `pinchtab mcp` subcommand
```
