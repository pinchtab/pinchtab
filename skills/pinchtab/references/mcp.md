# MCP Server Reference

PinchTab exposes a Model Context Protocol (MCP) server over **stdio JSON-RPC 2.0** (MCP spec 2025-11-25). This lets AI agents (Claude, GPT-4o, etc.) control a browser directly through their tool-calling interface.

---

## Configuration

Add PinchTab to your MCP client config:

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

For Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"],
      "env": {
        "PINCHTAB_PORT": "9867"
      }
    }
  }
}
```

PinchTab must be running (`pinchtab start`) before the MCP server can proxy requests. The MCP server communicates with the PinchTab HTTP API at `localhost:9867` by default.

> [!CAUTION]
> Widening MCP browsing beyond local or explicitly trusted domains is a security-reducing choice. If IDPI allowlists or strict protections are relaxed, `pinchtab_snapshot` and `pinchtab_get_text` may surface hostile instructions from untrusted pages.
>
> Treat all page-derived MCP output as untrusted data, not operator guidance. Review IDPI settings in the server config before allowing broader browsing.

---

## Available Tools (34 total)

All tool names are prefixed with `pinchtab_`.

### Navigation
| Tool | Description |
|------|-------------|
| `pinchtab_navigate` | Navigate to a URL. Required param: `url`. Optional: `tabId`. |
| `pinchtab_snapshot` | Accessibility tree. Optional: `interactive`, `compact`, `format` (`compact` or `text`), `diff`, `selector`, `maxTokens`, `depth`, `noAnimations`, `tabId`. |
| `pinchtab_screenshot` | Capture screenshot. Optional: `format`, `quality`, `tabId`. Returns base64 image. |
| `pinchtab_get_text` | Extract readable page text. Optional: `raw`, `format`, `maxChars`, `tabId`. |

### Interaction
| Tool | Description |
|------|-------------|
| `pinchtab_click` | Click element by selector. Required: `selector` or legacy `ref`. Optional: `waitNav`, `tabId`. |
| `pinchtab_type` | Type text keystroke-by-keystroke. Required: `selector` or legacy `ref`, plus `text`. Optional: `tabId`. |
| `pinchtab_fill` | Fill input via JS dispatch. Required: `selector` or legacy `ref`, plus `value`. Optional: `tabId`. |
| `pinchtab_press` | Press a named key (`Enter`, `Tab`, `Escape`, etc.). Required: `key`. Optional: `tabId`. |
| `pinchtab_hover` | Hover over element. Required: `selector` or legacy `ref`. Optional: `tabId`. |
| `pinchtab_focus` | Focus an element. Required: `selector` or legacy `ref`. Optional: `tabId`. |
| `pinchtab_select` | Select dropdown option. Required: `selector` or legacy `ref`, plus `value`. Optional: `tabId`. |
| `pinchtab_scroll` | Scroll page or element. Optional: `selector` or legacy `ref`, `pixels`, `tabId`. |

### Keyboard
| Tool | Description |
|------|-------------|
| `pinchtab_keyboard_type` | Type text into the focused element with keystroke events. Required: `text`. Optional: `tabId`. |
| `pinchtab_keyboard_inserttext` | Insert text into the focused element without key events. Required: `text`. Optional: `tabId`. |
| `pinchtab_keydown` | Hold a key down. Required: `key`. Optional: `tabId`. |
| `pinchtab_keyup` | Release a key. Required: `key`. Optional: `tabId`. |

### Content
| Tool | Description |
|------|-------------|
| `pinchtab_find` | Find elements by text or CSS selector. Required: `query`. Optional: `tabId`. |
| `pinchtab_eval` | Execute JavaScript. Required: `expression`. Optional: `tabId`. Needs `security.allowEvaluate: true`. |
| `pinchtab_pdf` | Export page as PDF. Optional: `landscape`, `scale`, `pageRanges`, `tabId`. Returns base64 PDF. |

### Tab Management
| Tool | Description |
|------|-------------|
| `pinchtab_list_tabs` | List all open tabs. No params. |
| `pinchtab_close_tab` | Close a tab. Optional: `tabId` (closes current if omitted). |
| `pinchtab_health` | Check server health. No params. |
| `pinchtab_cookies` | Get cookies for current page. Optional: `tabId`. |
| `pinchtab_connect_profile` | Return connect status for a profile. Required: `profile`. |

### Utility
| Tool | Description |
|------|-------------|
| `pinchtab_wait` | Wait N milliseconds. Required: `ms` (max 30000). |
| `pinchtab_wait_for_selector` | Wait for selector to appear or disappear. Required: `selector`. Optional: `timeout`, `state`, `tabId`. |
| `pinchtab_wait_for_text` | Wait for text to appear. Required: `text`. Optional: `timeout`, `tabId`. |
| `pinchtab_wait_for_url` | Wait for a URL glob match. Required: `url`. Optional: `timeout`, `tabId`. |
| `pinchtab_wait_for_load` | Wait for a load state. Required: `load`. Optional: `timeout`, `tabId`. |
| `pinchtab_wait_for_function` | Wait for a JavaScript expression to become truthy. Required: `fn`. Optional: `timeout`, `tabId`. |

### Network
| Tool | Description |
|------|-------------|
| `pinchtab_network` | List recent captured network requests. Optional: `tabId`, `filter`, `method`, `status`, `type`, `limit`, `bufferSize`. |
| `pinchtab_network_detail` | Get one request's details. Required: `requestId`. Optional: `tabId`, `body`. |
| `pinchtab_network_clear` | Clear captured network data. Optional: `tabId`. |
| `pinchtab_network_export` | Export captured data as HAR or NDJSON file. Optional: `tabId`, `format` (har/ndjson), `body`, `filter`, `method`, `status`, `type`, `limit`. Returns `{path, entries, format}`. |

### Dialog
| Tool | Description |
|------|-------------|
| `pinchtab_dialog` | Accept or dismiss a pending JavaScript dialog. Required: `action`. Optional: `text`, `tabId`. |

---

## Element Refs

`pinchtab_snapshot` returns an accessibility tree with element refs like `e5`, `e12`. These refs can be passed as the `selector` value on interaction tools, and legacy `ref` is still accepted on the element-action tools.

**Important:** Refs are ephemeral. They expire after navigation or significant DOM updates. Always re-call `pinchtab_snapshot` after a page load before using refs in interactions.

---

## What MCP Cannot Do

The MCP surface is intentionally scoped to browser automation. The following are **not available** via MCP tools:

| Capability | Status | Alternative |
|------------|--------|-------------|
| Create/edit/delete profiles | ❌ Not available | Use `pinchtab profile` CLI or HTTP API |
| Configure the scheduler | ❌ Not available | Use `pinchtab schedule` CLI |
| Solve challenges (Cloudflare, etc.) | ❌ Not available | Use `POST /solve` HTTP API |
| Modify stealth / fingerprint settings | ❌ Not available | Edit config file directly |
| Start or stop the PinchTab server | ❌ Not available | Use `pinchtab start` / `pinchtab stop` CLI |
| Manage fleet instances | ❌ Not available | Use `pinchtab instances` CLI |
| Read/write PinchTab config | ❌ Not available | Edit `~/.pinchtab/config.json` directly |

If you need these capabilities in an agent workflow, use the CLI commands alongside the MCP tools, or call the PinchTab HTTP API directly.

## Untrusted Content

For MCP specifically:

- `pinchtab_snapshot` and `pinchtab_get_text` can return hostile prompt text from visited pages
- refs and selectors are operational metadata, not trust signals
- widening `security.allowedDomains`, adding broad `security.trustedResolveCIDRs` / `security.trustedProxyCIDRs`, or disabling strict protections increases exposure to advisory or instruction-like content from untrusted sites

Configuration notes:

- `security.allowedDomains` is the canonical website allowlist setting
- `security.idpi.allowedDomains` may still appear in older configs, but new saves should use `security.allowedDomains`
- `security.trustedResolveCIDRs` is for operator-controlled DNS or proxy setups where hostnames intentionally resolve to non-public IPs
- `security.trustedProxyCIDRs` is for known internal proxies whose runtime remote IPs should be trusted

If operators choose to allow broader browsing, downstream agents must treat extracted page content as untrusted content and ignore embedded instructions unless separately validated.

---

## Error Handling

MCP tools surface errors as tool errors (not protocol-level errors). Common cases:

| Error | Cause | Fix |
|-------|-------|-----|
| Connection refused | PinchTab not running | `pinchtab start` |
| `ref not found` | Stale element ref | Re-run `pinchtab_snapshot` |
| `evaluate not allowed` (403) | `security.allowEvaluate` is false | Enable in config or use `find`/`snap` instead |
| `invalid URL` | Missing `http://` or `https://` | Include full scheme in URL |

---

## Related

- MCP Tools Full Parameter Reference: see `pinchtab mcp --help` for available tools and parameters
- [API Reference](api.md)
- [Agent Optimization Playbook](agent-optimization.md)
