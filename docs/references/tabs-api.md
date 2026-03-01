# Tabs API Reference

Tabs are individual browser windows/pages within an instance. Each tab has its own URL, content, and state. Tabs are the primary resource for browser operations in PinchTab.

## Quick Start

### List All Tabs
```bash
# CLI
pinchtab tabs

# Curl
curl http://localhost:9867/tabs | jq .

# Response
[
  {
    "id": "tab_abc123",
    "instanceId": "inst_xyz",
    "url": "https://example.com",
    "title": "Example Domain",
    "status": "ready"
  }
]
```

### Open a New Tab
```bash
# CLI (create new tab in instance)
pinchtab tab open inst_xyz https://example.com

# Curl
curl -X POST http://localhost:9867/instances/{id}/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Response
{
  "tabId": "tab_abc123",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

### Get Tab Info
```bash
# CLI
pinchtab tab info tab_abc123

# Curl
curl http://localhost:9867/tabs/tab_abc123

# Response
{
  "id": "tab_abc123",
  "instanceId": "inst_xyz",
  "url": "https://example.com",
  "title": "Example Domain",
  "status": "ready"
}
```

### Close Tab
```bash
# CLI
pinchtab tab close tab_abc123

# Curl
curl -X POST http://localhost:9867/tabs/tab_abc123/close

# Response
{
  "id": "tab_abc123",
  "status": "closed"
}
```

---

## Complete API Reference

### 1. Open Tab

**Endpoint:** `POST /instances/{id}/tabs/open`

**CLI:**
```bash
# Create tab in instance with URL
pinchtab tab open inst_xyz https://example.com

# Create tab without initial URL
pinchtab tab open inst_xyz
```

**Curl:**
```bash
# Minimal (instanceId only, no URL)
curl -X POST http://localhost:9867/instances/{id}/tabs/open \
  -H "Content-Type: application/json" \
  -d '{}'

# With URL
curl -X POST http://localhost:9867/instances/{id}/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

**Request Body:**
```json
{
  "url": "https://example.com"
}
```

**Parameters:**
- `url` (string, optional) — URL to navigate to after opening tab

**Response:** Tab object
```json
{
  "tabId": "tab_abc123",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

**Defaults:**
- If `url` not provided, tab opens with blank page
- Status is "ready" immediately after creation

---

### 2. List All Tabs

**Endpoint:** `GET /tabs`

**CLI:**
```bash
pinchtab tabs
```

**Curl:**
```bash
# List all tabs across all instances
curl http://localhost:9867/tabs | jq .

# Filter by instance
curl 'http://localhost:9867/tabs?instanceId=inst_xyz' | jq .
```

**Query Parameters:**
- `instanceId` (string, optional) — Filter tabs to specific instance

**Response:** Array of Tab objects
```json
[
  {
    "id": "tab_abc123",
    "instanceId": "inst_xyz",
    "url": "https://example.com",
    "title": "Example Domain",
    "status": "ready",
    "createdAt": "2026-03-01T05:25:30Z"
  },
  {
    "id": "tab_def456",
    "instanceId": "inst_xyz",
    "url": "https://google.com",
    "title": "Google",
    "status": "ready",
    "createdAt": "2026-03-01T05:26:00Z"
  }
]
```

**Tab Status Values:**
- `ready` — Tab is loaded and ready to accept commands
- `loading` — Tab is navigating to URL
- `error` — Tab failed to load

---

### 3. Get Tab Info

**Endpoint:** `GET /tabs/{id}`

**CLI:**
```bash
pinchtab tab info tab_abc123
```

**Curl:**
```bash
curl http://localhost:9867/tabs/tab_abc123 | jq .
```

**Response:** Single Tab object
```json
{
  "id": "tab_abc123",
  "instanceId": "inst_xyz",
  "url": "https://example.com",
  "title": "Example Domain",
  "status": "ready",
  "createdAt": "2026-03-01T05:25:30Z"
}
```

**Notes:**
- Returns 404 if tab not found
- Provides full tab state information

---

### 4. Close Tab

**Endpoint:** `POST /tabs/{id}/close`

**CLI:**
```bash
pinchtab tab close tab_abc123
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc123/close
```

**Response:**
```json
{
  "id": "tab_abc123",
  "status": "closed"
}
```

**Notes:**
- Gracefully closes tab
- Tab removed from instance
- Returns 404 if tab already closed/not found

---

## Complete Workflow Examples

### Example 1: Multi-Tab Workflow

```bash
#!/bin/bash

# Start instance
INST=$(pinchtab instance start --mode headed | jq -r .id)
echo "Instance: $INST"

# Open first tab
TAB1=$(pinchtab tab open $INST https://example.com | jq -r .id)
echo "Tab 1: $TAB1"

# Open second tab
TAB2=$(pinchtab tab open $INST https://google.com | jq -r .id)
echo "Tab 2: $TAB2"

# List all tabs in instance
echo -e "\nTabs in instance:"
curl -s "http://localhost:9867/tabs?instanceId=$INST" | jq '.[] | {id, url}'

# Get info on tab 1
echo -e "\nTab 1 info:"
pinchtab tab info $TAB1

# Close tab 1
echo -e "\nClosing tab 1..."
pinchtab tab close $TAB1

# Verify closed
echo "Remaining tabs:"
curl -s "http://localhost:9867/tabs?instanceId=$INST" | jq 'length' | xargs echo "Count:"

# Cleanup
pinchtab instance stop $INST
```

### Example 2: Batch Tab Operations

```bash
#!/bin/bash

INST=$1
URLS=(
  "https://example.com"
  "https://google.com"
  "https://github.com"
  "https://stackoverflow.com"
)

echo "Opening $((${#URLS[@]})) tabs in instance $INST..."

TABS=()
for url in "${URLS[@]}"; do
  TAB=$(curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
    -d "{\"url\":\"$url\"}" | jq -r .tabId)
  TABS+=($TAB)
  echo "Opened: $TAB"
done

# Wait for loading
sleep 3

# Show all tabs
echo -e "\nAll tabs:"
curl -s "http://localhost:9867/tabs?instanceId=$INST" | jq '.[] | {id, title}'

# Close all
echo -e "\nClosing all tabs..."
for tab in "${TABS[@]}"; do
  curl -s -X POST http://localhost:9867/tabs/$tab/close > /dev/null
  echo "Closed: $tab"
done
```

### Example 3: Sequential Tab Navigation

```bash
#!/bin/bash

# Start instance with specific profile
PROF="my-profile"
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -d "{\"profileId\":\"$PROF\"}" | jq -r .id)

echo "Started: $INST"

# Create single tab (will be used for all navigation)
TAB=$(pinchtab tab open $INST | jq -r .id)
echo "Tab: $TAB"

# Navigate through multiple sites
SITES=("example.com" "google.com" "github.com")

for site in "${SITES[@]}"; do
  echo "Navigating to $site..."
  curl -s -X POST http://localhost:9867/tabs/$TAB/navigate \
    -d "{\"url\":\"https://$site\"}" > /dev/null
  sleep 2
done

# Cleanup
pinchtab tab close $TAB
pinchtab instance stop $INST
```

---

## CLI Examples

### Quick Start with CLI

```bash
# List instances first
pinchtab instances

# Open tab in instance
pinchtab tab open inst_abc123 https://example.com

# List all tabs
pinchtab tabs

# List tabs in specific instance
pinchtab tabs inst_abc123

# Get tab info
pinchtab tab info tab_xyz789

# Close tab
pinchtab tab close tab_xyz789

# Open multiple tabs
for url in example.com google.com github.com; do
  pinchtab tab open inst_abc123 https://$url
done
```

---

## Integration Examples

### With Bash

```bash
# Get instance, create tabs, list them
INST=$(pinchtab instance start --mode headed | jq -r .id)

TAB1=$(curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
  -d "{\"url\":\"https://example.com\"}" | jq -r .tabId)

TAB2=$(curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
  -d "{\"url\":\"https://google.com\"}" | jq -r .tabId)

# List tabs
curl -s http://localhost:9867/tabs | jq '.[] | {id, url}'
```

### With Python

```python
import requests
import json

BASE = "http://localhost:9867"

# Start instance
inst_resp = requests.post(f"{BASE}/instances/start", json={"mode": "headed"})
inst_id = inst_resp.json()["id"]
print(f"Instance: {inst_id}")

# Open tabs
for url in ["https://example.com", "https://google.com"]:
  resp = requests.post(f"{BASE}/instances/{inst_id}/tabs/open",
    json={"url": url})
  tab = resp.json()
  print(f"Tab: {tab['tabId']} → {url}")

# List tabs
tabs = requests.get(f"{BASE}/tabs").json()
print(f"\nTotal tabs: {len(tabs)}")
for tab in tabs:
  print(f"  {tab['id']}: {tab['url']}")

# Close first tab
first_tab = tabs[0]["id"]
requests.post(f"{BASE}/tabs/{first_tab}/close")
print(f"\nClosed: {first_tab}")
```

### With JavaScript/Node.js

```javascript
const BASE = "http://localhost:9867";

async function tabWorkflow() {
  // Start instance
  const instResp = await fetch(`${BASE}/instances/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode: "headed" })
  });
  const inst = await instResp.json();
  console.log(`Instance: ${inst.id}`);

  // Open tabs
  const urls = ["https://example.com", "https://google.com"];
  const tabs = [];

  for (const url of urls) {
    const resp = await fetch(`${BASE}/instances/${inst.id}/tabs/open`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url })
    });
    const tab = await resp.json();
    tabs.push(tab);
    console.log(`Tab: ${tab.tabId} → ${url}`);
  }

  // List all tabs
  const listResp = await fetch(`${BASE}/tabs`);
  const allTabs = await listResp.json();
  console.log(`\nTotal tabs: ${allTabs.length}`);

  // Close first tab
  const closeResp = await fetch(`${BASE}/tabs/${tabs[0].id}/close`, {
    method: "POST"
  });
  console.log(`Closed: ${tabs[0].id}`);
}

tabWorkflow().catch(console.error);
```

---

## Error Handling

### Instance Not Found (404)

```bash
curl -X POST http://localhost:9867/instances/{id}/tabs/open \
  -d '{"url":"https://example.com"}'

# Response (404)
{
  "error": "instance not found",
  "statusCode": 404
}
```

### Tab Not Found (404)

```bash
curl http://localhost:9867/tabs/nonexistent

# Response (404)
{
  "error": "tab not found",
  "statusCode": 404
}
```

### Invalid JSON (400)

```bash
curl -X POST http://localhost:9867/instances/{id}/tabs/open -d 'invalid'

# Response (400)
{
  "error": "invalid JSON",
  "statusCode": 400
}
```

---

## Best Practices

### Tab Lifecycle

```bash
# 1. Open tab
TAB=$(pinchtab tab open $INST | jq -r .id)

# 2. Do work (navigate, interact, etc.)
pinchtab tab navigate $TAB https://example.com

# 3. Get results
pinchtab tab snapshot $TAB

# 4. Close tab
pinchtab tab close $TAB
```

### Resource Management

```bash
# Always close tabs when done
for tab_id in $(pinchtab tabs | jq -r '.[].id'); do
  pinchtab tab close $tab_id
done

# Stop instances
pinchtab instance stop $INSTANCE_ID
```

### Error Handling

```bash
# Check if tab exists before operating
if ! pinchtab tab info $TAB_ID > /dev/null 2>&1; then
  echo "Tab not found: $TAB_ID"
  exit 1
fi

# Handle navigation errors
if ! curl -s -X POST http://localhost:9867/tabs/$TAB_ID/navigate \
  -d "{\"url\":\"$url\"}" | jq -e .error > /dev/null; then
  echo "Navigation failed"
fi
```

---

## Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| **200** | Success (GET) | Tab info retrieved |
| **201** | Created | Tab opened |
| **204** | No content | Tab closed |
| **400** | Bad request | Invalid JSON, missing instanceId |
| **404** | Not found | Tab/instance not found |
| **500** | Server error | Internal error |

---

## Tab Lifecycle Diagram

```
OPEN (POST /instances/{id}/tabs/open)
  ↓
[status: "ready"] → Ready for commands
  ↓
Can navigate, interact, snapshot, etc.
  ↓
CLOSE (POST /tabs/{id}/close)
  ↓
[status: "closed"] → Tab removed
```

---

## Summary Table

| Operation | Method | Endpoint | CLI |
|-----------|--------|----------|-----|
| Open | POST | `/instances/{id}/tabs/open` | `pinchtab tab open <inst> [url]` |
| List All | GET | `/tabs` | `pinchtab tabs` |
| Get Info | GET | `/tabs/{id}` | `pinchtab tab info <id>` |
| Close | POST | `/tabs/{id}/close` | `pinchtab tab close <id>` |

---

## FAQ

**Q: How many tabs can I open in an instance?**
A: Technically unlimited, limited only by system resources (memory).

**Q: What happens when I navigate to a new URL in a tab?**
A: URL loads in the same tab. Use a new tab if you want to keep the old page.

**Q: Can I close all tabs at once?**
A: No, close individually. You can script it with a loop.

**Q: What happens to tabs when I stop an instance?**
A: All tabs are closed automatically when instance stops.

**Q: How do I know when a tab is ready after opening?**
A: Check status via GET /tabs/{id} or GET /tabs?instanceId=...

**Q: Can I reopen a closed tab?**
A: No, create a new tab instead.

---

---

## Tab Operations (Phase 4)

Once you have a tab ID, you can perform full browser control operations on it.

### Navigate Tab

**Endpoint:** `POST /tabs/{id}/navigate`

**CLI:**
```bash
pinchtab tab navigate <tab-id> https://example.com
pinchtab tab navigate <tab-id> https://example.com --timeout 30
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc/navigate \
  -d '{"url":"https://example.com","timeout":30,"blockImages":true}'
```

### Get Snapshot

**Endpoint:** `GET /tabs/{id}/snapshot`

**CLI:**
```bash
pinchtab tab snapshot <tab-id>
pinchtab tab snapshot <tab-id> --interactive --compact
pinchtab tab snapshot <tab-id> -i -c
```

**Curl:**
```bash
curl 'http://localhost:9867/tabs/tab_abc/snapshot?interactive=true&compact=true'
```

### Take Screenshot

**Endpoint:** `GET /tabs/{id}/screenshot`

**CLI:**
```bash
pinchtab tab screenshot <tab-id> -o screenshot.png
pinchtab tab screenshot <tab-id> -o out.jpg -q 85
```

**Curl:**
```bash
curl http://localhost:9867/tabs/tab_abc/screenshot > screenshot.png
```

### Execute Action

**Endpoint:** `POST /tabs/{id}/action`

**CLI:**
```bash
# Click element
pinchtab tab click <tab-id> e5

# Type text
pinchtab tab type <tab-id> e12 "hello world"

# Press key
pinchtab tab press <tab-id> Enter

# Fill input
pinchtab tab fill <tab-id> e12 "value"

# Hover element
pinchtab tab hover <tab-id> e5

# Scroll
pinchtab tab scroll <tab-id> down

# Select dropdown
pinchtab tab select <tab-id> e3 "option2"

# Focus element
pinchtab tab focus <tab-id> e5
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc/action \
  -d '{"kind":"click","ref":"e5"}'
```

### Execute Multiple Actions

**Endpoint:** `POST /tabs/{id}/actions`

**CLI:**
```bash
# Piped JSON
cat actions.json | pinchtab tab actions <tab-id>

# Inline JSON
pinchtab tab actions <tab-id> --json '[{"kind":"click","ref":"e5"},...]'
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc/actions \
  -d '{
    "actions": [
      {"kind":"click","ref":"e5"},
      {"kind":"type","ref":"e12","text":"search"},
      {"kind":"press","key":"Enter"}
    ]
  }'
```

### Get Page Text

**Endpoint:** `GET /tabs/{id}/text`

**CLI:**
```bash
pinchtab tab text <tab-id>
pinchtab tab text <tab-id> --raw
```

**Curl:**
```bash
curl http://localhost:9867/tabs/tab_abc/text
```

### Evaluate JavaScript

**Endpoint:** `POST /tabs/{id}/evaluate`

**CLI:**
```bash
pinchtab tab eval <tab-id> "document.title"
pinchtab tab eval <tab-id> "document.querySelectorAll('a').length"
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc/evaluate \
  -d '{"expression":"document.title"}'
```

### Export to PDF

**Endpoint:** `GET /tabs/{id}/pdf`

**CLI:**
```bash
pinchtab tab pdf <tab-id> -o output.pdf
pinchtab tab pdf <tab-id> -o out.pdf --landscape
```

**Curl:**
```bash
curl 'http://localhost:9867/tabs/tab_abc/pdf?landscape=true' > output.pdf
```

### Manage Cookies

**Endpoint:** `GET /tabs/{id}/cookies` and `POST /tabs/{id}/cookies`

**CLI:**
```bash
pinchtab tab cookies <tab-id>  # Get cookies
```

**Curl:**
```bash
curl http://localhost:9867/tabs/tab_abc/cookies
```

### Lock Tab

**Endpoint:** `POST /tabs/{id}/lock`

**CLI:**
```bash
pinchtab tab lock <tab-id> --owner my-agent --ttl 60
```

**Curl:**
```bash
curl -X POST http://localhost:9867/tabs/tab_abc/lock \
  -d '{"owner":"my-agent","ttl":60}'
```

### Unlock Tab

**Endpoint:** `POST /tabs/{id}/unlock`

**CLI:**
```bash
pinchtab tab unlock <tab-id> --owner my-agent
```

---

## Complete Tab Operation Example

```bash
#!/bin/bash

# Start instance
INST=$(pinchtab instance start --mode headed)
echo "Instance: $INST"

# Create tab
TAB=$(pinchtab tab new $INST | jq -r .id)
echo "Tab: $TAB"

# Navigate
pinchtab tab navigate $TAB https://example.com
sleep 2

# Get snapshot
pinchtab tab snapshot $TAB -i -c | jq .

# Click element
pinchtab tab click $TAB e5
sleep 1

# Get result
pinchtab tab snapshot $TAB -d

# Extract text
pinchtab tab text $TAB | jq .text

# Take screenshot
pinchtab tab screenshot $TAB -o result.png

# Export PDF
pinchtab tab pdf $TAB -o result.pdf

# Cleanup
pinchtab tab close $TAB
pinchtab instance stop $INST
```

---

## Related Documentation

- **Instance API** (docs/references/instance-api.md) — Manage instances
- **Profile API** (docs/references/profile-api.md) — Manage profiles
- **CLI Design** (docs/references/cli-design.md) — CLI command patterns
