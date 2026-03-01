# Getting Started

Get PinchTab running in 5 minutes, from zero to browser automation.

---

## Installation (Choose One)

### Option 1: One-Liner (Recommended)

**macOS / Linux:**
```bash
curl -fsSL https://pinchtab.com/install.sh | bash
```

Then verify:
```bash
pinchtab --version
```

### Option 2: npm

**Requires:** Node.js 18+

```bash
npm install -g pinchtab
pinchtab --version
```

**Troubleshooting npm:**
```bash
# If "command not found", add npm to PATH
export PATH="$(npm config get prefix)/bin:$PATH"
# Add to ~/.bashrc or ~/.zshrc to persist
```

### Option 3: Docker

**Requires:** Docker

```bash
docker run -d -p 9867:9867 pinchtab/pinchtab
curl http://localhost:9867/health
```

### Option 4: Build from Source

**Requires:** Go 1.25+, Git, Chrome/Chromium

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
go build -o pinchtab ./cmd/pinchtab
./pinchtab --version
```

**[Full build guide →](architecture/building.md)**

---

## Quick Start (5 Minutes)

### Step 1: Start the Orchestrator

**Terminal 1:**
```bash
pinchtab
```

**Expected output:**
```
🦀 Pinchtab Dashboard port=9867
dashboard ready url=http://localhost:9867/dashboard
```

The orchestrator is running on `http://127.0.0.1:9867`. Open the dashboard in your browser to see instances and profiles.

### Step 2: Create an Instance

**Terminal 2:**
```bash
# Create a headless instance (background Chrome)
INST=$(pinchtab instance launch --mode headless | jq -r '.id')
echo "Instance created: $INST"

# Wait for Chrome to initialize (~2-5 seconds)
sleep 3
```

### Step 3: Run Your First Command

```bash
# Navigate to a website
pinchtab instance navigate $INST https://example.com

# Get page structure
curl http://localhost:9867/tabs/$TAB_ID/snapshot | jq '.nodes | map({role, name}) | .[0:5]'

# Extract text
curl http://localhost:9867/tabs/$TAB_ID/text | jq '.text'
```

✅ **You're running PinchTab!**

---

## Common First Commands

### Get Page Content

```bash
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

# Navigate
TAB_ID=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# Read the page as text
curl http://localhost:9867/tabs/$TAB_ID/text

# Get interactive elements (snapshot)
curl http://localhost:9867/tabs/$TAB_ID/snapshot | jq '.nodes | map({ref, role, name})'
```

### Take a Screenshot

```bash
# Save screenshot for a specific tab
curl "http://localhost:9867/tabs/$TAB_ID/screenshot" -o page.png
```

### Export as PDF

```bash
# Save PDF
curl "http://localhost:9867/tabs/$TAB_ID/pdf?landscape=true" -o output.pdf
```

### Interact with the Page

```bash
# Get page structure (snapshot)
SNAPSHOT=$(curl http://localhost:9867/tabs/$TAB_ID/snapshot)

# Extract a reference (e.g., first button)
BUTTON_REF=$(echo "$SNAPSHOT" | jq -r '.nodes[] | select(.role=="button") | .ref' | head -1)

# Click the button
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"'$BUTTON_REF'"}'

# Or fill a form input
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com"}'
```

### Multiple Tabs

```bash
# Create new tab in instance
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}'

# List all tabs
curl http://localhost:9867/tabs

# Operate on specific tab
TAB_ID=$(curl http://localhost:9867/instances/$INST/tabs | jq -r '.[0].id')

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"e5","tabId":"'$TAB_ID'"}'
```

---

## Understanding the Workflow

### Key Concepts

**Orchestrator** (port 9867):
- Manages instances
- Routes requests via `/instances/{id}/*`
- No Chrome process itself

**Instance** (ports 9868-9968):
- Real Chrome browser process
- Has one or more tabs
- Isolated cookies, history, storage
- Each has unique ID: `inst_XXXXXXXX`

**Tab**:
- Single webpage within instance
- Has state (URL, DOM, focus, content)
- Unique ID: `tab_XXXXXXXX`

### Typical Workflow

```bash
# 1. Create instance (Chrome starts lazily on first request)
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

# 2. Navigate to a page
TAB_ID=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# 3. Get page structure (see buttons, links, inputs)
curl http://localhost:9867/tabs/$TAB_ID/snapshot

# 4. Interact with page (click, type, etc.)
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"e5"}'

# 5. Verify changes
curl http://localhost:9867/tabs/$TAB_ID/snapshot

# 6. Capture result
curl "http://localhost:9867/tabs/$TAB_ID/screenshot" -o page.png
curl "http://localhost:9867/tabs/$TAB_ID/pdf" -o report.pdf

# 7. Stop instance (clean up)
curl -X POST http://localhost:9867/instances/$INST/stop
```

---

## Using with curl (HTTP API)

You don't need the CLI. PinchTab is HTTP:

```bash
# Health check
curl http://localhost:9867/health

# Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch | jq -r '.id')
sleep 2

# Navigate
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Get snapshot
curl http://localhost:9867/tabs/$TAB_ID/snapshot

# Extract text
curl http://localhost:9867/tabs/$TAB_ID/text

# Stop instance
curl -X POST http://localhost:9867/instances/$INST/stop
```

**Full API reference** → [curl-commands.md](references/curl-commands.md)  
**Instance API details** → [instance-api.md](references/instance-api.md)

---

## Using with Python

```python
import requests
import json
import time

BASE = "http://localhost:9867"

# 1. Create instance
resp = requests.post(f"{BASE}/instances/launch", json={"mode": "headless"})
inst_id = resp.json()["id"]
print(f"Created instance: {inst_id}")

# Wait for Chrome to initialize
time.sleep(2)

# 2. Create tab by navigating
resp = requests.post(f"{BASE}/instances/{inst_id}/tabs/open", json={
    "url": "https://example.com"
})
tab_id = resp.json()["id"]
print(f"Navigated: {resp.json()}")

# 3. Get snapshot
snapshot = requests.get(f"{BASE}/tabs/{tab_id}/snapshot").json()

# Print interactive elements
for elem in snapshot.get("nodes", []):
    if elem.get("role") in ["button", "link"]:
        print(f"{elem['ref']}: {elem['role']} - {elem['name']}")

# 4. Click an element
requests.post(f"{BASE}/tabs/{tab_id}/action", json={
    "action": "click",
    "ref": "e5"
})

# 5. Get text
text = requests.get(f"{BASE}/tabs/{tab_id}/text").json()
print(f"Page text: {text['text'][:200]}...")

# 6. Stop instance
requests.post(f"{BASE}/instances/{inst_id}/stop")
print(f"Stopped instance: {inst_id}")
```

---

## Using with Node.js

```javascript
const fetch = require('node-fetch');

const BASE = "http://localhost:9867";

async function main() {
  try {
    // 1. Create instance
    const launchResp = await fetch(`${BASE}/instances/launch`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode: "headless" })
    });
    const launch = await launchResp.json();
    const instId = launch.id;
    console.log(`Created instance: ${instId}`);

    // Wait for Chrome to initialize
    await new Promise(r => setTimeout(r, 2000));

    // 2. Create tab by navigating
    const navResp = await fetch(`${BASE}/instances/${instId}/tabs/open`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url: "https://example.com"
      })
    });
    const navData = await navResp.json();
    const tabId = navData.id;

    // 3. Get snapshot
    const snapResp = await fetch(`${BASE}/tabs/${tabId}/snapshot`);
    const snap = await snapResp.json();

    // Print interactive elements
    snap.nodes
      .filter(n => ["button", "link"].includes(n.role))
      .forEach(n => console.log(`${n.ref}: ${n.role} - ${n.name}`));

    // 4. Click element
    await fetch(`${BASE}/tabs/${tabId}/action`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        action: "click",
        ref: "e5"
      })
    });

    // 5. Get text
    const textResp = await fetch(`${BASE}/tabs/${tabId}/text`);
    const text = await textResp.json();
    console.log(`Page text: ${text.text.substring(0, 200)}...`);

    // 6. Stop instance
    await fetch(`${BASE}/instances/${instId}/stop`, {
      method: "POST"
    });
    console.log(`Stopped instance: ${instId}`);

  } catch (error) {
    console.error("Error:", error);
  }
}

main();
```

---

## Configuration

### Orchestrator Configuration

```bash
# Custom port (orchestrator)
BRIDGE_PORT=9868 pinchtab

# Auth token for remote access
BRIDGE_TOKEN=my-secret-token pinchtab

# Bind to all interfaces (for remote access)
BRIDGE_BIND=0.0.0.0 pinchtab

# Custom Chrome binary (used by all instances)
CHROME_BIN=/usr/bin/google-chrome pinchtab
```

### Instance Configuration

Instance-specific options are set when creating instances:

```bash
# Headless (default, fastest)
pinchtab instance launch --mode headless

# Headed (visible window, for debugging)
pinchtab instance launch --mode headed

# Specific port (usually auto-allocated)
pinchtab instance launch --port 9999

# With persistent profile
pinchtab profile create work
PROF_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="work") | .id')
curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$PROF_ID'","mode":"headed"}'
```

**[Full configuration →](references/configuration.md)**

---

## Common Scenarios

### Scenario 1: Scrape a Website

```bash
# Create instance
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

# Navigate
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com/article"}'

# Extract text
curl http://localhost:9867/tabs/$TAB_ID/text | jq '.text'

# Save to file
curl http://localhost:9867/tabs/$TAB_ID/text | jq -r '.text' > article.txt

# Stop instance
curl -X POST http://localhost:9867/instances/$INST/stop
```

### Scenario 2: Fill and Submit a Form

```bash
# Create instance
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

# Navigate to form
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com/contact"}'

# Get form structure
SNAP=$(curl http://localhost:9867/tabs/$TAB_ID/snapshot)

# Fill fields (get refs from snapshot)
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e3","text":"John Doe"}'

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e5","text":"john@example.com"}'

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e7","text":"My message here"}'

# Click submit
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"e10"}'

# Verify success
curl http://localhost:9867/tabs/$TAB_ID/snapshot | jq '.nodes | length'
```

### Scenario 3: Login + Stay Logged In

```bash
# Create persistent profile
pinchtab profile create mylogin
PROF_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="mylogin") | .id')

# Start instance with profile
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$PROF_ID'"}' | jq -r '.id')
sleep 2

# Login
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com/login"}'

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e3","text":"user@example.com"}'

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e5","text":"password"}'

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"e7"}'

# Stop instance (profile saved)
curl -X POST http://localhost:9867/instances/$INST/stop

# Later: restart with same profile (already logged in!)
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$PROF_ID'"}' | jq -r '.id')
sleep 2

# Navigate to dashboard (cookies preserved)
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com/dashboard"}'
```

### Scenario 4: Generate PDF Report

```bash
# Create instance
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

TAB_ID=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://reports.example.com/monthly"}' | jq -r '.id')
# Export PDF
curl "http://localhost:9867/tabs/$TAB_ID/pdf?landscape=true" -o report.pdf
```

### Scenario 5: Multi-Tab Workflow

```bash
# Create instance
INST=$(pinchtab instance launch | jq -r '.id')
sleep 2

# Open first tab (source)
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://source.example.com"}'

# Open second tab (destination)
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://destination.example.com"}'

# List tabs
TABS=$(curl http://localhost:9867/instances/$INST/tabs)
SOURCE_TAB=$(echo "$TABS" | jq -r '.[0].id')
DEST_TAB=$(echo "$TABS" | jq -r '.[1].id')

# Extract from source tab
DATA=$(curl "http://localhost:9867/tabs/$TAB_ID/text" | jq -r '.text')

# Fill destination tab
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"'$DATA'","tabId":"'$DEST_TAB'"}'
```

---

## Troubleshooting

### "connection refused" / "Cannot connect"

**Problem:** Orchestrator not running

**Solution:**
```bash
# Terminal 1: Start orchestrator
pinchtab

# Terminal 2: Check health (once running)
curl http://localhost:9867/health
```

### "Instance stuck in 'starting' state"

**Problem:** Chrome takes time to initialize (8-20 seconds)

**Solution:**
```bash
# Poll instance status
INST=$(pinchtab instance launch | jq -r '.id')

# Wait until 'running'
while [ "$(curl -s http://localhost:9867/instances/$INST | jq -r '.status')" != "running" ]; do
  sleep 0.5
done

# Now safe to use
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}'
```

### "Port already in use"

**Problem:** Port 9867 (or instance port) is taken

**Solution:**
```bash
# Use different orchestrator port
BRIDGE_PORT=9868 pinchtab

# Or kill the process using 9867
lsof -i :9867
kill -9 <PID>

# Instance ports auto-allocated from 9868-9968, no manual config needed
```

### "Chrome not found"

**Problem:** Chrome/Chromium not installed

**Solution:**
```bash
# macOS
brew install chromium

# Linux (Ubuntu/Debian)
sudo apt install chromium-browser

# Or specify custom Chrome
CHROME_BIN=/path/to/chrome pinchtab
```

### "Empty snapshot / 404 error"

**Problem:** Instance ID is invalid or instance was stopped

**Solution:**
```bash
# List running instances
curl http://localhost:9867/instances

# Use valid instance ID
INST=$(pinchtab instance launch | jq -r '.id')

# Verify it's running
curl http://localhost:9867/instances/$INST
```

### "ref e5 not found"

**Problem:** Page updated or different page loaded

**Solution:**
```bash
# Get fresh snapshot
SNAP=$(curl http://localhost:9867/tabs/$TAB_ID/snapshot)

# Extract new ref from snapshot
NEW_REF=$(echo "$SNAP" | jq -r '.nodes[] | select(.role=="button") | .ref' | head -1)

# Use new ref
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"'$NEW_REF'"}'
```

---

## Common Features

### Extract Text Efficiently

```bash
# Get text content
curl http://localhost:9867/tabs/$TAB_ID/text

# Get snapshot (DOM structure)
curl http://localhost:9867/tabs/$TAB_ID/snapshot

# Filter snapshot to interactive elements
curl http://localhost:9867/tabs/$TAB_ID/snapshot | \
  jq '.nodes[] | select(.role | IN("button", "link", "textbox"))'
```

### Interact with Page

```bash
# Click element
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"click","ref":"e5"}'

# Type text
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"type","ref":"e3","text":"hello"}'

# Fill input
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"fill","ref":"e3","text":"value"}'

# Press key
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"press","key":"Enter"}'

# Focus element
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"focus","ref":"e5"}'

# Hover element
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"hover","ref":"e5"}'

# Select dropdown
curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -d '{"kind":"select","ref":"e8","text":"Option 2"}'
```

### Run JavaScript

```bash
# Evaluate expression
curl -X POST http://localhost:9867/instances/$INST/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'

# Get page info
curl -X POST http://localhost:9867/instances/$INST/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"JSON.stringify({title: document.title, url: location.href})"}'
```

---

## Performance Tips

### Use Headless for Speed

```bash
# Headless (default, faster)
pinchtab instance launch --mode headless

# Headed (visible window, slower, better for debugging)
pinchtab instance launch --mode headed
```

### Parallel Processing

```bash
# Create multiple instances for concurrent work
for i in 1 2 3 4 5; do
  INST=$(pinchtab instance launch | jq -r '.id')
  INSTANCES+=("$INST")
done

# Distribute work across instances (round-robin)
for URL in "${URLS[@]}"; do
  INST="${INSTANCES[$((INDEX % 5))]}"
  curl -X POST "http://localhost:9867/instances/$INST/tabs/open" \
    -d '{"url":"'$URL'"}' &
  ((INDEX++))
done
wait
```

### Token Efficiency

```bash
# Use text extraction (cheaper than screenshots)
curl http://localhost:9867/tabs/$TAB_ID/text      # Lower tokens

# Use snapshot (cheaper than screenshot)
curl http://localhost:9867/tabs/$TAB_ID/snapshot  # Lower tokens

# Screenshots are expensive (JPG encoding)
curl "http://localhost:9867/tabs/$TAB_ID/screenshot" # Higher tokens
```

---

## Quick Reference

| Task | Command |
|------|---------|
| Start orchestrator | `pinchtab` |
| Health check | `curl http://localhost:9867/health` |
| Create instance | `INST=$(pinchtab instance launch \| jq -r '.id')` |
| Navigate | `curl -X POST http://localhost:9867/instances/$INST/tabs/open -d '{"url":"..."}' ` |
| See structure | `curl http://localhost:9867/tabs/$TAB_ID/snapshot` |
| Get text | `curl http://localhost:9867/tabs/$TAB_ID/text` |
| Click element | `curl -X POST http://localhost:9867/tabs/$TAB_ID/action -d '{"kind":"click","ref":"e5"}'` |
| Type text | `curl -X POST http://localhost:9867/tabs/$TAB_ID/action -d '{"kind":"type","ref":"e3","text":"hello"}'` |
| Screenshot | `curl "http://localhost:9867/tabs/$TAB_ID/screenshot" -o page.png` |
| PDF export | `curl http://localhost:9867/tabs/$TAB_ID/pdf -o out.pdf` |
| List instances | `curl http://localhost:9867/instances` |
| List tabs | `curl http://localhost:9867/instances/$INST/tabs` |
| New tab | `curl -X POST http://localhost:9867/instances/$INST/tabs/open -d '{"url":"..."}'` |
| Stop instance | `curl -X POST http://localhost:9867/instances/$INST/stop` |

---

## Getting Help

- **Issues** → [GitHub Issues](https://github.com/pinchtab/pinchtab/issues)
- **Q&A** → [GitHub Discussions](https://github.com/pinchtab/pinchtab/discussions)
- **Docs** → [Full documentation](overview.md)

**Happy automating!** 🦀
