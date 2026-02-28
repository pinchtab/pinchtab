# PinchTab API Workflows: Getting Started

Learn the correct way to use PinchTab's API through practical, step-by-step examples.

## Core Concept: Tab-Centric Design

PinchTab is **tab-centric**, not URL-centric. This is the key to understanding the API.

### Why Tab-Centric?

```
❌ WRONG ASSUMPTION:
"I can call /snapshot and pass a URL"
curl '/snapshot?url=https://example.com'

✅ CORRECT MODEL:
1. Create a tab (which navigates to a URL) → get tabId
2. Use tabId for all subsequent operations
```

### Why This Design?

- **Multiple tabs** can exist simultaneously, each with different state
- **Each tab has its own** cookies, navigation history, scripts, DOM
- **You need to specify WHICH tab** you're working with → `?tabId=X`
- **Matches Chrome's DevTools Protocol** (the standard browser API)
- **Enables multi-agent coordination** (agents can lock tabs)

---

## The Complete Workflow Pattern

Every PinchTab workflow follows this pattern:

```
1. Create tab (+ navigate to URL)  → Get tabId
                ↓
2. Interact with tab (snapshot, click, type, eval)
                ↓
3. Verify results (snapshot again, screenshot)
                ↓
4. Optional: Close tab when done
```

---

## Workflow 1: Text Extraction (Simplest)

**Goal:** Get readable text from a webpage.

### Step-by-Step

**Step 1: Create tab + navigate**
```bash
curl -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}'
```

**Response:**
```json
{
  "tabId": "ABC123DEF456",
  "title": "Example Domain",
  "url": "https://example.com/"
}
```

**Step 2: Extract text (use tabId)**
```bash
curl "http://localhost:9867/text?tabId=ABC123DEF456"
```

**Response:**
```json
{
  "text": "Example Domain\nThis domain is for use in examples...",
  "length": 234
}
```

**Complete script:**
```bash
#!/bin/bash
URL="https://example.com"

# Create tab and extract tabId
TAB_RESPONSE=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"new\",\"url\":\"$URL\"}")

TABID=$(echo $TAB_RESPONSE | jq -r '.tabId')
echo "Created tab: $TABID"

# Get text
curl -s "http://localhost:9867/text?tabId=$TABID" | jq '.text'

# Cleanup
curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TABID\"}"
```

---

## Workflow 2: Snapshot + Click (Interaction)

**Goal:** See page structure, click an element, verify the change.

### Step-by-Step

**Step 1: Create tab + navigate**
```bash
curl -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com/interactive"}'
```

**Response:**
```json
{
  "tabId": "XYZ789",
  "title": "Interactive Page",
  "url": "https://example.com/interactive"
}
```

**Step 2: Get snapshot (see interactive elements)**
```bash
curl "http://localhost:9867/snapshot?tabId=XYZ789&filter=interactive"
```

**Response:**
```json
{
  "elements": [
    {
      "ref": "e0",
      "role": "heading",
      "name": "Interactive Page"
    },
    {
      "ref": "e5",
      "role": "button",
      "name": "Click Me!"
    },
    {
      "ref": "e8",
      "role": "link",
      "name": "Learn More"
    }
  ]
}
```

**Step 3: Click element (e5)**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"XYZ789"}'
```

**Step 4: Verify result (snapshot again)**
```bash
curl "http://localhost:9867/snapshot?tabId=XYZ789&filter=interactive"
```

**Complete script:**
```bash
#!/bin/bash
URL="https://example.com/interactive"

# Step 1: Create tab
TAB=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"new\",\"url\":\"$URL\"}")

TABID=$(echo $TAB | jq -r '.tabId')
echo "Tab created: $TABID"

# Step 2: Get interactive elements
echo "Getting interactive elements..."
SNAPSHOT=$(curl -s "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive")
echo $SNAPSHOT | jq '.elements[] | {ref, name}'

# Step 3: Click first button (ref=e5)
echo "Clicking button e5..."
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"click\",\"ref\":\"e5\",\"tabId\":\"$TABID\"}"

# Step 4: Verify change
echo "Verifying result..."
curl -s "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" | jq '.elements'

# Cleanup
curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TABID\"}"
```

---

## Workflow 3: Form Filling

**Goal:** Fill a form and submit it.

### Step-by-Step

**Step 1: Navigate to form**
```bash
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com/login"}' | jq -r '.tabId')
```

**Step 2: Get form fields (snapshot)**
```bash
curl -s "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" | jq '.elements[]'
```

**Output:**
```json
{
  "ref": "e3",
  "role": "textbox",
  "name": "Email"
}
{
  "ref": "e5",
  "role": "textbox",
  "name": "Password"
}
{
  "ref": "e7",
  "role": "button",
  "name": "Sign In"
}
```

**Step 3: Fill email field (e3)**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com","tabId":"'"$TABID"'"}'
```

**Step 4: Fill password field (e5)**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"mypassword","tabId":"'"$TABID"'"}'
```

**Step 5: Click submit (e7)**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7","tabId":"'"$TABID"'"}'
```

**Step 6: Verify success (check page)**
```bash
curl -s "http://localhost:9867/text?tabId=$TABID" | jq '.text'
```

**Complete script:**
```bash
#!/bin/bash
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com/login"}' | jq -r '.tabId')

echo "Tab: $TABID"

# Get form
echo "Getting form fields..."
curl -s "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" | jq '.elements[] | {ref, name}'

# Fill email
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"fill\",\"ref\":\"e3\",\"text\":\"user@example.com\",\"tabId\":\"$TABID\"}"

# Fill password
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"fill\",\"ref\":\"e5\",\"text\":\"mypassword\",\"tabId\":\"$TABID\"}"

# Click submit
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"click\",\"ref\":\"e7\",\"tabId\":\"$TABID\"}"

# Wait for page load
sleep 2

# Check result
echo "Login result:"
curl -s "http://localhost:9867/text?tabId=$TABID" | jq '.text' | head -5

# Cleanup
curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TABID\"}"
```

---

## Workflow 4: Multi-Tab Coordination

**Goal:** Work with multiple tabs simultaneously.

### Step-by-Step

**Step 1: Create first tab**
```bash
TAB1=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://github.com"}' | jq -r '.tabId')

echo "Tab 1: $TAB1"
```

**Step 2: Create second tab**
```bash
TAB2=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://stackoverflow.com"}' | jq -r '.tabId')

echo "Tab 2: $TAB2"
```

**Step 3: List all tabs**
```bash
curl -s http://localhost:9867/tabs | jq '.tabs[]'
```

**Step 4: Get snapshot from each tab**
```bash
echo "Tab 1 snapshot:"
curl -s "http://localhost:9867/snapshot?tabId=$TAB1&format=text&maxTokens=500"

echo "Tab 2 snapshot:"
curl -s "http://localhost:9867/snapshot?tabId=$TAB2&format=text&maxTokens=500"
```

**Step 5: Interact with specific tabs**
```bash
# Click on Tab 1
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"click\",\"ref\":\"e5\",\"tabId\":\"$TAB1\"}"

# Click on Tab 2
curl -s -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"click\",\"ref\":\"e3\",\"tabId\":\"$TAB2\"}"
```

**Complete script:**
```bash
#!/bin/bash

# Create tabs
TAB1=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://github.com"}' | jq -r '.tabId')

TAB2=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://stackoverflow.com"}' | jq -r '.tabId')

echo "Tab 1: $TAB1"
echo "Tab 2: $TAB2"

# List all tabs
echo "Open tabs:"
curl -s http://localhost:9867/tabs | jq '.tabs[] | {id, url, title}'

# Get content from each
echo "Tab 1 content:"
curl -s "http://localhost:9867/text?tabId=$TAB1" | jq '.text' | head -3

echo "Tab 2 content:"
curl -s "http://localhost:9867/text?tabId=$TAB2" | jq '.text' | head -3

# Cleanup
curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TAB1\"}"

curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TAB2\"}"
```

---

## Workflow 5: PDF Export

**Goal:** Generate a PDF from a webpage.

### Step-by-Step

**Step 1: Navigate**
```bash
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com/report"}' | jq -r '.tabId')
```

**Step 2: Generate PDF**
```bash
curl "http://localhost:9867/pdf?tabId=$TABID&landscape=true&displayHeaderFooter=true" \
  -o report.pdf
```

**Complete script:**
```bash
#!/bin/bash
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com/report"}' | jq -r '.tabId')

echo "Creating PDF from tab: $TABID"

# Generate PDF with options
curl -s "http://localhost:9867/pdf?tabId=$TABID&landscape=true&displayHeaderFooter=true&headerTemplate=%3Cdiv%3EReport%3C%2Fdiv%3E" \
  -o report.pdf

echo "PDF saved: report.pdf"

# Cleanup
curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d "{\"action\":\"close\",\"tabId\":\"$TABID\"}"
```

---

## Common Mistakes & Solutions

### ❌ Mistake 1: Calling /snapshot without tabId

```bash
# WRONG - returns about:blank
curl "http://localhost:9867/snapshot?format=text"

# CORRECT - use tabId
curl "http://localhost:9867/snapshot?tabId=ABC123&format=text"
```

### ❌ Mistake 2: Passing URL to /action or /snapshot

```bash
# WRONG - /snapshot doesn't take a URL
curl "http://localhost:9867/snapshot?url=https://example.com"

# CORRECT - create tab first, then use tabId
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}' | jq -r '.tabId')
curl "http://localhost:9867/snapshot?tabId=$TABID"
```

### ❌ Mistake 3: Not extracting tabId from response

```bash
# WRONG - response is JSON, not plain text
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}')

# Use $TABID in queries → gets entire JSON object

# CORRECT - extract with jq
TABID=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}' | jq -r '.tabId')
```

### ❌ Mistake 4: Using refs from old snapshots

```bash
# WRONG - page changed, e5 no longer a button
curl "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" > snapshot1.json
# ... do something ...
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"'"$TABID"'"}'

# CORRECT - get fresh snapshot before clicking
curl "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" > snapshot2.json
# Read refs from snapshot2.json
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7","tabId":"'"$TABID"'"}'
```

---

## Key Takeaways

1. **Create tab first** — Every workflow starts with creating a tab
2. **Extract tabId** — Use jq: `jq -r '.tabId'`
3. **Pass tabId everywhere** — All operations need `?tabId=X` or in request body
4. **Get fresh snapshots** — Refs change when page updates; get new snapshot
5. **Chain operations** — Navigate → Snapshot → Click → Snapshot → Close

---

## Quick Reference: When to Use Each Endpoint

| Goal | Endpoint | Requires tabId |
|------|----------|---|
| Create/navigate | `POST /tab` | No (creates tab) |
| See page structure | `GET /snapshot` | Yes |
| See only buttons/links | `GET /snapshot?filter=interactive` | Yes |
| Click/type/press | `POST /action` | Yes |
| Extract text | `GET /text` | Yes |
| Run JavaScript | `POST /execute` | Yes |
| Screenshot | `GET /screenshot` | Yes |
| PDF export | `GET /pdf` | Yes |
| List tabs | `GET /tabs` | No |
| Close tab | `POST /tab` (close) | Yes (in body) |

---

## Debugging Tips

### Check if tab exists
```bash
curl http://localhost:9867/tabs | jq '.tabs[] | select(.id=="ABC123")'
```

### Get current page URL
```bash
curl -s "http://localhost:9867/text?tabId=$TABID" | jq '.url'
```

### Verify tabId is valid
```bash
curl -s "http://localhost:9867/snapshot?tabId=$TABID" | jq 'if .url then "Valid" else "Invalid" end'
```

### See all interactive elements
```bash
curl -s "http://localhost:9867/snapshot?tabId=$TABID&filter=interactive" | jq '.elements[].name'
```

---

## Next Steps

- **Workflow examples:** See [`docs/curl-commands.md`](curl-commands.md) for more examples
- **CLI alternative:** Try `pinchtab` CLI commands in [`docs/cli-commands.md`](cli-commands.md)
- **API reference:** Full endpoint documentation in [`docs/curl-commands.md`](curl-commands.md)
