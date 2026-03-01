# API Documentation Examples

This document shows what documented endpoints look like with structured comments.

## Example 1: Navigate to URL (POST /navigate)

### Description
Navigate a tab to a URL or create a new tab and navigate

### Request

**Endpoint:** `POST /navigate`

**Body Parameters:**
- `tabId` (string, optional) — Tab ID to navigate in. If omitted, creates new tab
- `url` (string, required) — URL to navigate to
- `newTab` (bool, optional, default: false) — Force create new tab
- `waitTitle` (float64, optional, default: 0) — Wait for title change (ms)
- `timeout` (float64, optional, default: 30000) — Timeout for navigation (ms)

### Response

**Success (200):**
```json
{
  "tabId": "tab-abc123",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

**Error (400):**
```json
{
  "error": "invalid_url",
  "message": "URL is required"
}
```

### Examples

**CLI:**
```bash
pinchtab nav https://example.com
```

**cURL — Create new tab:**
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

**cURL — Navigate existing tab:**
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","url":"https://google.com"}'
```

---

## Example 2: Get Page Snapshot (GET /snapshot)

### Description
Returns the accessibility tree of a tab with clickable elements, form fields, and text content

### Request

**Endpoint:** `GET /snapshot`

**Query Parameters:**
- `tabId` (string, required) — Tab ID
- `filter` (string, optional, default: "all") — Filter type:
  - `"all"` — All elements
  - `"interactive"` — Only clickable/inputs
- `interactive` (bool, optional) — Alias for filter=interactive
- `compact` (bool, optional, default: false) — Compact output (shorter ref names)
- `depth` (int, optional, default: -1) — Max nesting depth (full tree)
- `text` (bool, optional, default: true) — Include text content
- `format` (string, optional, default: "json") — Output format: "json" or "yaml"
- `diff` (bool, optional, default: false) — Include diff with previous snapshot

### Response

**Success (200):**
```json
{
  "title": "Example Domain",
  "url": "https://example.com",
  "elements": [
    {
      "ref": "e0",
      "role": "heading",
      "name": "Example Domain",
      "tag": "h1"
    },
    {
      "ref": "e1",
      "role": "paragraph",
      "name": "This domain is for use in examples...",
      "tag": "p"
    },
    {
      "ref": "e5",
      "role": "link",
      "name": "More information",
      "tag": "a",
      "href": "https://www.iana.org/domains/example"
    }
  ]
}
```

### Examples

**CLI — Interactive only:**
```bash
pinchtab snap -i -c
```

**cURL — All elements:**
```bash
curl "http://localhost:9867/snapshot?tabId=abc123"
```

**cURL — Interactive elements only:**
```bash
curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive"
```

**cURL — Compact format:**
```bash
curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive&compact=true"
```

**Python:**
```python
import requests

response = requests.get(
    "http://localhost:9867/snapshot",
    params={"tabId": "abc123", "filter": "interactive"}
)
tree = response.json()
for el in tree["elements"]:
    print(f"{el['ref']}: {el['name']}")
```

---

## Example 3: Perform Action (POST /action)

### Description
Interact with page elements: click, type text, fill inputs, press keys, hover, focus, scroll, select

### Request

**Endpoint:** `POST /action`

**Body Parameters:**
- `tabId` (string, required) — Tab ID
- `kind` (string, required) — Action type:
  - `"click"` — Click element
  - `"type"` — Type text (appends to existing)
  - `"fill"` — Fill input (replaces existing)
  - `"press"` — Press key
  - `"hover"` — Hover over element
  - `"focus"` — Focus element
  - `"scroll"` — Scroll
  - `"select"` — Select option
- `ref` (string, required) — Element reference from snapshot (e.g., "e5")
- `text` (string, optional) — Text to type or fill (for "type"/"fill")
- `value` (string, optional) — Value for "select" action
- `key` (string, optional) — Key to press (for "press" action, e.g., "Enter", "Tab")
- `x` (int, optional) — X coordinate for "scroll"
- `y` (int, optional) — Y coordinate for "scroll"

### Response

**Success (200):**
```json
{
  "success": true
}
```

**Error (404):**
```json
{
  "error": "element_not_found",
  "ref": "e5"
}
```

### Examples

**CLI — Click button:**
```bash
pinchtab click e5
```

**CLI — Type into input:**
```bash
pinchtab type e3 "user@example.com"
```

**CLI — Fill form field:**
```bash
pinchtab fill e3 "John Doe"
```

**cURL — Click:**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"click","ref":"e5"}'
```

**cURL — Type into input:**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"type","ref":"e3","text":"user@example.com"}'
```

**cURL — Fill form (replaces existing text):**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"fill","ref":"e3","text":"John Doe"}'
```

**cURL — Press Enter key:**
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"press","ref":"e7","key":"Enter"}'
```

**cURL — Click multiple actions in sequence:**
```bash
# 1. Click login button
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"click","ref":"e10"}'

# 2. Fill email
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"fill","ref":"e3","text":"user@example.com"}'

# 3. Fill password
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"fill","ref":"e5","text":"password123"}'

# 4. Submit form
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","kind":"press","ref":"e7","key":"Enter"}'
```

---

## Complete Workflow Example

### Automated Login + Screenshot

**CLI:**
```bash
# 1. Navigate to login page
pinchtab nav https://example.com/login

# 2. Get page structure
pinchtab snap -i -c

# 3. Fill email
pinchtab fill e3 "user@example.com"

# 4. Fill password
pinchtab fill e5 "password123"

# 5. Click submit
pinchtab click e7

# 6. Wait a moment
sleep 2

# 7. Take screenshot
pinchtab screenshot -o login-success.jpg
```

**Python:**
```python
import requests
import json
import time

BASE = "http://localhost:9867"

# 1. Navigate
r = requests.post(f"{BASE}/navigate", json={"url": "https://example.com/login"})
tab_id = r.json()["tabId"]
print(f"Navigated to login page: {tab_id}")

# 2. Get page structure
r = requests.get(f"{BASE}/snapshot", params={"tabId": tab_id, "filter": "interactive"})
elements = r.json()["elements"]
for el in elements:
    if "email" in el.get("name", "").lower():
        email_ref = el["ref"]
    if "password" in el.get("name", "").lower():
        password_ref = el["ref"]
    if "submit" in el.get("name", "").lower() or "login" in el.get("name", "").lower():
        submit_ref = el["ref"]

# 3. Fill email
requests.post(f"{BASE}/action", json={"tabId": tab_id, "kind": "fill", "ref": email_ref, "text": "user@example.com"})

# 4. Fill password
requests.post(f"{BASE}/action", json={"tabId": tab_id, "kind": "fill", "ref": password_ref, "text": "password123"})

# 5. Submit
requests.post(f"{BASE}/action", json={"tabId": tab_id, "kind": "click", "ref": submit_ref})

# 6. Wait for page to load
time.sleep(2)

# 7. Take screenshot
r = requests.get(f"{BASE}/screenshot", params={"tabId": tab_id})
with open("login-success.jpg", "wb") as f:
    f.write(r.content)
print("Screenshot saved")
```

---

## Notes

- All examples use `http://localhost:9867` as the base URL
- In production, use your actual server address
- Tab IDs are returned from `/navigate` or other tab operations
- Refs come from `/snapshot` responses
- CLI commands require the pinchtab server to be running
