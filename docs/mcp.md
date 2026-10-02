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

PinchTab currently exposes 47 tools:

- Navigation: 9
- Interaction: 8
- Keyboard: 1
- Content: 4
- Recording: 1
- Site: 1
- Tab management: 6
- Human handoff: 3
- Wait utilities: 1
- Network: 6
- Diagnostics: 6
- Dialog: 1

### Navigation

- `pinchtab_navigate`
- `pinchtab_back` — go back one history entry; returns the tab ID, the URL landed on and `navigated`, which is `false` when the tab was at the edge of history and did not move
- `pinchtab_forward` — go forward one history entry; same response as `pinchtab_back`
- `pinchtab_reload` — reload the current page; returns the tab ID and the URL landed on, without `navigated`
- `pinchtab_snapshot`
- `pinchtab_frame`
- `pinchtab_screenshot`
- `pinchtab_capture` — paired screenshot + snapshot from one DOM epoch
- `pinchtab_get_text` — extract page text; `mode` selects `readability` (default), `raw`, or `markdown` (preserves links and tables, best for article-shaped pages). `mode` supersedes the legacy boolean `raw`. Example: `pinchtab_get_text {"mode":"markdown"}`

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
- `pinchtab_extract` — typed values against a JSON `schema` (`tabId`, `scope`, `maxItems` optional); every field carries a ref for the action tools and a confidence. Prefer it over snapshot-then-parse for structured values, and pin a `low` field with `x-pinchtab-hint`. See [Extract](reference/extract.md)

### Recording

- `pinchtab_record` — `action` is `start`, `stop` or `status`

### Site

- `pinchtab_scrape` — crawl a site to markdown (HTTP-first, browser-enrich thin/JS pages). Use `preview=true` for a cheap outline, then expand chosen URLs with `only`. Its `timeoutSeconds` is a crawl budget and the one seconds-unit argument; every wait elsewhere is `timeoutMs`.

### Tab Management

- `pinchtab_list_tabs`
- `pinchtab_close_tab`
- `pinchtab_health`
- `pinchtab_cookies` (requires `security.allowCookies`)
- `pinchtab_cookies_set` (requires `security.allowCookies`)
- `pinchtab_connect_profile`

### Human Handoff

- `pinchtab_handoff` — pause a tab (`tabId` required; `reason`, `timeoutMs` optional) so a human can finish a CAPTCHA, login or consent step; every action tool on that tab is refused with `tab_paused_handoff` until it resumes
- `pinchtab_resume` — resume the tab (`tabId` required; `status` optional). Call it only after the user confirms they finished
- `pinchtab_handoff_status` — report `paused_handoff` (with `reason`, `pausedAt`, `expiresAt`) or `active`, so an agent can watch a `timeoutMs` auto-resume without resuming itself

### Wait Utilities

- `pinchtab_wait` — `for` is `ms`, `selector`, `text`, `url`, `load` or `function`, and `value` carries the condition; `timeoutMs` bounds a browser-backed wait (`timeout` is its deprecated alias)

### Network

- `pinchtab_network`
- `pinchtab_network_detail`
- `pinchtab_network_clear`
- `pinchtab_network_route`
- `pinchtab_network_unroute`
- `pinchtab_network_rules`

### Diagnostics

- `pinchtab_console` — read (or `clear`) the tab's browser console logs
- `pinchtab_errors` — read (or `clear`) the tab's uncaught JavaScript exceptions; check this when a snapshot looks healthy but actions do nothing
- `pinchtab_a11y_audit` — accessibility audit; `engine=axe` runs vendored axe-core in the isolated world and returns ref-mapped violations with WCAG tags and help URLs
- `pinchtab_memory` — JavaScript heap usage and DOM counters for the tab; `gc=true` collects garbage first so two reads compare retained memory
- `pinchtab_memory_snapshot` — take a V8 heap snapshot to a server-side file and return only its path and a summary (top constructors, duplicate strings); needs `security.allowMemory`. See [Memory](reference/memory.md)
- `pinchtab_memory_compare` — compare two snapshot ids: per-constructor count and self-size deltas, largest first, plus new duplicate strings; `retained=true` adds retained sizes from the head snapshot's dominator tree; needs `security.allowMemory`

### Dialog

- `pinchtab_dialog`

## Selector Model

For selector-based interaction tools, prefer `selector`. `ref`, `element` and `target` are still accepted as deprecated aliases on the element-action tools.

## Argument Names

Every tool refuses an argument its schema does not declare. The call returns an error naming the unknown key, the nearest declared name when one is close (`filter: "interactive"` on `pinchtab_snapshot` points at `interactive: true`), and the tool's declared arguments; nothing runs. Deprecated spellings stay declared, with a description naming the canonical one: `ref`/`element`/`target` for `selector`, `timeout` for `timeoutMs` on `pinchtab_wait`, `onDialog`/`promptText` for `dialogAction`/`dialogText` on `pinchtab_click`, `value` for `text` on `pinchtab_type`, `option` for `value` on `pinchtab_select`, and `text` for `value` on `pinchtab_fill`.

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
4. Call `pinchtab_wait` or `pinchtab_network` when needed
5. Call `pinchtab_back` to leave a dead end, or `pinchtab_reload` to retry the page

`pinchtab_back`, `pinchtab_forward` and `pinchtab_reload` take an optional `tabId`
and `snap`, so `snap: true` returns the page after the navigation in one
round-trip.

`pinchtab_snapshot` supports MCP-safe output controls:

- `compact=true` or `format="compact"` for the most token-efficient text snapshot
- `format="text"` for the full text snapshot
- `noAnimations=true` to reduce animation noise before capture

For full parameter details, see [MCP Tool Reference](./reference/mcp-tools.md).
