# MCP Tool Reference

PinchTab currently exposes 34 MCP tools. All tool names are prefixed with `pinchtab_` and are served over stdio JSON-RPC.

For selector-based interaction tools, prefer `selector`. `ref` is still accepted as a deprecated fallback on the element-action tools.

Selector forms include:

- `e5`
- `#login`
- `xpath://button`
- `text:Submit`
- `find:login button`

## Navigation

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_navigate` | `url` required, `tabId` optional | Uses `/navigate`; omitting `tabId` opens a new tab |
| `pinchtab_snapshot` | `tabId`, `interactive`, `compact`, `diff`, `selector` | `selector` scopes the snapshot |
| `pinchtab_screenshot` | `tabId`, `format`, `quality` | `format` is `jpeg` or `png` |
| `pinchtab_get_text` | `tabId`, `raw` | `raw=true` maps to `/text?mode=raw` |

## Interaction

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_click` | `selector` required, `tabId`, `ref` | Click element by selector |
| `pinchtab_type` | `selector` required, `text` required, `tabId`, `ref` | Sends key events |
| `pinchtab_press` | `key` required, `tabId` | Press a key such as `Enter` |
| `pinchtab_hover` | `selector` required, `tabId`, `ref` | Hover element |
| `pinchtab_focus` | `selector` required, `tabId`, `ref` | Focus element |
| `pinchtab_select` | `selector` required, `value` required, `tabId`, `ref` | Select `<option>` by value |
| `pinchtab_scroll` | `selector`, `pixels`, `tabId`, `ref` | Omit `selector` to scroll the page |
| `pinchtab_fill` | `selector` required, `value` required, `tabId`, `ref` | Direct fill instead of keystrokes |

## Keyboard

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_keyboard_type` | `text` required, `tabId` | Types at the currently focused element |
| `pinchtab_keyboard_inserttext` | `text` required, `tabId` | Paste-like insert without key events |
| `pinchtab_keydown` | `key` required, `tabId` | Hold a key down |
| `pinchtab_keyup` | `key` required, `tabId` | Release a key |

## Content

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_eval` | `expression` required, `tabId` | Requires `security.allowEvaluate` |
| `pinchtab_pdf` | `tabId`, `landscape`, `scale`, `pageRanges` | Returns base64-encoded PDF content |
| `pinchtab_find` | `query` required, `tabId` | Semantic element search |

## Tab Management

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_list_tabs` | none | Lists open tabs |
| `pinchtab_close_tab` | `tabId` | Closes the given tab |
| `pinchtab_health` | none | Checks server health |
| `pinchtab_cookies` | `tabId` | Reads cookies for a tab |
| `pinchtab_connect_profile` | `profile` required | Returns the connect URL and instance status for a profile |

## Wait Utilities

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_wait` | `ms` required | Fixed-duration wait, capped at 30000 ms |
| `pinchtab_wait_for_selector` | `selector` required, `timeout`, `state`, `tabId` | `state` is `visible` or `hidden` |
| `pinchtab_wait_for_text` | `text` required, `timeout`, `tabId` | Wait for body text |
| `pinchtab_wait_for_url` | `url` required, `timeout`, `tabId` | URL glob match |
| `pinchtab_wait_for_load` | `load` required, `timeout`, `tabId` | Currently supports `networkidle` |
| `pinchtab_wait_for_function` | `fn` required, `timeout`, `tabId` | JS expression must become truthy |

## Network

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_network` | `tabId`, `filter`, `method`, `status`, `type`, `limit`, `bufferSize` | Lists recent network requests |
| `pinchtab_network_detail` | `requestId` required, `tabId`, `body` | `body=true` includes response body when available |
| `pinchtab_network_clear` | `tabId` | Clears one tab or all tabs when omitted |

## Dialog

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_dialog` | `action` required, `text`, `tabId` | `action` is `accept` or `dismiss` |

## Return Shapes

Typical results:

- navigation tools return JSON from the matching HTTP endpoint
- `pinchtab_snapshot` returns snapshot text
- `pinchtab_get_text` returns page text
- `pinchtab_screenshot` and `pinchtab_pdf` return JSON containing base64 payloads
- wait tools return wait status JSON
- network tools return the same request logs you would see from `/network`

For setup and client configuration, see [MCP Server](../mcp.md).
