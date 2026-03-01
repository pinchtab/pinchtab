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

**Step 3: Navigate to URL (returns tabId)**
```bash
TAB=$(curl -s -X POST http://localhost:9867/instances/inst_abc123/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq -r '.tabId')

echo "Tab created: $TAB"
```

**Response contains:**
```json
{
  "tabId": "tab_def456",
  "title": "Example Domain",
  "url": "https://example.com/"
}
```

**Step 4: Extract text (use tabId)**
```bash
curl "http://localhost:9867/instances/inst_abc123/text?tabId=$TAB"
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

# Step 3: Navigate and capture tabId
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\"}" | jq -r '.tabId')

echo "Created tab: $TAB"

# Step 4: Get text using tabId
echo "Extracting text from $URL..."
curl -s "http://localhost:9867/instances/$INST/text?tabId=$TAB" | jq '.text'

# Step 5: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 2: Snapshot + Click (Interaction)

**Goal:** See page structure, click an element, verify the change.

### Step-by-Step

**Step 1: Create instance and navigate (get tabId)**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/interactive"}' | jq -r '.tabId')

echo "Instance: $INST, Tab: $TAB"
```

**Step 2: Get snapshot (see interactive elements, use tabId)**
```bash
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB"
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

**Step 3: Click element (e5, use tabId)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"'$TAB'"}'
```

**Step 4: Verify result (snapshot again, use tabId)**
```bash
curl "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | jq '.nodes'
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

# Step 2: Wait and navigate (capture tabId)
sleep 2
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\"}" | jq -r '.tabId')

echo "Tab: $TAB"

# Step 3: Get interactive elements using tabId
echo "Getting interactive elements..."
SNAPSHOT=$(curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB")
echo "$SNAPSHOT" | jq '.nodes[] | select(.role | IN("button", "link")) | {ref, name}'

# Step 4: Click button using tabId
echo "Clicking button e5..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"'$TAB'"}'

# Step 5: Wait for action and verify using tabId
sleep 1
echo "Verifying result..."
curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | jq '.nodes[] | {ref, role, name}' | head -10

# Step 6: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 3: Form Filling

**Goal:** Fill a form and submit it.

### Step-by-Step

**Step 1: Create instance and navigate to form (capture tabId)**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/login"}' | jq -r '.tabId')

echo "Instance: $INST, Tab: $TAB"
```

**Step 2: Get form fields (snapshot with tabId)**
```bash
curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | jq '.nodes[] | select(.role | IN("textbox", "button"))'
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

**Step 3: Fill email field (e3, use tabId)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com","tabId":"'$TAB'"}'
```

**Step 4: Fill password field (e5, use tabId)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"mypassword","tabId":"'$TAB'"}'
```

**Step 5: Click submit (e7, use tabId)**
```bash
curl -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7","tabId":"'$TAB'"}'
```

**Step 6: Verify success (check page, use tabId)**
```bash
curl -s "http://localhost:9867/instances/$INST/text?tabId=$TAB" | jq '.text'
```

**Complete script:**
```bash
#!/bin/bash

# Step 1: Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

echo "Instance: $INST"

# Step 2: Wait and navigate (capture tabId)
sleep 2
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/login"}' | jq -r '.tabId')

echo "Tab: $TAB"

# Step 3: Get form fields
echo "Getting form fields..."
curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB" | \
  jq '.nodes[] | select(.role | IN("textbox", "button")) | {ref, name}'

# Step 4: Fill email using tabId
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com","tabId":"'$TAB'"}'

# Step 5: Fill password using tabId
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"mypassword","tabId":"'$TAB'"}'

# Step 6: Click submit using tabId
echo "Submitting form..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e7","tabId":"'$TAB'"}'

# Step 7: Wait for page load
sleep 2

# Step 8: Check result using tabId
echo "Login result:"
curl -s "http://localhost:9867/instances/$INST/text?tabId=$TAB" | jq '.text' | head -5

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

**Step 2: Create first tab (get tabId)**
```bash
TAB1=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}' | jq -r '.tabId')

echo "Tab 1: $TAB1"
```

**Step 3: Create second tab (get tabId)**
```bash
TAB2=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://stackoverflow.com"}' | jq -r '.tabId')

echo "Tab 2: $TAB2"
```

**Step 4: List all tabs in instance**
```bash
curl -s "http://localhost:9867/instances/$INST/tabs" | jq '.[] | {id, title, url}'
```

**Step 5: Get snapshot from each tab (use tabIds)**
```bash
# Get content from Tab 1
curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB1" | jq '.nodes | length'

# Get content from Tab 2
curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB2" | jq '.nodes | length'
```

**Step 6: Interact with tabs (use tabIds)**
```bash
# Click on element in Tab 1
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"'$TAB1'"}'

# Click on element in Tab 2
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e3","tabId":"'$TAB2'"}'

# Take screenshot of Tab 1
curl "http://localhost:9867/tabs/$TAB1/screenshot" -o screenshot1.png
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

# Step 3: Create tabs and capture tabIds
echo "Creating tabs..."
TAB1=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}' | jq -r '.tabId')

TAB2=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://stackoverflow.com"}' | jq -r '.tabId')

echo "Tab 1: $TAB1"
echo "Tab 2: $TAB2"

# Step 4: List tabs
echo "Tabs in instance:"
curl -s "http://localhost:9867/instances/$INST/tabs" | jq '.[] | {id, title, url}'

# Step 5: Get content from Tab 1 using tabId
echo "Content from Tab 1:"
curl -s "http://localhost:9867/instances/$INST/text?tabId=$TAB1" | jq '.text' | head -3

# Step 6: Get snapshots from both tabs
echo "Getting snapshots..."
echo "Tab 1 nodes: $(curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB1" | jq '.nodes | length')"
echo "Tab 2 nodes: $(curl -s "http://localhost:9867/instances/$INST/snapshot?tabId=$TAB2" | jq '.nodes | length')"

# Step 7: Interact with elements in each tab
echo "Clicking element in Tab 1..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5","tabId":"'$TAB1'"}'

echo "Clicking element in Tab 2..."
curl -s -X POST http://localhost:9867/instances/$INST/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e3","tabId":"'$TAB2'"}'

# Step 8: Take screenshots of each tab
echo "Taking screenshots..."
curl "http://localhost:9867/tabs/$TAB1/screenshot" -o /tmp/tab1.png
curl "http://localhost:9867/tabs/$TAB2/screenshot" -o /tmp/tab2.png

# Step 9: Cleanup
curl -s -X POST "http://localhost:9867/instances/$INST/stop"
echo "Stopped instance: $INST"
```

---

## Workflow 5: PDF Export

**Goal:** Generate a PDF from a webpage.

### Step-by-Step

**Step 1: Create instance and navigate (capture tabId)**
```bash
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

sleep 2

TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/report"}' | jq -r '.tabId')

echo "Instance: $INST, Tab: $TAB"
```

**Step 2: Generate PDF (use tabId)**
```bash
curl "http://localhost:9867/tabs/$TAB/pdf?landscape=true" \
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

# Step 3: Navigate to report page (capture tabId)
echo "Navigating to report page..."
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/report"}' | jq -r '.tabId')

echo "Tab: $TAB"

# Step 4: Generate PDF with options using tabId
echo "Generating PDF..."
curl -s "http://localhost:9867/tabs/$TAB/pdf?landscape=true&displayHeaderFooter=true" \
  -o report.pdf

echo "PDF saved: report.pdf"

# Step 5: Take screenshot for preview using tabId
echo "Taking preview screenshot..."
curl -s "http://localhost:9867/tabs/$TAB/screenshot" -o /tmp/report_preview.png

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
4. **Use tab-scoped paths for tab work** — Prefer `/tabs/{id}/...` for navigate/snapshot/screenshot/pdf
5. **Get fresh snapshots** — Refs change when page updates; get new snapshot
6. **Stop instances** — Clean up when done to free resources
7. **Chain operations** — Create instance → Navigate → Snapshot → Click → Snapshot → Stop

---

## Quick Reference: When to Use Each Endpoint

| Goal | Endpoint | Notes |
|------|----------|-------|
| Create instance | `POST /instances/launch` | Returns instance ID |
| Navigate | `POST /tabs/{id}/navigate` | Navigates a specific tab |
| See page structure | `GET /tabs/{id}/snapshot` | Returns DOM nodes + refs |
| Click/type/press | `POST /tabs/{id}/action` | Use ref from snapshot |
| Extract text | `GET /tabs/{id}/text` | Returns readable text |
| Run JavaScript | `POST /tabs/{id}/evaluate` | Returns JSON result |
| Screenshot | `GET /tabs/{id}/screenshot` | Returns JPEG image |
| PDF export | `GET /tabs/{id}/pdf` | Returns PDF file |
| List tabs | `GET /instances/{id}/tabs` | All tabs in instance |
| New tab | `POST /instances/{id}/tabs/open` | Open URL in new tab |
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
