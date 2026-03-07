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

You should get a JSON health response from the bridge runtime.

## 2. Open A Tab

```bash
TAB=$(curl -s -X POST "$BASE/tab" \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://www.wikipedia.org"}' \
  | jq -r '.tabId // .id')

echo "$TAB"
```

This opens a new tab and stores the tab ID in `TAB`.

## 3. List Tabs

```bash
curl -s "$BASE/tabs" | jq .
```

This confirms the bridge sees the tab you just opened.

## 4. Capture An Interactive Snapshot

```bash
curl -s "$BASE/tabs/$TAB/snapshot?filter=interactive" | jq .
```

This returns the accessibility-style tree used for refs and actions.

## 5. Extract Page Text

```bash
curl -s "$BASE/tabs/$TAB/text" | jq .
```

This verifies text extraction works for the current tab.

## 6. Find A Search Input Ref

```bash
SEARCH_REF=$(curl -s "$BASE/tabs/$TAB/snapshot?filter=interactive" \
  | jq -r '.. | objects | select(.role? == "textbox" or .role? == "searchbox" or .name? == "Search Wikipedia") | .ref' \
  | head -1)

echo "$SEARCH_REF"
```

This extracts one interactive ref from the snapshot.

## 7. Fill And Press Enter

```bash
curl -s -X POST "$BASE/tabs/$TAB/action" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"fill\",\"ref\":\"$SEARCH_REF\",\"text\":\"Browser automation\"}" | jq .

curl -s -X POST "$BASE/tabs/$TAB/action" \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}' | jq .
```

This verifies action execution and should trigger navigation.

## 8. Snapshot Again After Navigation

```bash
curl -s "$BASE/tabs/$TAB/snapshot?filter=interactive" | jq .
```

This confirms the bridge remains usable after a page transition.

## 9. Take A Screenshot

```bash
curl -s "$BASE/tabs/$TAB/screenshot" > smoke.jpg
ls -lh smoke.jpg
```

This verifies screenshot generation.

## 10. Export A PDF

```bash
curl -s "$BASE/tabs/$TAB/pdf" > smoke.pdf
ls -lh smoke.pdf
```

This verifies PDF export.

## 11. Read Cookies

```bash
curl -s "$BASE/tabs/$TAB/cookies" | jq .
```

This checks cookie retrieval for the active page.

## 12. Lock And Unlock The Tab

```bash
curl -s -X POST "$BASE/tabs/$TAB/lock" \
  -H "Content-Type: application/json" \
  -d '{"owner":"smoke-test","ttl":60}' | jq .

curl -s -X POST "$BASE/tabs/$TAB/unlock" \
  -H "Content-Type: application/json" \
  -d '{"owner":"smoke-test"}' | jq .
```

This verifies tab ownership locking works.

## 13. Open A Second Tab

```bash
TAB2=$(curl -s -X POST "$BASE/tab" \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}' \
  | jq -r '.tabId // .id')

echo "$TAB2"
curl -s "$BASE/tabs" | jq .
```

This confirms the bridge handles multiple tabs in one runtime.

## Optional: Evaluate JavaScript

This only works if `evaluate` is enabled in config.

```bash
curl -s -X POST "$BASE/tabs/$TAB/evaluate" \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}' | jq .
```

## Optional: Use The Top-Level Shorthand Endpoints

```bash
curl -s -X POST "$BASE/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq .

curl -s "$BASE/snapshot?filter=interactive" | jq .
```

These verify the direct bridge shortcuts in addition to the tab-scoped endpoints.

## What This Example Proves

If the full sequence works, your bridge runtime can:
- start and respond to API requests
- manage multiple tabs
- inspect page structure and text
- execute actions
- render visual outputs
- support locking and basic browser state APIs

If this example fails, check:
- the bridge is running on `127.0.0.1:9867`
- Chrome can be started successfully
- required features such as `evaluate` are enabled before testing them
