# Pinchtab API Reference

Base URL for all examples: `http://localhost:9867`

> **CLI alternative:** All endpoints have CLI equivalents. Use `pinchtab help` for the full list. Examples are shown as `# CLI:` comments below.

## Agent Attribution

If an agent is calling the HTTP API directly, include `X-Agent-Id: <agent-id>` on the requests that should stay attributable to that agent.

Example:

```bash
curl -X POST /navigate \
  -H 'X-Agent-Id: agent-crawl-01' \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://pinchtab.com"}'
```

Notes:

- CLI users should prefer `pinchtab --agent-id <agent-id> ...` instead of setting the header manually
- scheduler-submitted tasks reuse their `agentId` as `X-Agent-Id` when the task is executed

## Navigate

```bash
# CLI: pinchtab nav https://pinchtab.com [--new-tab] [--block-images]
curl -X POST /navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://pinchtab.com"}'

# With options: custom timeout, block images, open in new tab
curl -X POST /navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://pinchtab.com", "timeout": 60, "blockImages": true, "newTab": true}'
```

## Snapshot (accessibility tree)

```bash
# CLI: pinchtab snap [-i] [-c] [-d] [-s main] [--max-tokens 2000]
# Full tree
curl /snapshot

# Interactive elements only (buttons, links, inputs) — much smaller
curl "/snapshot?filter=interactive"

# Limit depth
curl "/snapshot?depth=5"

# Smart diff — only changes since last snapshot (massive token savings)
curl "/snapshot?diff=true"

# Text format — indented tree, ~40-60% fewer tokens than JSON
curl "/snapshot?format=text"

# Compact format — one-line-per-node, 56-64% fewer tokens than JSON (recommended)
curl "/snapshot?format=compact"

# YAML format
curl "/snapshot?format=yaml"

# Scope to CSS selector (e.g. main content only)
curl "/snapshot?selector=main"

# Truncate to ~N tokens
curl "/snapshot?maxTokens=2000"

# Combine for maximum efficiency
curl "/snapshot?format=compact&selector=main&maxTokens=2000&filter=interactive"

# Disable animations before capture
curl "/snapshot?noAnimations=true"

# Write to file
curl "/snapshot?output=file&path=/tmp/snapshot.json"
```

Returns flat JSON array of nodes with `ref`, `role`, `name`, `depth`, `value`, `nodeId`.

**Token optimization**: Use `?format=compact` for best token efficiency. Add `?filter=interactive` for action-oriented tasks (~75% fewer nodes). Use `?selector=main` to scope to relevant content. Use `?maxTokens=2000` to cap output. Use `?diff=true` on multi-step workflows to see only changes. Combine all params freely.

## Act on elements

```bash
# CLI: pinchtab click e5 / pinchtab type e12 hello / pinchtab press Enter
# Click by ref
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "click", "ref": "e5"}'

# Type into focused element (click first, then type)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "click", "ref": "e12"}'
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "type", "ref": "e12", "text": "hello world"}'

# Press a key
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "press", "key": "Enter"}'

# Focus an element
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "focus", "ref": "e3"}'

# Fill (set value directly, no keystrokes)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "fill", "selector": "#email", "text": "user@pinchtab.com"}'

# Hover (trigger dropdowns/tooltips)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "hover", "ref": "e8"}'

# Move pointer without clicking
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "mouse-move", "ref": "e8"}'

# Press and release a mouse button
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "mouse-down", "button": "left"}'
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "mouse-up", "button": "left"}'

# Wheel at an element or coordinates
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "mouse-wheel", "ref": "e8", "deltaY": 240}'
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "mouse-wheel", "x": 400, "y": 320, "deltaY": -320}'

# Select dropdown option (by value or visible text)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "select", "ref": "e10", "value": "option2"}'

# Scroll to element
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "scroll", "ref": "e20"}'

# Scroll by pixels (infinite scroll pages)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "scroll", "scrollY": 800}'

# Click and wait for navigation (link clicks)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "click", "ref": "e5", "waitNav": true}'
```

Notes:

- selector-based click and double-click paths resolve through backend node IDs before dispatching pointer events
- low-level pointer actions accept `ref`, `selector`, `nodeId`, or `x`/`y`
- `mouse-down` and `mouse-up` accept `button` with `left`, `right`, or `middle`
- `mouse-wheel` accepts `deltaX` and `deltaY`; when omitted, legacy `scrollX` / `scrollY` still work
- `mouse-down`, `mouse-up`, and `mouse-wheel` use the current pointer position when you do not pass a fresh target

## Wait for page state

```bash
# CLI: pinchtab wait 'text:Done' / pinchtab wait --url '**/dashboard'
curl -X POST /wait -H 'Content-Type: application/json' \
  -d '{"selector":"text:Done","timeout":15000}'

curl -X POST /wait -H 'Content-Type: application/json' \
  -d '{"url":"**/dashboard","timeout":15000}'

curl -X POST /wait -H 'Content-Type: application/json' \
  -d '{"load":"networkidle","timeout":15000}'

curl -X POST /wait -H 'Content-Type: application/json' \
  -d '{"fn":"document.readyState === \"complete\"","timeout":15000}'
```

## Batch actions

```bash
# Execute multiple actions in sequence
curl -X POST /actions -H 'Content-Type: application/json' \
  -d '{"actions":[{"kind":"click","ref":"e3"},{"kind":"type","ref":"e3","text":"hello"},{"kind":"press","key":"Enter"}]}'

# Stop on first error (default: false)
curl -X POST /actions -H 'Content-Type: application/json' \
  -d '{"tabId":"TARGET_ID","actions":[...],"stopOnError":true}'
```

## Extract text

```bash
# CLI: pinchtab text [--raw]
# Readability mode (default) — strips nav/footer/ads
curl /text

# Raw innerText
curl "/text?mode=raw"
```

Returns `{url, title, text}`. Cheapest option (~1K tokens for most pages).

Default mode picks the first **visible** `<article>` / `[role="main"]` / `<main>` (skips `display:none`) and strips nav/footer/ads. Use `mode=raw` for full `innerText`, or `/snapshot` for structured UI text like prices and button labels.

## PDF export

Prefer returning base64 or raw bytes unless the user explicitly wants a file written to disk.
When writing to disk, use a safe temporary or workspace path.

```bash
# CLI: pinchtab pdf --tab TAB_ID [-o file.pdf] [--landscape] [--scale 0.8]
# Returns base64 JSON
curl "/tabs/TAB_ID/pdf"

# Raw PDF bytes
curl "/tabs/TAB_ID/pdf?raw=true" -o page.pdf

# Save to disk in a safe temp location
curl "/tabs/TAB_ID/pdf?output=file&path=/tmp/pinchtab-page.pdf"

# Landscape with custom scale
curl "/tabs/TAB_ID/pdf?landscape=true&scale=0.8&raw=true" -o page.pdf

# Custom paper size (Letter: 8.5x11, A4: 8.27x11.69)
curl "/tabs/TAB_ID/pdf?paperWidth=8.5&paperHeight=11&marginTop=0.5&marginLeft=0.5&raw=true" -o custom.pdf

# Export specific pages
curl "/tabs/TAB_ID/pdf?pageRanges=1-5&raw=true" -o pages.pdf

# With header/footer
curl "/tabs/TAB_ID/pdf?displayHeaderFooter=true&headerTemplate=%3Cspan%20class=title%3E%3C/span%3E&raw=true" -o header.pdf

# Accessible PDF with document outline
curl "/tabs/TAB_ID/pdf?generateTaggedPDF=true&generateDocumentOutline=true&raw=true" -o accessible.pdf

# Honor CSS page size
curl "/tabs/TAB_ID/pdf?preferCSSPageSize=true&raw=true" -o css-sized.pdf
```

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `paperWidth` | float | 8.5 | Paper width in inches |
| `paperHeight` | float | 11.0 | Paper height in inches |
| `landscape` | bool | false | Landscape orientation |
| `marginTop` | float | 0.4 | Top margin in inches |
| `marginBottom` | float | 0.4 | Bottom margin in inches |
| `marginLeft` | float | 0.4 | Left margin in inches |
| `marginRight` | float | 0.4 | Right margin in inches |
| `scale` | float | 1.0 | Print scale (0.1–2.0) |
| `pageRanges` | string | all | Pages to export (e.g., `1-3,5`) |
| `displayHeaderFooter` | bool | false | Show header and footer |
| `headerTemplate` | string | — | HTML template for header |
| `footerTemplate` | string | — | HTML template for footer |
| `preferCSSPageSize` | bool | false | Honor CSS `@page` size |
| `generateTaggedPDF` | bool | false | Generate accessible/tagged PDF |
| `generateDocumentOutline` | bool | false | Embed document outline |
| `output` | string | JSON | `file` to save to disk, default returns base64 |
| `path` | string | auto | Destination path (prefer temp or workspace paths with `output=file`) |
| `raw` | bool | false | Return raw PDF bytes instead of JSON |

Wraps `Page.printToPDF`. Prints background graphics by default.

## Download files

Prefer raw bytes or base64 responses unless the user explicitly asks for a saved file.

```bash
# Returns base64 JSON by default (uses browser session/cookies/stealth)
curl "/download?url=https://site.com/report.pdf"

# Raw bytes (pipe to file)
curl "/download?url=https://site.com/image.jpg&raw=true" -o image.jpg

# Save directly to disk in a safe temp location
curl "/download?url=https://site.com/export.csv&output=file&path=/tmp/pinchtab-export.csv"
```

## Upload files

Only upload local files the user explicitly provided or approved for the task.

```bash
# Upload a local file to a file input
curl -X POST "/upload?tabId=TAB_ID" -H "Content-Type: application/json" \
  -d '{"selector": "input[type=file]", "paths": ["/tmp/user-approved-photo.jpg"]}'

# Upload base64-encoded data
curl -X POST /upload -H "Content-Type: application/json" \
  -d '{"selector": "#avatar-input", "files": ["data:image/png;base64,iVBOR..."]}'
```

Sets files on `<input type=file>` elements via CDP. Fires `change` events. Selector defaults to `input[type=file]` if omitted.

## Screenshot

```bash
# CLI: pinchtab ss [-o file.jpg] [-q 80]
# Returns raw JPEG (default)
curl "/screenshot?raw=true" -o screenshot.jpg
curl "/screenshot?raw=true&quality=50" -o screenshot.jpg

# Returns raw PNG
curl "/screenshot?raw=true&format=png" -o screenshot.png
```

## Evaluate JavaScript

Use this sparingly. Prefer `text`, `snapshot`, and normal actions first.
Default to read-only DOM inspection and avoid reading cookies, localStorage, or unrelated page secrets unless the user explicitly asks for that behavior.

```bash
# CLI: pinchtab eval "document.title"
curl -X POST /evaluate -H 'Content-Type: application/json' \
  -d '{"expression": "document.title"}'

# Resolve a returned promise before responding
curl -X POST /evaluate -H 'Content-Type: application/json' \
  -d '{"expression": "Promise.resolve(document.title)", "awaitPromise": true}'
```

Set `awaitPromise: true` when the expression returns a promise and you want the resolved value. If omitted, behavior stays unchanged.

## Tab management

```bash
# CLI: pinchtab tabs / pinchtab tabs new <url> / pinchtab tabs close <id>
# List tabs
curl /tabs

# Open new tab
curl -X POST /tab -H 'Content-Type: application/json' \
  -d '{"action": "new", "url": "https://pinchtab.com"}'

# Close tab
curl -X POST /tab -H 'Content-Type: application/json' \
  -d '{"action": "close", "tabId": "TARGET_ID"}'
```

Multi-tab: pass `?tabId=TARGET_ID` to snapshot/screenshot/text, or `"tabId"` in POST body.

## Tab-specific endpoints

All read/action endpoints have tab-scoped variants using `/tabs/{id}/...`:

```bash
# Navigate a specific tab
curl -X POST /tabs/TARGET_ID/navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://pinchtab.com"}'

# Snapshot a specific tab
curl "/tabs/TARGET_ID/snapshot"
curl "/tabs/TARGET_ID/snapshot?filter=interactive&format=compact"

# Screenshot a specific tab
curl "/tabs/TARGET_ID/screenshot?raw=true" -o tab-screenshot.jpg

# Extract text from a specific tab
curl "/tabs/TARGET_ID/text"

# Action on a specific tab
curl -X POST /tabs/TARGET_ID/action \
  -H 'Content-Type: application/json' \
  -d '{"kind": "click", "ref": "e5"}'

# Batch actions on a specific tab
curl -X POST /tabs/TARGET_ID/actions \
  -H 'Content-Type: application/json' \
  -d '{"actions": [{"kind": "click", "ref": "e3"}, {"kind": "type", "ref": "e3", "text": "hello"}]}'

# Wait on a specific tab
curl -X POST /tabs/TARGET_ID/wait \
  -H 'Content-Type: application/json' \
  -d '{"selector":"text:Done","timeout":15000}'

# Pause automation for manual intervention
curl -X POST /tabs/TARGET_ID/handoff \
  -H 'Content-Type: application/json' \
  -d '{"reason":"captcha","timeoutMs":120000}'

# Inspect or resume handoff state
curl /tabs/TARGET_ID/handoff
curl -X POST /tabs/TARGET_ID/resume \
  -H 'Content-Type: application/json' \
  -d '{"status":"completed"}'
```

These are equivalent to using `?tabId=TARGET_ID` on top-level endpoints but follow REST conventions. The tab ID comes from `/tabs` or from the `tabId` field in navigate/tab creation responses.

`GET /tabs/{id}/handoff` returns the current status for that tab. `POST /tabs/{id}/resume` clears `paused_handoff` and can carry resume metadata such as `status` or `resolvedData`.

## Tab locking (multi-agent)

```bash
# Lock a tab (default 30s timeout, max 5min)
curl -X POST /lock -H 'Content-Type: application/json' \
  -d '{"tabId": "TARGET_ID", "owner": "agent-1", "timeoutSec": 60}'

# Unlock
curl -X POST /unlock -H 'Content-Type: application/json' \
  -d '{"tabId": "TARGET_ID", "owner": "agent-1"}'
```

Locked tabs show `owner` and `lockedUntil` in `/tabs`. Returns 409 on conflict.

## Cookies

```bash
# Get cookies for current page
curl /cookies

# Set cookies
curl -X POST /cookies -H 'Content-Type: application/json' \
  -d '{"url":"https://pinchtab.com","cookies":[{"name":"session","value":"abc123"}]}'
```

## Solve challenges

PinchTab includes a pluggable solver framework for browser challenges (Cloudflare Turnstile, CAPTCHAs, interstitials). Solvers auto-detect the challenge type and resolve it using human-like interaction.

```bash
# List available solvers
curl /solvers

# Auto-detect and solve (tries each solver in order)
curl -X POST /solve -H 'Content-Type: application/json' \
  -d '{"maxAttempts": 3, "timeout": 30000}'

# Use a specific solver by name
curl -X POST /solve/cloudflare -H 'Content-Type: application/json' \
  -d '{"maxAttempts": 3}'

# Solve on a specific tab
curl -X POST /tabs/TAB_ID/solve -H 'Content-Type: application/json' \
  -d '{"solver": "cloudflare"}'

# Solve on a specific tab with path-based solver
curl -X POST /tabs/TAB_ID/solve/cloudflare -H 'Content-Type: application/json' \
  -d '{}'
```

**Request fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `solver` | string | — | Solver name (omit for auto-detect) |
| `tabId` | string | — | Target tab (omit for default tab) |
| `maxAttempts` | int | 3 | Maximum solve attempts |
| `timeout` | float | 30000 | Overall timeout in ms |

**Response:**

```json
{
  "tabId": "DEADBEEF",
  "solver": "cloudflare",
  "solved": true,
  "challengeType": "managed",
  "attempts": 1,
  "title": "Example Site"
}
```

Returns `solved: true, attempts: 0` when no challenge is detected — safe to call speculatively.

**Built-in solvers:** `cloudflare` (Turnstile/interstitial — detects via page title, clicks checkbox with human-like input).

**Stealth requirement:** Solvers work best with `stealthLevel: "full"`. Cloudflare checks browser fingerprints before and after the checkbox click. Verify stealth is active with `GET /stealth/status`.

## Network Export

```bash
# Export as HAR 1.2 (stream to response)
curl /network/export?format=har

# Export as NDJSON (one JSON per line)
curl /network/export?format=ndjson

# Save to server-side file
curl "/network/export?format=har&output=file&path=session.har"

# Include response bodies (10 MB cap per entry)
curl "/network/export?format=har&body=true"

# Include raw sensitive headers (Cookie, Authorization)
curl "/network/export?format=har&redact=false"

# Live streaming export (entries written to file as they arrive)
curl -N "/network/export/stream?format=ndjson&path=live.ndjson"

# Tab-scoped
curl /tabs/TAB_ID/network/export?format=har
```

All standard network filters apply: `filter`, `method`, `status`, `type`, `limit`.

Formats are pluggable. `GET /network/export?format=unknown` returns `{"available": ["har", "ndjson"]}`.

## Stealth

```bash
# Check stealth status and score
curl /stealth/status

# Rotate browser fingerprint
curl -X POST /fingerprint/rotate -H 'Content-Type: application/json' \
  -d '{"os":"windows"}'
# os: "windows", "mac", or omit for random
```

## Health check

```bash
curl /health
```

## Session Auth

If the user already gives you an agent session token, send it as:

```bash
curl -H "Authorization: Session ses_..." /health
```
