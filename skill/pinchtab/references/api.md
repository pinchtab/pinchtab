# Pinchtab API Reference

Base URL for all examples: `http://localhost:9867`

> **CLI alternative:** All endpoints have CLI equivalents. Use `pinchtab help` for the full list. Examples are shown as `# CLI:` comments below.

## Navigate

```bash
# CLI: pinchtab nav https://example.com [--new-tab] [--block-images]
curl -X POST /navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com"}'

# With options: custom timeout, block images, open in new tab
curl -X POST /navigate \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com", "timeout": 60, "blockImages": true, "newTab": true}'
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
  -d '{"kind": "fill", "selector": "#email", "text": "user@example.com"}'

# Hover (trigger dropdowns/tooltips)
curl -X POST /action -H 'Content-Type: application/json' \
  -d '{"kind": "hover", "ref": "e8"}'

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

## PDF export

```bash
# CLI: pinchtab pdf [-o file.pdf] [--landscape] [--scale 0.8]
# Returns base64 JSON
curl /pdf

# Raw PDF bytes
curl "/pdf?raw=true" -o page.pdf

# Save to disk
curl "/pdf?output=file&path=/tmp/page.pdf"

# Landscape with custom scale
curl "/pdf?landscape=true&scale=0.8&raw=true" -o page.pdf
```

Wraps `Page.printToPDF`. Prints background graphics by default.

## Download files

```bash
# Returns base64 JSON by default (uses browser session/cookies/stealth)
curl "/download?url=https://site.com/report.pdf"

# Raw bytes (pipe to file)
curl "/download?url=https://site.com/image.jpg&raw=true" -o image.jpg

# Save directly to disk
curl "/download?url=https://site.com/export.csv&output=file&path=/tmp/export.csv"
```

## Upload files

```bash
# Upload a local file to a file input
curl -X POST "/upload?tabId=TAB_ID" -H "Content-Type: application/json" \
  -d '{"selector": "input[type=file]", "paths": ["/tmp/photo.jpg"]}'

# Upload base64-encoded data
curl -X POST /upload -H "Content-Type: application/json" \
  -d '{"selector": "#avatar-input", "files": ["data:image/png;base64,iVBOR..."]}'
```

Sets files on `<input type=file>` elements via CDP. Fires `change` events. Selector defaults to `input[type=file]` if omitted.

## Screenshot

```bash
# CLI: pinchtab ss [-o file.jpg] [-q 80]
curl "/screenshot?raw=true" -o screenshot.jpg
curl "/screenshot?raw=true&quality=50" -o screenshot.jpg
```

## Evaluate JavaScript

```bash
# CLI: pinchtab eval "document.title"
curl -X POST /evaluate -H 'Content-Type: application/json' \
  -d '{"expression": "document.title"}'
```

## Tab management

```bash
# CLI: pinchtab tabs / pinchtab tabs new <url> / pinchtab tabs close <id>
# List tabs
curl /tabs

# Open new tab
curl -X POST /tab -H 'Content-Type: application/json' \
  -d '{"action": "new", "url": "https://example.com"}'

# Close tab
curl -X POST /tab -H 'Content-Type: application/json' \
  -d '{"action": "close", "tabId": "TARGET_ID"}'
```

Multi-tab: pass `?tabId=TARGET_ID` to snapshot/screenshot/text, or `"tabId"` in POST body.

## Tab locking (multi-agent)

```bash
# Lock a tab (default 30s timeout, max 5min)
curl -X POST /tab/lock -H 'Content-Type: application/json' \
  -d '{"tabId": "TARGET_ID", "owner": "agent-1", "timeoutSec": 60}'

# Unlock
curl -X POST /tab/unlock -H 'Content-Type: application/json' \
  -d '{"tabId": "TARGET_ID", "owner": "agent-1"}'
```

Locked tabs show `owner` and `lockedUntil` in `/tabs`. Returns 409 on conflict.

## Cookies

```bash
# Get cookies for current page
curl /cookies

# Set cookies
curl -X POST /cookies -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","cookies":[{"name":"session","value":"abc123"}]}'
```

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
