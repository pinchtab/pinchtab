# Pinchtab API Reference

Complete HTTP API reference for Pinchtab instances, tabs, and browser operations.

## Overview

Pinchtab uses an **instance-scoped** REST API where:
- Each **instance** is an independent browser process on a specific port
- Each instance can use a different **profile** (Chrome user data directory)
- Each instance can be **headed** (visible window) or **headless** (background)
- Instances manage **tabs** (browser tabs/pages)
- **Chrome starts lazily** on the first request that needs it

```
┌─────────────────────────────────────────┐
│   Pinchtab Dashboard (port 9870)        │
│   Orchestrator - Manages instances      │
└─────────────────────────────────────────┘
           │
           ├─ Instance 1 (port 9868, headed, profile=work)
           │   └─ Chrome browser (visible window)
           │       ├─ Tab 1: linkedin.com
           │       ├─ Tab 2: github.com
           │       └─ Tab 3: gmail.com
           │
           ├─ Instance 2 (port 9869, headless, profile=scraping)
           │   └─ Chrome browser (no window)
           │       ├─ Tab 1: api.example.com
           │       └─ Tab 2: data.example.com
           │
           └─ Instance 3 (port 9870, headless, profile=default)
               └─ Chrome browser (no window)
                   └─ Tab 1: search.example.com
```

## Base URL

```
http://127.0.0.1:9867          # Dashboard (manages instances)
http://127.0.0.1:9868          # Instance 1
http://127.0.0.1:9869          # Instance 2
http://127.0.0.1:9870          # Instance 3
```

## Instance Management API

### Create Instance

**Endpoint:**
```
POST /instances
```

**Request Body:**
```json
{
  "profile": "work",
  "headless": false
}
```

**Parameters:**
- `profile` (string) — Chrome profile name (stored in `~/.pinchtab/profiles/{name}`)
- `headless` (boolean) — `true` for headless mode (no window), `false` for visible window

**Response (201 Created):**
```json
{
  "id": "work-9868",
  "profile": "work",
  "headless": false,
  "status": "running",
  "port": "9868",
  "startTime": "2026-02-28T18:35:18Z",
  "tabs": []
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances \
  -H "Content-Type: application/json" \
  -d '{"profile":"work","headless":false}'
```

---

### List All Instances

**Endpoint:**
```
GET /instances
```

**Response:**
```json
[
  {
    "id": "work-9868",
    "profile": "work",
    "headless": false,
    "status": "running",
    "port": "9868",
    "startTime": "2026-02-28T18:35:18Z",
    "tabs": [
      {"id": "tab-1", "url": "https://linkedin.com", "title": "LinkedIn"},
      {"id": "tab-2", "url": "https://github.com", "title": "GitHub"}
    ]
  },
  {
    "id": "scraping-9869",
    "profile": "scraping",
    "headless": true,
    "status": "running",
    "port": "9869",
    "startTime": "2026-02-28T18:35:20Z",
    "tabs": [
      {"id": "tab-3", "url": "https://api.example.com", "title": "API"}
    ]
  }
]
```

**Example (curl):**
```bash
curl http://localhost:9867/instances
```

---

### Get Instance Details

**Endpoint:**
```
GET /instances/{id}
```

**Response:**
```json
{
  "id": "work-9868",
  "profile": "work",
  "headless": false,
  "status": "running",
  "port": "9868",
  "startTime": "2026-02-28T18:35:18Z",
  "tabs": [
    {"id": "tab-1", "url": "https://linkedin.com", "title": "LinkedIn"},
    {"id": "tab-2", "url": "https://github.com", "title": "GitHub"}
  ]
}
```

**Example (curl):**
```bash
curl http://localhost:9867/instances/work-9868
```

---

### Stop Instance

**Endpoint:**
```
DELETE /instances/{id}
```

**Response (204 No Content):**

**Example (curl):**
```bash
curl -X DELETE http://localhost:9867/instances/work-9868
```

---

## Instance Operations

All operations target a specific instance via its ID or port.

### Navigate (Create Tab + Navigate)

**Endpoint:**
```
POST /instances/{id}/navigate?url=<url>
```

**Query Parameters:**
- `url` (required) — URL to navigate to
- `blockImages` (optional) — `true` to block image loading
- `blockAds` (optional) — `true` to block ad domains

**Response:**
```json
{
  "tabId": "tab-1",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

**Notes:**
- **Creates a NEW tab** every time
- **Starts Chrome lazily** on first request if not running
- **Respects instance mode**: Headed instances show window, headless don't
- Returns immediately after navigation starts (doesn't wait for full load)

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/work-9868/navigate?url=https://linkedin.com
```

**Example (bash):**
```bash
TAB_JSON=$(curl -s -X POST http://localhost:9867/instances/work-9868/navigate \
  -d '{"url":"https://example.com"}')
TAB_ID=$(echo $TAB_JSON | jq -r '.tabId')
echo "Navigated to tab: $TAB_ID"
```

---

### Get Instance Tabs

**Endpoint:**
```
GET /instances/{id}/tabs
```

**Response:**
```json
[
  {"id": "tab-1", "url": "https://linkedin.com", "title": "LinkedIn"},
  {"id": "tab-2", "url": "https://github.com", "title": "GitHub"}
]
```

**Example (curl):**
```bash
curl http://localhost:9867/instances/work-9868/tabs
```

---

### Navigate Existing Tab

**Endpoint:**
```
POST /instances/{id}/tabs/{tabId}/navigate?url=<url>
```

**Query Parameters:**
- `url` (required) — URL to navigate to

**Response:**
```json
{
  "tabId": "tab-1",
  "url": "https://linkedin.com/login",
  "title": "LinkedIn Sign In"
}
```

**Notes:**
- Navigates the **existing tab** (reuses cookies, history, etc.)
- Better for workflows that need session continuity

**Example (curl):**
```bash
curl -X POST "http://localhost:9867/instances/work-9868/tabs/tab-1/navigate?url=https://linkedin.com/login"
```

---

### Get Snapshot

**Endpoint:**
```
GET /instances/{id}/snapshot
```

**Query Parameters:**
- `tabId` (optional) — Specific tab to snapshot (defaults to last active)
- `filter` (optional) — `interactive` for buttons/links/inputs only
- `format` (optional) — `compact` or `text`
- `maxTokens` (optional) — Truncate to ~N tokens
- `depth` (optional) — Max tree depth

**Response:**
```json
{
  "elements": [
    {"ref": "e0", "role": "heading", "name": "LinkedIn Sign In"},
    {"ref": "e1", "role": "textbox", "name": "Email or phone"},
    {"ref": "e2", "role": "textbox", "name": "Password"},
    {"ref": "e3", "role": "button", "name": "Sign in"}
  ]
}
```

**Example (curl):**
```bash
curl "http://localhost:9867/instances/work-9868/snapshot?filter=interactive&format=compact"
```

---

### Click Element

**Endpoint:**
```
POST /instances/{id}/action?kind=click&ref=<ref>&tabId=<tabId>
```

**Query Parameters:**
- `kind` (required) — `click`
- `ref` (required) — Element ref from snapshot
- `tabId` (optional) — Target tab

**Response:**
```json
{"success": true}
```

**Example (curl):**
```bash
curl -X POST "http://localhost:9867/instances/work-9868/action?kind=click&ref=e3&tabId=tab-1"
```

---

### Type Text

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "type",
  "ref": "e1",
  "text": "user@example.com",
  "tabId": "tab-1"
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/work-9868/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e1","text":"user@example.com","tabId":"tab-1"}'
```

---

### Fill Input (Direct)

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "fill",
  "ref": "e1",
  "text": "value",
  "tabId": "tab-1"
}
```

Sets input value directly without triggering key events.

---

### Press Key

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "press",
  "key": "Enter",
  "tabId": "tab-1"
}
```

**Keys:** `Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, etc.

---

### Hover Element

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "hover",
  "ref": "e5",
  "tabId": "tab-1"
}
```

---

### Focus Element

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "focus",
  "ref": "e5",
  "tabId": "tab-1"
}
```

---

### Scroll

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body (scroll to element):**
```json
{
  "kind": "scroll",
  "ref": "e5",
  "tabId": "tab-1"
}
```

**Request Body (scroll by pixels):**
```json
{
  "kind": "scroll",
  "pixels": 500,
  "tabId": "tab-1"
}
```

---

### Select Dropdown

**Endpoint:**
```
POST /instances/{id}/action
```

**Request Body:**
```json
{
  "kind": "select",
  "ref": "e7",
  "value": "Option 2",
  "tabId": "tab-1"
}
```

---

### Extract Text

**Endpoint:**
```
GET /instances/{id}/text
```

**Query Parameters:**
- `tabId` (optional) — Target tab
- `mode` (optional) — `raw` for raw innerText, default for readability extraction

**Response:**
```json
{
  "text": "Example Domain\nThis domain is for use in examples...",
  "length": 234
}
```

**Example (curl):**
```bash
curl "http://localhost:9867/instances/work-9868/text?tabId=tab-1&mode=raw"
```

---

### Execute JavaScript

**Endpoint:**
```
POST /instances/{id}/execute
```

**Request Body:**
```json
{
  "expression": "document.title",
  "tabId": "tab-1"
}
```

**Response:**
```json
{
  "result": "Example Domain"
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/work-9868/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title","tabId":"tab-1"}'
```

---

### Take Screenshot

**Endpoint:**
```
GET /instances/{id}/screenshot
```

**Query Parameters:**
- `tabId` (optional) — Target tab
- `quality` (optional) — JPEG quality 0-100 (default: 90)

**Response (image/jpeg):**
```
[Binary JPEG data]
```

**Example (curl):**
```bash
curl "http://localhost:9867/instances/work-9868/screenshot?tabId=tab-1&quality=85" \
  -o screenshot.jpg
```

---

### Export PDF

**Endpoint:**
```
GET /instances/{id}/pdf
```

**Query Parameters:**
- `tabId` (optional) — Target tab
- `landscape` (optional) — `true` for landscape
- `paperWidth`, `paperHeight` (optional) — Paper dimensions in inches
- `marginTop`, `marginBottom`, `marginLeft`, `marginRight` (optional) — Margins in inches
- `scale` (optional) — Print scale 0.1-2.0
- `pageRanges` (optional) — Pages (e.g., "1-3,5")
- `displayHeaderFooter` (optional) — `true` to show header/footer
- `headerTemplate`, `footerTemplate` (optional) — HTML templates
- `generateTaggedPDF` (optional) — `true` for accessible PDF
- `generateDocumentOutline` (optional) — `true` for document outline
- `output` (optional) — `json` (base64) or `file` (save to disk)

**Response (application/pdf):**
```
[Binary PDF data]
```

**Example (curl):**
```bash
curl "http://localhost:9867/instances/work-9868/pdf?tabId=tab-1&landscape=true" \
  -o output.pdf
```

---

## Aggregate Endpoints

### Get All Tabs (Across All Instances)

**Endpoint:**
```
GET /tabs
```

**Response:**
```json
[
  {"instanceId": "work-9868", "tabId": "tab-1", "url": "https://linkedin.com", "title": "LinkedIn"},
  {"instanceId": "work-9868", "tabId": "tab-2", "url": "https://github.com", "title": "GitHub"},
  {"instanceId": "scraping-9869", "tabId": "tab-3", "url": "https://api.example.com", "title": "API"}
]
```

**Example (curl):**
```bash
curl http://localhost:9867/tabs
```

---

## Complete Agent Workflow Example

### Scenario: Login to LinkedIn, visit profile, take screenshot

```bash
#!/bin/bash

BASE="http://localhost:9867"

# 1. Create instance (headed mode to see what's happening)
echo "Creating instance..."
INST=$(curl -s -X POST $BASE/instances \
  -H "Content-Type: application/json" \
  -d '{"profile":"linkedin","headless":false}')
INST_ID=$(echo $INST | jq -r '.id')
echo "Instance: $INST_ID"

# 2. Navigate to LinkedIn login (creates first tab)
echo "Navigating to LinkedIn..."
NAV=$(curl -s -X POST "$BASE/instances/$INST_ID/navigate?url=https://linkedin.com/login")
TAB_ID=$(echo $NAV | jq -r '.tabId')
echo "Tab: $TAB_ID"

# 3. Get page structure
echo "Getting page structure..."
SNAP=$(curl -s "$BASE/instances/$INST_ID/snapshot?filter=interactive&tabId=$TAB_ID")
echo $SNAP | jq '.elements[]' | head -5

# 4. Find email input (ref=e1) and type
echo "Entering email..."
curl -s -X POST "$BASE/instances/$INST_ID/action" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"type\",\"ref\":\"e1\",\"text\":\"user@example.com\",\"tabId\":\"$TAB_ID\"}"

# 5. Find password input (ref=e2) and type
echo "Entering password..."
curl -s -X POST "$BASE/instances/$INST_ID/action" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"type\",\"ref\":\"e2\",\"text\":\"password123\",\"tabId\":\"$TAB_ID\"}"

# 6. Find sign-in button (ref=e3) and click
echo "Clicking sign in..."
curl -s -X POST "$BASE/instances/$INST_ID/action" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"click\",\"ref\":\"e3\",\"tabId\":\"$TAB_ID\"}"

# 7. Wait for page load
sleep 3

# 8. Navigate to profile (creates new tab)
echo "Navigating to profile..."
NAV2=$(curl -s -X POST "$BASE/instances/$INST_ID/navigate?url=https://linkedin.com/in/myprofile")
TAB_ID2=$(echo $NAV2 | jq -r '.tabId')
echo "New tab: $TAB_ID2"

# 9. Take screenshot
echo "Taking screenshot..."
curl -s "$BASE/instances/$INST_ID/screenshot?tabId=$TAB_ID2&quality=90" \
  -o profile.jpg
echo "Saved: profile.jpg"

# 10. List all tabs on instance
echo "All tabs on instance:"
curl -s "$BASE/instances/$INST_ID/tabs" | jq '.'
```

---

## Error Responses

### 400 Bad Request
```json
{
  "error": "invalid request body",
  "details": "..."
}
```

### 401 Unauthorized
```json
{
  "error": "authentication required"
}
```

### 404 Not Found
```json
{
  "error": "instance not found",
  "id": "unknown-9868"
}
```

### 500 Server Error
```json
{
  "error": "internal server error",
  "details": "..."
}
```

---

## Authentication

Include Bearer token if server requires auth:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:9867/instances
```

Set via `BRIDGE_TOKEN` env var when starting server:

```bash
BRIDGE_TOKEN=secret_token pinchtab
```

---

## Key Design Principles

1. **Instance-Scoped** - All operations target a specific instance
2. **Lazy Browser Init** - Chrome starts on first request, not at instance creation
3. **Tab Creation on Navigate** - `/navigate` always creates new tabs
4. **Profile & Mode per Instance** - Each instance has its own Chrome profile and headed/headless setting
5. **Stateful Operations** - Cookies, history, and session state persist within an instance
6. **Multi-Agent Safe** - Each agent gets its own instance with isolated state

---

## CLI Equivalents

Most endpoints have CLI shortcuts:

```bash
# Create instance
pinchtab instances                    # List all
pinchtab launch --profile work --headed  # Create

# Navigate & interact
pinchtab nav https://example.com     # Navigate (on default instance)
pinchtab snap                        # Snapshot
pinchtab click e5                    # Click
pinchtab type e1 "text"              # Type

# List tabs
pinchtab tabs                        # All tabs across instances
```

See [CLI Commands Reference](cli-commands.md) for full CLI documentation.
