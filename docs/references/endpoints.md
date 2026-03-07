# API Reference

Complete HTTP API reference for instances, tabs, and browser operations.

## Overview

Pinchtab uses a **server-first** API model:
- the **Pinchtab server** listens on port `9867` and manages profiles and instances
- each managed **instance** is backed by a single `pinchtab bridge` runtime
- tab operations are tab-first under `/tabs/{id}/...`
- instance lifecycle operations live under `/instances/...`
- attach is the advanced path for registering externally managed Chrome

```text
HTTP Client
    │
    │ All requests: http://localhost:9867
    │
    ↓
┌──────────────────────────────────────────┐
│   PinchTab Server (HTTP, port 9867)      │
│   ┌────────────────────────────────────┐ │
│   │ - Dashboard                         │ │
│   │ - Instance management               │ │
│   │ - Profile management                │ │
│   │ - Routing to bridge runtimes        │ │
│   └────────────────────────────────────┘ │
└─────┬──────────────────────────────────────┘
      │
      │ HTTP proxying to managed instances
      │
      ├─────────────────────────────────────┐
      │                                     │
      ↓                                     ↓
┌──────────────────┐        ┌──────────────────┐
│ Bridge Runtime 1 │        │ Bridge Runtime 2 │
│ + Chrome         │        │ + Chrome         │
│ profile: work    │        │ profile: scraping│
├──────────────────┤        ├──────────────────┤
│ Tab 1: LinkedIn  │        │ Tab 1: API       │
│ Tab 2: GitHub    │        │ Tab 2: Data      │
│ Tab 3: Email     │        └──────────────────┘
└──────────────────┘
```

## API Summary

| Category | Operations | Purpose |
|---|---|---|
| **Instance Management** | `POST /instances/start` `POST /instances/launch` `POST /instances/attach` `GET /instances` `GET /instances/{id}` `POST /instances/{id}/stop` | Create, attach, list, inspect, and stop instances |
| **Navigation** | `POST /tabs/{id}/navigate` | Navigate an existing tab by tab ID |
| **Tab Operations** | `POST /instances/{id}/tabs/open` `GET /instances/{id}/tabs` `POST /tabs/{tabId}/navigate` | Open/list tabs and navigate existing tab |
| **Page Inspection** | `GET /tabs/{id}/snapshot` `GET /tabs/{id}/text` | Get accessibility tree, extract text |
| **User Actions** | `POST /tabs/{id}/action` `POST /tabs/{id}/actions` | Click, type, fill, press, hover, focus, scroll, select |
| **JavaScript** | `POST /tabs/{id}/evaluate` | Execute JavaScript in the page |
| **Visual** | `GET /tabs/{id}/screenshot` `GET /tabs/{id}/pdf` | Take screenshot, export PDF |
| **Profiles** | `GET /profiles` `POST /profiles` | List and create browser profiles |
| **Aggregate** | `GET /tabs` | Get all tabs across all instances |

---

## Base URL

```
http://127.0.0.1:9867          # PinchTab Server (all endpoints)
```

All requests go to port 9867. Instance-scoped proxy operations use `/instances/{id}/...`, while tab-first operations use `/tabs/{tabId}/...` (for example `POST /tabs/{tabId}/navigate` and `GET /tabs/{tabId}/screenshot`).

---

## Instance Management API

### Launch or Start Instance

**Endpoint:**
```
POST /instances/start
POST /instances/launch
```

**Request Body:**
```json
{
  "profileId": "work",
  "mode": "headed"
}
```

**Parameters:**
- `profileId` (string, optional) - profile ID or name
- `name` (string, optional on `/instances/launch`) - instance/profile name when launching by name
- `mode` (string, optional) - `headless` or `headed`
- `port` (string, optional) - explicit port if you do not want auto-allocation

**Behavior:**
- Creates a managed instance record on the server
- Spawns a `pinchtab bridge` child process for managed instances
- Chrome may initialize lazily on the first browser request
- Returns instance metadata used for later tab operations

**Response (201 Created):**
```json
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "work",
  "headless": false,
  "status": "starting",
  "port": "9868",
  "startTime": "2026-02-28T18:35:18Z"
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","mode":"headed"}'
```

### Attach Instance

**Endpoint:**
```
POST /instances/attach
```

**Request Body:**
```json
{
  "name": "shared-chrome",
  "cdpUrl": "ws://127.0.0.1:9222/devtools/browser/..."
}
```

Attach is only available when enabled in config under `attach.enabled`, and the target must pass `allowHosts` and `allowSchemes` policy checks.

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
      {"id": "tab-3", "url": "https://api.pinchtab.com", "title": "API"}
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
POST /instances/{id}/stop
```

**Response:**
```json
{
  "id": "inst_0a89a5bb",
  "status": "stopped"
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/stop
```

---

## Instance Operations

All operations target a specific instance via its ID or port.

### Open Tab (Create Tab)

**Endpoint:**
```
POST /instances/{id}/tabs/open
```

**Request Body:**
- `url` (optional) — URL to open immediately in the new tab

**Response:**
```json
{
  "tabId": "tab-1",
  "url": "https://pinchtab.com",
  "title": "Example Domain"
}
```

**Notes:**
- **Creates a NEW tab** every time
- If `url` is provided, the new tab opens that URL
- Use `POST /tabs/{tabId}/navigate` to navigate an existing tab later

**Example (curl):**
```bash
curl -X POST http://localhost:9867/instances/work-9868/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://linkedin.com"}'
```

**Example (bash):**
```bash
TAB_JSON=$(curl -s -X POST http://localhost:9867/instances/work-9868/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}')
TAB_ID=$(echo $TAB_JSON | jq -r '.tabId')
echo "Opened tab: $TAB_ID"
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
POST /tabs/{tabId}/navigate?url=<url>
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
curl -X POST "http://localhost:9867/tabs/tab-1/navigate?url=https://linkedin.com/login"
```

---

### Get Snapshot

**Endpoint:**
```
GET /tabs/{tabId}/snapshot
```

**Query Parameters:**
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
curl "http://localhost:9867/tabs/tab-1/snapshot?filter=interactive&format=compact"
```

---

### Click Element

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "click",
  "ref": "e3"
}
```

**Response:**
```json
{"success": true}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/tabs/tab-1/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e3"}'
```

---

### Type Text

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "type",
  "ref": "e1",
  "text": "user@pinchtab.com"
}
```

**Example (curl):**
```bash
curl -X POST http://localhost:9867/tabs/tab-1/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e1","text":"user@pinchtab.com"}'
```

---

### Fill Input (Direct)

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "fill",
  "ref": "e1",
  "text": "value"
}
```

Sets input value directly without triggering key events.

---

### Press Key

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "press",
  "key": "Enter"
}
```

**Keys:** `Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, etc.

---

### Hover Element

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "hover",
  "ref": "e5"
}
```

---

### Focus Element

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "focus",
  "ref": "e5"
}
```

---

### Scroll

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body (scroll to element):**
```json
{
  "kind": "scroll",
  "ref": "e5"
}
```

**Request Body (scroll by pixels):**
```json
{
  "kind": "scroll",
  "pixels": 500
}
```

---

### Select Dropdown

**Endpoint:**
```
POST /tabs/{id}/action
```

**Request Body:**
```json
{
  "kind": "select",
  "ref": "e7",
  "value": "Option 2"
}
```

---

### Extract Text

**Endpoint:**
```
GET /tabs/{id}/text
```

**Query Parameters:**
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
curl "http://localhost:9867/tabs/tab-1/text?mode=raw"
```

---

### Execute JavaScript

**Endpoint:**
```
POST /tabs/{id}/evaluate
```

**Request Body:**
```json
{
  "expression": "document.title"
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
curl -X POST http://localhost:9867/tabs/tab-1/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'
```

---

### Take Screenshot

**Endpoint:**
```
GET /tabs/{id}/screenshot
```

**Query Parameters:**
- `quality` (optional) — JPEG quality 0-100 (default: 90)

**Response (image/jpeg):**
```
[Binary JPEG data]
```

**Example (curl):**
```bash
curl "http://localhost:9867/tabs/tab-1/screenshot?quality=85" \
  -o screenshot.jpg
```

---

### Export PDF

**Endpoint:**
```
GET /tabs/{id}/pdf
```

**Query Parameters:**
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
curl "http://localhost:9867/tabs/tab-1/pdf?landscape=true" \
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
  {"instanceId": "scraping-9869", "tabId": "tab-3", "url": "https://api.pinchtab.com", "title": "API"}
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
# This STARTS Chrome immediately with visible window
echo "Creating instance..."
INST=$(curl -s -X POST $BASE/instances \
  -H "Content-Type: application/json" \
  -d '{"profile":"linkedin","headless":false}')
INST_ID=$(echo $INST | jq -r '.id')
echo "Instance: $INST_ID (Chrome now running and visible)"

# 2. Navigate to LinkedIn login (creates first tab)
# Chrome is already running, navigation is fast
echo "Navigating to LinkedIn..."
NAV=$(curl -s -X POST "$BASE/instances/$INST_ID/tabs/open" \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://linkedin.com/login"}')
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
  -d "{\"kind\":\"type\",\"ref\":\"e1\",\"text\":\"user@pinchtab.com\",\"tabId\":\"$TAB_ID\"}"

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
NAV2=$(curl -s -X POST "$BASE/instances/$INST_ID/tabs/open" \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://linkedin.com/in/myprofile"}')
TAB_ID2=$(echo $NAV2 | jq -r '.tabId')
echo "New tab: $TAB_ID2"

# 9. Take screenshot
echo "Taking screenshot..."
curl -s "$BASE/tabs/$TAB_ID2/screenshot?quality=90" \
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

Set via `PINCHTAB_TOKEN` when starting the server:

```bash
PINCHTAB_TOKEN=secret_token pinchtab
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
pinchtab nav https://pinchtab.com     # Navigate (on default instance)
pinchtab snap                        # Snapshot
pinchtab click e5                    # Click
pinchtab type e1 "text"              # Type

# List tabs
pinchtab tabs                        # All tabs across instances
```

See [CLI Commands Reference](cli-commands.md) for full CLI documentation.
