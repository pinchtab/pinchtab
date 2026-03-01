# API Workflows

Learn the correct way to use PinchTab's API through practical, step-by-step examples.

## Core Concepts: Orchestrator → Instance → Tab

PinchTab uses a three-level hierarchy:

```
Orchestrator (port 9867)
  ↓
Instance (port 9868+)  ← Creates/manages Chrome process
  ↓
Tab                    ← Individual webpage within instance
```

### Why This Design?

- **Multiple instances** enable isolation (separate cookies, history, profiles)
- **Multiple tabs per instance** allow parallel work within same session
- **Orchestrator manages everything** via consistent routing (`/instances/{id}/...`)
- **Complete isolation** — Each instance has its own Chrome process, no resource contention
- **Supports multi-agent workflows** — Agents can coordinate via orchestrator

### Key Principles

1. **Create instance first** (once per workflow or reused)
2. **Create tabs within instance** (multiple tabs per instance)
3. **Use instance-scoped endpoints** (`/instances/{id}/...`)
4. **Stop instance when done** (cleanup, free resources)

---

## The Complete Workflow Pattern

Every PinchTab workflow follows this pattern:

```
1. Create instance       → Get instId
                ↓
2. Create tab (navigate) → Get tabId
                ↓
3. Interact with tab (snapshot, click, type, eval)
                ↓
4. Verify results (snapshot again, screenshot)
                ↓
5. Stop instance (cleanup)
```

---

## Workflow 1: Text Extraction (Simplest)

**Goal:** Get readable text from a webpage.

### Step-by-Step

**Step 1: Create instance**
```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
```

**Response:**
```json
{
  "id": "inst_abc123",
  "port": "9868",
  "status": "starting"
}
```

**Step 2: Wait for Chrome to initialize**
```bash
sleep 2
```

**Step 3: Navigate to URL**
```bash
curl -X POST http://localhost:9867/instances/inst_abc123/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

**Response:**
```json
{
  "tabId": "tab_def456",
  "title": "Example Domain",
  "url": "https://example.com/"
}
```

**Step 4: Extract text**
```bash
curl "http://localhost:9867/instances/inst_abc123/text"
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

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Created instance: $INST"

# Step 2: Wait for Chrome initialization
sleep 2

# Step 3: Navigate
curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\"}" > /dev/null

# Step 4: Get text
echo "Extracting text from $URL..."
curl -s "http://localhost:9867/instances/$INST/text" | jq '.text'

# Step 5: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 2: Snapshot + Click (Interaction)

**Goal:** See page structure, click an element, verify the change.

### Step-by-Step

**Step 1: Create instance and navigate**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/interactive"}' > /dev/null
```

**Step 2: Get snapshot (see interactive elements)**
```bash
curl "http://localhost:9867/instances/$INST/snapshot"
```

**Response:**
```json
{
  "nodes": [
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
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
```

**Step 4: Verify result (snapshot again)**
```bash
curl "http://localhost:9867/instances/$INST/snapshot" | jq '.nodes'
```

**Complete script:**
```bash
#!/bin/bash
URL="https://example.com/interactive"

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Instance: $INST"

# Step 2: Wait and navigate
sleep 2
curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\"}" > /dev/null

# Step 3: Get interactive elements
echo "Getting interactive elements..."
SNAPSHOT=$(curl -s "http://localhost:9867/instances/$INST/snapshot")
echo "$SNAPSHOT" | jq '.nodes[] | select(.role | IN("button", "link")) | {ref, name}'

# Step 4: Click button (ref=e5)
echo "Clicking button e5..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Step 5: Wait for action and verify
sleep 1
echo "Verifying result..."
curl -s "http://localhost:9867/instances/$INST/snapshot" | jq '.nodes[] | {ref, role, name}' | head -10

# Step 6: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 3: Form Filling

**Goal:** Fill a form and submit it.

### Step-by-Step

**Step 1: Create instance and navigate to form**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/login"}' > /dev/null
```

**Step 2: Get form fields (snapshot)**
```bash
curl -s "http://localhost:9867/instances/$INST/snapshot" | jq '.nodes[] | select(.role | IN("textbox", "button"))'
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
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com"}'
```

**Step 4: Fill password field (e5)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"mypassword"}'
```

**Step 5: Click submit (e7)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7"}'
```

**Step 6: Verify success (check page)**
```bash
curl -s "http://localhost:9867/instances/$INST/text" | jq '.text'
```

**Complete script:**
```bash
#!/bin/bash

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Instance: $INST"

# Step 2: Wait and navigate
sleep 2
curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/login"}' > /dev/null

# Step 3: Get form fields
echo "Getting form fields..."
curl -s "http://localhost:9867/instances/$INST/snapshot" | \
  jq '.nodes[] | select(.role | IN("textbox", "button")) | {ref, name}'

# Step 4: Fill email
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com"}'

# Step 5: Fill password
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"mypassword"}'

# Step 6: Click submit
echo "Submitting form..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7"}'

# Step 7: Wait for page load
sleep 2

# Step 8: Check result
echo "Login result:"
curl -s "http://localhost:9867/instances/$INST/text" | jq '.text' | head -5

# Step 9: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 4: Multi-Tab Coordination

**Goal:** Work with multiple tabs simultaneously within one instance.

### Step-by-Step

**Step 1: Create instance**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2
```

**Step 2: Create first tab**
```bash
curl -s -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"instanceId":"'$INST'","url":"https://github.com"}'
```

**Step 3: Create second tab**
```bash
curl -s -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"instanceId":"'$INST'","url":"https://stackoverflow.com"}'
```

**Step 4: List all tabs in instance**
```bash
curl -s "http://localhost:9867/instances/$INST/tabs" | jq '.[] | {id, title, url}'
```

**Step 5: Get snapshot from each tab**
```bash
TAB1=$(curl -s "http://localhost:9867/instances/$INST/tabs" | jq -r '.[0].id')
TAB2=$(curl -s "http://localhost:9867/instances/$INST/tabs" | jq -r '.[1].id')

# Get content from Tab 1
curl -s "http://localhost:9867/instances/$INST/text" | jq '.text' | head -3

# For Tab 2, we'd need explicit tab routing (advanced)
```

**Step 6: Interact with tabs**
```bash
# Click on element in Tab 1
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Take screenshot
curl "http://localhost:9867/instances/$INST/screenshot" -o screenshot.png
```

**Complete script:**
```bash
#!/bin/bash

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Instance: $INST"

# Step 2: Wait for initialization
sleep 2

# Step 3: Create tabs
echo "Creating tabs..."
curl -s -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"instanceId":"'$INST'","url":"https://github.com"}' > /dev/null

curl -s -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"instanceId":"'$INST'","url":"https://stackoverflow.com"}' > /dev/null

# Step 4: List tabs
echo "Tabs in instance:"
curl -s "http://localhost:9867/instances/$INST/tabs" | jq '.[] | {id, title, url}'

# Step 5: Get content from Tab 1
echo "Content from Tab 1:"
curl -s "http://localhost:9867/instances/$INST/text" | jq '.text' | head -3

# Step 6: Get snapshots
echo "Getting snapshots..."
curl -s "http://localhost:9867/instances/$INST/snapshot" | jq '.nodes | length'

# Step 7: Interact with element
echo "Clicking element..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Step 8: Take screenshot
echo "Taking screenshot..."
curl "http://localhost:9867/instances/$INST/screenshot" -o /tmp/page.png

# Step 9: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 5: PDF Export

**Goal:** Generate a PDF from a webpage.

### Step-by-Step

**Step 1: Create instance and navigate**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/report"}' > /dev/null
```

**Step 2: Generate PDF**
```bash
curl "http://localhost:9867/instances/$INST/pdf?landscape=true" \
  -o report.pdf
```

**Complete script:**
```bash
#!/bin/bash

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Instance: $INST"

# Step 2: Wait for initialization
sleep 2

# Step 3: Navigate to report page
echo "Navigating to report page..."
curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/report"}' > /dev/null

# Step 4: Generate PDF with options
echo "Generating PDF..."
curl -s "http://localhost:9867/instances/$INST/pdf?landscape=true&displayHeaderFooter=true" \
  -o report.pdf

echo "PDF saved: report.pdf"

# Step 5: Take screenshot for preview
echo "Taking preview screenshot..."
curl -s "http://localhost:9867/instances/$INST/screenshot" -o /tmp/report_preview.png

# Step 6: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Common Mistakes & Solutions

### ❌ Mistake 1: Forgetting to create instance first

```bash
# WRONG - instance doesn't exist yet
curl "http://localhost:9867/instances/$INST/snapshot"

# CORRECT - create instance, wait, then use
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')
sleep 2
curl "http://localhost:9867/instances/$INST/snapshot"
```

### ❌ Mistake 2: Not waiting for Chrome initialization

```bash
# WRONG - instance starts but Chrome isn't ready yet
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

curl -X POST http://localhost:9867/instances/$INST/navigate \
  -d '{"url":"https://example.com"}'  # Likely timeout or error

# CORRECT - wait for Chrome
sleep 2
```

### ❌ Mistake 3: Not extracting instance ID from response

```bash
# WRONG - trying to use JSON object as instance ID
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}')

# CORRECT - extract with jq
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')
```

### ❌ Mistake 4: Using refs from old snapshots

```bash
# WRONG - page changed, e5 no longer a button
curl "http://localhost:9867/instances/$INST/snapshot" > snapshot1.json
# ... do something ...
curl -X POST http://localhost:9867/instances/$INST/action \
  -d '{"kind":"click","ref":"e5"}'

# CORRECT - get fresh snapshot before clicking
curl "http://localhost:9867/instances/$INST/snapshot" > snapshot2.json
# Read refs from snapshot2.json
curl -X POST http://localhost:9867/instances/$INST/action \
  -d '{"kind":"click","ref":"e7"}'
```

### ❌ Mistake 5: Forgetting to stop instances

```bash
# WRONG - instances accumulate, wasting resources
INST1=$(curl -s -X POST http://localhost:9867/instances/launch ...)
INST2=$(curl -s -X POST http://localhost:9867/instances/launch ...)
# ... do work, then exit script ...
# Instances still running!

# CORRECT - stop when done
curl -X POST http://localhost:9867/instances/$INST1/stop
curl -X POST http://localhost:9867/instances/$INST2/stop
```

---

## Key Takeaways

1. **Create instance first** — Every workflow starts with creating an instance
2. **Wait for Chrome** — Sleep 2 seconds to allow lazy initialization
3. **Extract instance ID** — Use jq: `jq -r '.id'`
4. **Use instance-scoped paths** — All operations use `/instances/{id}/...`
5. **Get fresh snapshots** — Refs change when page updates; get new snapshot
6. **Stop instances** — Clean up when done to free resources
7. **Chain operations** — Create instance → Navigate → Snapshot → Click → Snapshot → Stop

---

## Quick Reference: When to Use Each Endpoint

| Goal | Endpoint | Notes |
|------|----------|-------|
| Create instance | `POST /instances/launch` | Returns instance ID |
| Navigate | `POST /instances/{id}/navigate` | Creates/uses first tab |
| See page structure | `GET /instances/{id}/snapshot` | Returns DOM nodes + refs |
| Click/type/press | `POST /instances/{id}/action` | Use ref from snapshot |
| Extract text | `GET /instances/{id}/text` | Returns readable text |
| Run JavaScript | `POST /instances/{id}/evaluate` | Returns JSON result |
| Screenshot | `GET /instances/{id}/screenshot` | Returns PNG image |
| PDF export | `GET /instances/{id}/pdf` | Returns PDF file |
| List tabs | `GET /instances/{id}/tabs` | All tabs in instance |
| New tab | `POST /tabs/open` | Open URL in new tab |
| Stop instance | `POST /instances/{id}/stop` | Cleanup, free resources |

---

## Debugging Tips

### Check if instance is running
```bash
curl http://localhost:9867/instances/$INST | jq '.status'
```

### Get instance logs
```bash
curl http://localhost:9867/instances/$INST/logs
```

### List all running instances
```bash
curl http://localhost:9867/instances | jq '.[] | {id, status, port}'
```

### Get current page URL and title
```bash
curl -s "http://localhost:9867/instances/$INST/snapshot" | jq '{url, title}'
```

### See all interactive elements
```bash
curl -s "http://localhost:9867/instances/$INST/snapshot" | \
  jq '.nodes[] | select(.role | IN("button", "link", "textbox")) | {ref, name}'
```

### Wait for instance to be ready
```bash
while [ "$(curl -s http://localhost:9867/instances/$INST | jq -r '.status')" != "running" ]; do
  sleep 0.5
done
echo "Instance ready!"
```

---

---

## For More Information

- **Instance API:** See [`references/instance-api.md`](references/instance-api.md) for complete endpoint details
- **Tabs API:** See [`references/tabs-api.md`](references/tabs-api.md) for tab management
- **curl Examples:** See [`curl-commands.md`](references/curl-commands.md) for more API examples
- **Core Concepts:** See [`core-concepts.md`](core-concepts.md) to understand instances, profiles, and tabs
