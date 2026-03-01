# Before/After: Broken Examples vs. Fixed Examples

**Date:** 2026-03-01  
**Purpose:** Show exactly what's wrong in documentation and how to fix it

---

## Example 1: Text Extraction Workflow

### ❌ CURRENT (BROKEN) - From showcase.md

```bash
# Step 3: Navigate to URL (returns tabId)
TAB=$(curl -s -X POST http://localhost:9867/instances/inst_abc123/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq -r '.tabId')

# Step 4: Extract text (use tabId)
curl "http://localhost:9867/instances/inst_abc123/text?tabId=$TAB"
```

**Result:** ❌ 404 Not Found
```
POST /instances/inst_abc123/navigate → Route does not exist
GET /instances/inst_abc123/text?tabId=... → Route does not exist
```

### ✅ FIXED - What It Should Be

```bash
# Step 3: Create tab by navigating (returns tabId)
TAB=$(curl -s -X POST http://localhost:9867/tabs/$(jq -r '.id' <<< '...')/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# OR simpler: Create instance, then open tab in it
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# Step 4: Extract text (use tabId)
curl "http://localhost:9867/tabs/$TAB/text"
```

**Result:** ✅ 200 OK
```
POST /tabs/{id}/navigate → Works ✓
GET /tabs/{id}/text → Works ✓
```

---

## Example 2: Snapshot and Click Workflow

### ❌ CURRENT (BROKEN) - From showcase.md

```bash
# Get snapshot
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | jq '.nodes'

# Click a button
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "click",
    "ref": "e123"
  }'

# Get snapshot again
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | jq '.nodes'
```

**Result:** ❌ 404 errors
```
GET /instances/$INST/snapshot?tabId=... → Route does not exist
POST /instances/$INST/action → Route does not exist
```

### ✅ FIXED - What It Should Be

```bash
# Get snapshot
curl "http://localhost:9867/tabs/$TAB/snapshot" | jq '.nodes'

# Click a button
curl -X POST http://localhost:9867/tabs/$TAB/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "click",
    "ref": "e123"
  }'

# Get snapshot again
curl "http://localhost:9867/tabs/$TAB/snapshot" | jq '.nodes'
```

**Result:** ✅ 200 OK
```
GET /tabs/$TAB/snapshot → Works ✓
POST /tabs/$TAB/action → Works ✓
```

---

## Example 3: PDF Export

### ❌ CURRENT (MIXED) - From showcase.md

```bash
# Part works (this is actually correct!):
curl "http://localhost:9867/tabs/$TAB/pdf?landscape=true" \
  -o report.pdf

# But surrounding documentation mentions removed routes:
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB"  # ❌ Removed
```

**Result:** ⚠️ PDF part works, but surrounding examples don't

### ✅ FIXED - Consistent Pattern

```bash
# All operations use /tabs/{id}/ pattern
curl "http://localhost:9867/tabs/$TAB/snapshot" \
  -o snapshot.json

curl "http://localhost:9867/tabs/$TAB/pdf?landscape=true" \
  -o report.pdf

curl "http://localhost:9867/tabs/$TAB/screenshot" \
  -o screenshot.png
```

**Result:** ✅ All consistent

---

## Example 4: Multi-Tab Workflow

### ❌ CURRENT (BROKEN) - From showcase.md

```bash
# Create second tab - how?
# Documentation says: POST /instances/$INST/tab
TAB2=$(curl -s -X POST http://localhost:9867/instances/$INST/tab \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example2.com"}' | jq -r '.id')

# Interact with tabs
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB1"
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB2"
```

**Result:** ❌ Mixed - `POST /instances/{id}/tab` proxies but snapshot routes don't exist

### ✅ FIXED - Consistent Pattern

```bash
# Create second tab
TAB2=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example2.com"}' | jq -r '.id')

# Interact with tabs - both tabs use same /tabs/{id}/ pattern
curl "http://localhost:9867/tabs/$TAB1/snapshot" | jq '.title'
curl "http://localhost:9867/tabs/$TAB2/snapshot" | jq '.title'

# Take screenshots of each
curl "http://localhost:9867/tabs/$TAB1/screenshot" -o tab1.png
curl "http://localhost:9867/tabs/$TAB2/screenshot" -o tab2.png
```

**Result:** ✅ Consistent - all `/tabs/{id}/` pattern

---

## Example 5: Form Filling

### ❌ CURRENT (BROKEN) - From showcase.md

```bash
# Navigate
curl -X POST http://localhost:9867/instances/$INST/navigate \
  -d '{"url":"https://example.com/form"}'

# Get form elements
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" \
  | jq '.nodes[] | select(.role=="textbox")'

# Fill form
curl -X POST http://localhost:9867/instances/$INST/action \
  -d '{"action":"type", "ref":"input_1", "text":"John"}'
```

**Result:** ❌ All three routes fail

### ✅ FIXED - Correct Pattern

```bash
# Create tab by navigating (in the instance)
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/form"}'

# Get form elements
curl "http://localhost:9867/tabs/$TAB/snapshot" \
  | jq '.nodes[] | select(.role=="textbox")'

# Fill form
curl -X POST http://localhost:9867/tabs/$TAB/action \
  -H "Content-Type: application/json" \
  -d '{"action":"type", "ref":"input_1", "text":"John"}'
```

**Result:** ✅ All routes work

---

## Pattern Summary

### ❌ BROKEN PATTERNS (Remove These)
```bash
POST /instances/{id}/navigate
GET  /instances/{id}/snapshot?tabId=...
POST /instances/{id}/action
GET  /instances/{id}/text?tabId=...
POST /instances/{id}/actions
POST /instances/{id}/evaluate
GET  /instances/{id}/screenshot?tabId=...
POST /instances/{id}/cookies
GET  /instances/{id}/cookies?tabId=...
```

### ✅ CORRECT PATTERNS (Use These)
```bash
# For tab operations at orchestrator level:
POST /tabs/{id}/navigate
GET  /tabs/{id}/snapshot
POST /tabs/{id}/action
GET  /tabs/{id}/text
POST /tabs/{id}/actions
POST /tabs/{id}/evaluate
GET  /tabs/{id}/screenshot
POST /tabs/{id}/cookies
GET  /tabs/{id}/cookies

# For instance operations:
POST /instances/{id}/tabs/open      (create tab in instance)
GET  /instances/{id}/tabs           (list instance tabs)
POST /instances/{id}/tab            (legacy, works via proxy)
```

---

## Affected Locations in showcase.md

| Line Range | Current Pattern | Should Be | Impact |
|-----------|-----------------|-----------|--------|
| ~81 | `/instances/$INST/navigate` | `/instances/$INST/tabs/open` | Workflow breaks |
| ~99 | `/instances/$INST/text?tabId=` | `/tabs/$TAB/text` | Can't extract text |
| ~126 | `/instances/$INST/snapshot?tabId=` | `/tabs/$TAB/snapshot` | Can't snapshot |
| ~166 | `/instances/$INST/snapshot?tabId=` | `/tabs/$TAB/snapshot` | Can't verify action |
| ~194 | `/instances/$INST/action` | `/tabs/$TAB/action` | Can't interact |
| ~201 | `/instances/$INST/snapshot?tabId=` | `/tabs/$TAB/snapshot` | Can't verify state |
| ~231 | `/instances/$INST/action` | `/tabs/$TAB/action` | Can't click |
| ~270 | `/instances/$INST/snapshot?tabId=` | `/tabs/$TAB/snapshot` | Can't filter elements |
| ~294 | `/instances/$INST/action` | `/tabs/$TAB/action` | Multiple action failures |

---

## Quick Fix Checklist

### For showcase.md
- [ ] Replace `POST /instances/{id}/navigate` → `POST /instances/{id}/tabs/open`
- [ ] Replace `GET /instances/{id}/snapshot?tabId=` → `GET /tabs/{id}/snapshot`
- [ ] Replace `GET /instances/{id}/text?tabId=` → `GET /tabs/{id}/text`
- [ ] Replace `POST /instances/{id}/action` → `POST /tabs/{id}/action`
- [ ] Replace `POST /instances/{id}/actions` → `POST /tabs/{id}/actions`
- [ ] Replace `GET /instances/{id}/screenshot?tabId=` → `GET /tabs/{id}/screenshot`
- [ ] Replace `POST /instances/{id}/evaluate` → `POST /tabs/{id}/evaluate`

### For Other Documentation Files
- [ ] Check `docs/get-started.md` for same patterns
- [ ] Check `docs/guides/common-patterns.md` for same patterns
- [ ] Check `docs/guides/multi-instance.md` for same patterns
- [ ] Check `docs/references/endpoints.md` for same patterns
- [ ] Check `skill/pinchtab/references/api.md` for same patterns

---

## Validation After Fixing

To verify fixes work:

```bash
# Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

# Create tab
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# Test operations (should all 200)
curl http://localhost:9867/tabs/$TAB/snapshot -w "%{http_code}"      # Should be 200
curl http://localhost:9867/tabs/$TAB/text -w "%{http_code}"         # Should be 200
curl http://localhost:9867/tabs/$TAB/pdf -w "%{http_code}"          # Should be 200
curl http://localhost:9867/tabs/$TAB/screenshot -w "%{http_code}"   # Should be 200

# Stop
curl -X POST http://localhost:9867/instances/$INST/stop
```

If all return 200, fixes are good!

---

## Summary

**Before (Broken):**
```
User reads showcase.md
→ Tries examples
→ Gets 404 errors
→ Frustrated
```

**After (Fixed):**
```
User reads showcase.md
→ Tries examples
→ All work
→ Happy
```

---

**Ready for implementation.**
