# Getting Started with Pinchtab

Get Pinchtab running in 5 minutes, from zero to browser automation.

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

### Step 1: Start the Server

**Terminal 1:**
```bash
pinchtab
```

**Expected output:**
```
🦀 PINCH! PINCH! port=9867 cdp= stealth=light
auth disabled (set BRIDGE_TOKEN to enable)
startup configuration bind=127.0.0.1 port=9867 headless=true ...
```

The server is running on `http://127.0.0.1:9867`.

### Step 2: Run Your First Command

**Terminal 2:**
```bash
pinchtab quick https://example.com
```

**What happens:**
1. Navigates to the URL
2. Shows page structure
3. Lists interactive elements
4. Suggests next actions

**Output:**
```
🦀 Navigating to https://example.com ...

📋 Page structure:
[accessibility tree...]

📌 Title: Example Domain
🔗 URL: https://example.com/

💡 Quick actions:
  pinchtab click <ref>        # Click an element
  pinchtab type <ref> <text>  # Type into input
  pinchtab screenshot         # Take a screenshot
  pinchtab pdf -o output.pdf  # Save as PDF
```

✅ **You're running Pinchtab!**

---

## Common First Commands

### Get Page Content

```bash
# Read the page as text
pinchtab text

# See interactive elements
pinchtab snap -i -c

# Get raw HTML content
pinchtab text --raw
```

### Take a Screenshot

```bash
pinchtab screenshot -o page.jpg
pinchtab screenshot -o page.jpg -q 75  # Lower quality
```

### Export as PDF

```bash
pinchtab pdf -o output.pdf
pinchtab pdf -o report.pdf --landscape
```

### Interact with the Page

```bash
# First, get interactive elements
pinchtab snap -i -c

# Then click one (replace 'e5' with a real ref)
pinchtab click e5

# Verify the change
pinchtab snap -i -c

# Or fill a form
pinchtab fill e3 "user@example.com"
pinchtab fill e5 "password"
pinchtab click e7  # Submit button
```

### Multiple Tabs

```bash
# Open second tab
pinchtab tabs new https://github.com

# List all tabs
pinchtab tabs

# Get snapshot from specific tab (replace tabId)
pinchtab snap --tab abc123def456
```

---

## Understanding the Workflow

### Key Concept: Tab-Centric

Pinchtab is **tab-centric**, not URL-centric:

```
❌ WRONG: pinchtab snap https://example.com
✅ CORRECT:
  1. pinchtab nav https://example.com    (uses current tab)
  2. pinchtab snap                       (snapshots current tab)
```

**Why?** Because a tab has state (cookies, history, focus, loaded content). Multiple tabs can exist. You need to specify which tab to work with.

### Typical Workflow

```
1. Navigate to a page
   pinchtab nav https://example.com

2. Get page structure
   pinchtab snap -i -c    # See buttons, links, inputs

3. Interact with page
   pinchtab click e5      # Click element
   pinchtab type e3 "text"  # Type text

4. Verify changes
   pinchtab snap -i -c    # Check new state

5. Capture result
   pinchtab screenshot    # Take screenshot
   pinchtab pdf -o out.pdf  # Export PDF
```

---

## Using with curl (HTTP API)

You don't need the CLI. Pinchtab is HTTP:

```bash
# Health check
curl http://localhost:9867/health

# Navigate (returns tabId)
curl -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}'

# Get snapshot
curl "http://localhost:9867/snapshot?tabId=abc123"

# Extract text
curl "http://localhost:9867/text?tabId=abc123"
```

**Full API reference** → [curl-commands.md](references/curl-commands.md)

---

## Using with Python

```python
import requests
import json

BASE = "http://localhost:9867"

# Create tab + navigate
resp = requests.post(f"{BASE}/tab", json={
    "action": "new",
    "url": "https://example.com"
})
tab_id = resp.json()["tabId"]

# Get snapshot
snapshot = requests.get(f"{BASE}/snapshot", params={
    "tabId": tab_id,
    "filter": "interactive"
}).json()

# Print interactive elements
for elem in snapshot["elements"]:
    print(f"{elem['ref']}: {elem['role']} - {elem['name']}")

# Click an element
requests.post(f"{BASE}/action", json={
    "kind": "click",
    "ref": "e5",
    "tabId": tab_id
})
```

---

## Using with Node.js

```javascript
const fetch = require('node-fetch');

const BASE = "http://localhost:9867";

async function main() {
  // Create tab
  const navResp = await fetch(`${BASE}/tab`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      action: "new",
      url: "https://example.com"
    })
  });
  const nav = await navResp.json();
  const tabId = nav.tabId;

  // Get snapshot
  const snapResp = await fetch(
    `${BASE}/snapshot?tabId=${tabId}&filter=interactive`
  );
  const snap = await snapResp.json();

  // Click element
  await fetch(`${BASE}/action`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      kind: "click",
      ref: "e5",
      tabId: tabId
    })
  });
}

main();
```

---

## Configuration

### Basic Configuration

```bash
# Custom port
BRIDGE_PORT=9868 pinchtab

# Visible window
BRIDGE_HEADLESS=false pinchtab

# Stealth mode (bypass bot detection)
BRIDGE_STEALTH=full pinchtab

# Block ads
BRIDGE_BLOCK_ADS=true pinchtab

# Custom Chrome
CHROME_BINARY=/usr/bin/google-chrome pinchtab
```

### Combined

```bash
BRIDGE_PORT=9868 \
BRIDGE_STEALTH=full \
BRIDGE_BLOCK_ADS=true \
BRIDGE_HEADLESS=false \
pinchtab
```

**[Full configuration →](architecture/pinchtab-architecture.md)**

---

## Common Scenarios

### Scenario 1: Scrape a Website

```bash
# Navigate
pinchtab nav https://example.com/article

# Extract readable text
pinchtab text > article.txt

# Or with curl
curl http://localhost:9867/text > article.json
jq '.text' article.json
```

### Scenario 2: Fill a Form

```bash
# Navigate to form
pinchtab nav https://example.com/contact

# Get interactive elements
pinchtab snap -i -c

# Fill form fields (use refs from snapshot)
pinchtab fill e3 "John Doe"
pinchtab fill e5 "john@example.com"
pinchtab fill e7 "My message here"

# Click submit
pinchtab click e10

# Verify success
pinchtab snap
```

### Scenario 3: Login + Stay Logged In

```bash
# Login
pinchtab nav https://example.com/login
pinchtab fill e3 "user@example.com"
pinchtab fill e5 "password"
pinchtab click e7

# Later, even after restarting Pinchtab, you're still logged in
pkill pinchtab
sleep 2
pinchtab &

# Tab and session restored
pinchtab nav https://example.com/dashboard
pinchtab snap  # Already authenticated
```

### Scenario 4: Generate PDF Report

```bash
# Navigate to report page
pinchtab nav https://reports.example.com/monthly

# Export as PDF with header/footer
pinchtab pdf -o report.pdf \
  --landscape \
  --display-header-footer \
  --header-template "<div>Monthly Report</div>" \
  --footer-template "<div>Page <span class='pageNumber'></span></div>"
```

### Scenario 5: Multi-Tab Workflow

```bash
# Open first tab
pinchtab nav https://source.example.com
SOURCE_TAB=$(pinchtab tabs | jq -r '.[0].id')

# Open second tab
pinchtab tabs new https://destination.example.com
DEST_TAB=$(pinchtab tabs | jq -r '.[1].id')

# Extract from source
DATA=$(curl "http://localhost:9867/text?tabId=$SOURCE_TAB")

# Fill in destination
curl -X POST http://localhost:9867/action \
  -d "{\"kind\":\"fill\",\"ref\":\"e3\",\"text\":\"$DATA\",\"tabId\":\"$DEST_TAB\"}"
```

---

## Troubleshooting

### "connection refused" / "Cannot connect"

**Problem:** Server not running

**Solution:**
```bash
# Terminal 1: Start server
pinchtab

# Terminal 2: Use client (once server is running)
pinchtab snap
```

### "Port already in use"

**Problem:** Port 9867 is taken

**Solution:**
```bash
# Use different port
BRIDGE_PORT=9868 pinchtab

# Or kill the process using 9867
lsof -i :9867
kill -9 <PID>
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
CHROME_BINARY=/path/to/chrome pinchtab
```

### "Empty snapshot / about:blank"

**Problem:** Tab not navigated yet

**Solution:**
```bash
# Always navigate first
pinchtab nav https://example.com

# Then snapshot
pinchtab snap
```

### "ref e5 not found"

**Problem:** Ref changed (page updated)

**Solution:**
```bash
# Get fresh snapshot
pinchtab snap -i -c

# Use new refs from this snapshot
pinchtab click <new-ref>
```

---

## Next Steps

### 1. Try Some Workflows
→ [Practical examples & workflows](showcase.md)

### 2. Learn the CLI
→ [CLI commands reference](references/cli-commands.md)

### 3. Use the HTTP API
→ [API reference with curl examples](references/curl-commands.md)

### 4. Advanced Usage
→ [API workflows & patterns](showcase.md)

### 5. Production Deployment
→ [Docker deployment guide](guides/docker.md)

### 6. Understand the Architecture
→ [How Pinchtab works](architecture/pinchtab-architecture.md)

---

## Common Features

### Extract Text Efficiently

```bash
# Readable text (strips ads, nav)
pinchtab text              # 800 tokens

# Raw text (full page)
pinchtab text --raw        # ~1,500 tokens

# Filter by selector
pinchtab snap -s "main"    # Only <main> element

# Limit tokens
pinchtab snap --max-tokens 1000
```

### Interact Precisely

```bash
# Get clickable elements only
pinchtab snap -i

# Click by ref
pinchtab click e5

# Type text
pinchtab type e3 "hello"

# Press keys
pinchtab press Enter
pinchtab press Tab
pinchtab press Escape

# Focus element
pinchtab focus e5

# Hover over element
pinchtab hover e5

# Select dropdown
pinchtab select e8 "Option 2"
```

### Run JavaScript

```bash
# Get page title
curl -X POST http://localhost:9867/execute \
  -d '{"expression":"document.title"}'

# Count links
curl -X POST http://localhost:9867/execute \
  -d '{"expression":"document.querySelectorAll(\"a\").length"}'

# Complex logic
curl -X POST http://localhost:9867/execute \
  -d '{"expression":"JSON.stringify({title: document.title, url: location.href})"}'
```

---

## Performance Tips

### Token Efficiency

```bash
# Use text, not screenshot
pinchtab text                    # 800 tokens
pinchtab screenshot              # 10,000+ tokens

# Filter to interactive elements only
pinchtab snap -i -c              # ~1,000 tokens
pinchtab snap                    # ~3,600 tokens

# Limit by tokens
pinchtab snap --max-tokens 1000
```

### Speed

```bash
# Parallel tabs for concurrent work
pinchtab tabs new https://...
pinchtab tabs new https://...
# Process multiple tabs in parallel

# Cache responses when possible
# Snapshot once, extract refs, then interact
pinchtab snap > snapshot.json
# Use refs from snapshot.json multiple times
```

---

## Quick Reference

| Task | Command |
|------|---------|
| Start server | `pinchtab` |
| Health check | `curl http://localhost:9867/health` |
| Navigate | `pinchtab nav https://...` |
| See structure | `pinchtab snap -i -c` |
| Get text | `pinchtab text` |
| Click element | `pinchtab click e5` |
| Type text | `pinchtab type e3 "hello"` |
| Screenshot | `pinchtab screenshot -o page.jpg` |
| PDF export | `pinchtab pdf -o out.pdf` |
| List tabs | `pinchtab tabs` |
| New tab | `pinchtab tabs new https://...` |
| Show help | `pinchtab help` |

---

## Getting Help

- **Issues** → [GitHub Issues](https://github.com/pinchtab/pinchtab/issues)
- **Q&A** → [GitHub Discussions](https://github.com/pinchtab/pinchtab/discussions)
- **Docs** → [Full documentation](overview.md)

---

## What's Next?

1. ✅ You've installed and run Pinchtab
2. 📚 Learn more → [Full documentation](overview.md)
3. 🎯 Try workflows → [Practical examples](showcase.md)
4. 🔧 Go deeper → [Architecture docs](architecture/)

**Happy automating!** 🦀
