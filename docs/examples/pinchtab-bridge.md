# Example: Bridge Smoke Test

This example is a happy-path smoke test for a running `pinchtab bridge` instance on `127.0.0.1:9867`.

It is useful when you want to verify that the single-instance runtime can:
- respond to health checks
- open and list tabs
- navigate and inspect pages
- perform actions
- capture screenshots and PDFs
- read cookies
- use tab locking

For the bridge-mode mental model, see [Expert Guide: Bridge Mode](../guides/expert-bridge-mode.md).

## Prerequisites

Start the bridge:

```bash
pinchtab bridge
```

Set the base URL:

```bash
BASE=http://127.0.0.1:9867
```

The commands below assume `jq` is installed.

## 1. Check Health

```bash
curl -s "$BASE/health" | jq .
```

```bash
# CLI alternative
pinchtab health
```

```jsonc
// Response
{
  "status": "ok",
  "tabs": 1
}
```

## 2. Navigate To A Page

```bash
curl -s -X POST "$BASE/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com/pinchtab/pinchtab"}' | jq .
```

```bash
# CLI alternative
pinchtab nav https://github.com/pinchtab/pinchtab
```

```jsonc
// Response
{
  "tabId": "BD78E40ED7400A4B0E73B99415E1B9EA",
  "title": "GitHub - pinchtab/pinchtab",
  "url": "https://github.com/pinchtab/pinchtab"
}
```

## 3. List Tabs

```bash
curl -s "$BASE/tabs" | jq .
```

```bash
# CLI alternative
pinchtab tabs
```

```jsonc
// Response
{
  "tabs": [
    {
      "id": "BD78E40ED7400A4B0E73B99415E1B9EA",
      "title": "GitHub - pinchtab/pinchtab",
      "type": "page",
      "url": "https://github.com/pinchtab/pinchtab"
    }
  ]
}
```

## 4. Capture An Interactive Snapshot

```bash
curl -s "$BASE/snapshot?filter=interactive" | jq .
```

```bash
# CLI alternative (compact format)
pinchtab snap -i -c
```

```jsonc
// Response (JSON)
{
  "nodes": [
    { "ref": "e0", "role": "link", "name": "Skip to content" },
    { "ref": "e1", "role": "link", "name": "GitHub Homepage" },
    { "ref": "e14", "role": "button", "name": "Search or jump to…" }
  ]
}
```

```bash
# CLI compact output
# GitHub - pinchtab/pinchtab | https://github.com/pinchtab/pinchtab | 219 nodes
e0:link "Skip to content"
e1:link "GitHub Homepage"
e14:button "Search or jump to…"
...
```

## 5. Extract Page Text

```bash
curl -s "$BASE/text" | jq .
```

```bash
# CLI alternative
pinchtab text
```

```jsonc
// Response
{
  "text": "High-performance browser automation bridge and multi-instance orchestrator...",
  "title": "GitHub - pinchtab/pinchtab",
  "url": "https://github.com/pinchtab/pinchtab"
}
```

## 6. Click An Element

First get the snapshot to cache refs:

```bash
curl -s "$BASE/snapshot?filter=interactive" > /dev/null
```

Then click:

```bash
curl -s -X POST "$BASE/action" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e14"}' | jq .
```

```bash
# CLI alternative (run snap first to cache refs)
pinchtab snap -i > /dev/null
pinchtab click e14
```

```jsonc
// Response
{
  "success": true,
  "result": {
    "clicked": true
  }
}
```

## 7. Type Into A Search Box

After clicking the search button (e14), a search input appears:

```bash
curl -s "$BASE/snapshot?filter=interactive" | jq '.. | objects | select(.role == "combobox")' | head -10
```

```bash
# CLI alternative
pinchtab snap -i -c | grep combobox
# Output: e221:combobox "Search" val="repo:pinchtab/pinchtab"
```

Type into it:

```bash
curl -s -X POST "$BASE/action" \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e221","text":"browser automation"}' | jq .
```

```bash
# CLI alternative
pinchtab type e221 "browser automation"
```

```jsonc
// Response
{
  "success": true,
  "result": {
    "typed": "browser automation"
  }
}
```

## 8. Press A Key

```bash
curl -s -X POST "$BASE/action" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Escape"}' | jq .
```

```bash
# CLI alternative
pinchtab press Escape
```

```jsonc
// Response
{
  "success": true,
  "result": {
    "pressed": "Escape"
  }
}
```

## 9. Take A Screenshot

```bash
curl -s "$BASE/screenshot" > smoke.jpg
ls -lh smoke.jpg
```

```bash
# CLI alternative
pinchtab ss -o smoke.jpg
```

```bash
# Output
Saved smoke.jpg (55876 bytes)
-rw-------  1 user  staff  55K Mar  7 18:20 smoke.jpg
```

## 10. Export A PDF

```bash
curl -s "$BASE/pdf" > smoke.pdf
ls -lh smoke.pdf
```

```bash
# CLI alternative
pinchtab pdf -o smoke.pdf
```

```jsonc
// Output
Saved smoke.pdf (1494657 bytes)
-rw-------  1 user  staff  1.4M Mar  7 18:51 smoke.pdf
```

## 11. Read Cookies

```bash
TAB=$(curl -s "$BASE/tabs" | jq -r '.tabs[0].id')
curl -s "$BASE/tabs/$TAB/cookies" | jq '.cookies[:2]'
```

```jsonc
// Response
[
  {
    "domain": ".github.com",
    "name": "_gh_sess",
    "path": "/",
    "secure": true,
    "httpOnly": true
  }
]
```

> **Note:** No direct CLI alternative for cookies. Use curl or the API.

## 12. Lock And Unlock The Tab

```bash
TAB=$(curl -s "$BASE/tabs" | jq -r '.tabs[0].id')

curl -s -X POST "$BASE/tabs/$TAB/lock" \
  -H "Content-Type: application/json" \
  -d '{"owner":"smoke-test","ttl":60}' | jq .
```

```jsonc
// Response
{
  "locked": true,
  "owner": "smoke-test",
  "expiresAt": "2026-03-07T18:30:43+01:00"
}
```

```bash
curl -s -X POST "$BASE/tabs/$TAB/unlock" \
  -H "Content-Type: application/json" \
  -d '{"owner":"smoke-test"}' | jq .
```

```jsonc
// Response
{
  "unlocked": true
}
```

> **Note:** No direct CLI alternative for lock/unlock. Use curl or the API.

## 13. Open A Second Tab

```bash
curl -s -X POST "$BASE/tab" \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://pinchtab.com"}' | jq .
```

```bash
# CLI alternative
pinchtab nav https://pinchtab.com
```

```jsonc
// Response
{
  "tabId": "F266512A8B4FF5E537ADECDCD898BA02",
  "title": "PinchTab — Browser Control for AI Agents",
  "url": "https://pinchtab.com/"
}
```

## CLI Quick Reference

| Action | CLI Command |
|--------|-------------|
| Health check | `pinchtab health` |
| Navigate | `pinchtab nav <url>` |
| List tabs | `pinchtab tabs` |
| Snapshot | `pinchtab snap -i -c` |
| Extract text | `pinchtab text` |
| Click | `pinchtab click <ref>` |
| Type | `pinchtab type <ref> <text>` |
| Press key | `pinchtab press Enter` |
| Screenshot | `pinchtab ss -o file.jpg` |
| PDF export | `pinchtab pdf -o file.pdf` |

> **Tip:** Run `pinchtab snap` before `click` or `type` to cache element refs.

## What This Example Proves

If the full sequence works, your bridge runtime can:
- start and respond to API requests
- manage multiple tabs
- inspect page structure and text
- execute actions (click, type, press)
- render visual outputs (screenshots, PDFs)
- support locking and basic browser state APIs

If this example fails, check:
- the bridge is running on `127.0.0.1:9867`
- Chrome can be started successfully
