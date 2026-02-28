# Pinchtab Curl/API Commands

Complete HTTP API reference for Pinchtab. Use `curl` or any HTTP client to interact with the API directly.

## Quick Start

**Terminal 1: Start server**
```bash
pinchtab
```

**Terminal 2: Make API calls**
```bash
curl http://localhost:9867/health
curl -X POST http://localhost:9867/navigate -H "Content-Type: application/json" -d '{"url":"https://example.com"}'
curl http://localhost:9867/snapshot
```

## Base URL

```
http://127.0.0.1:9867
http://localhost:9867
```

With authentication:
```bash
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:9867/health
```

---

## Health & Status

### Health Check
```bash
curl http://localhost:9867/health
```

**Response:**
```json
{
  "status": "ok",
  "version": "0.7.6",
  "uptime": 3600
}
```

---

## Navigation

### Navigate to URL
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

**Request body:**
```json
{
  "url": "https://example.com",
  "newTab": false,
  "blockImages": false,
  "blockAds": false
}
```

**Query parameters:**
- `url` (required) — URL to navigate to
- `newTab` — Open in new tab (default: false)
- `blockImages` — Don't load images (default: false)
- `blockAds` — Block ad domains (default: false)

**Response:**
```json
{
  "title": "Example Domain",
  "url": "https://example.com",
  "tabId": "abc123",
  "targetId": "target-1"
}
```

**Examples:**
```bash
# Navigate to URL
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://google.com"}'

# Open in new tab
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com","newTab":true}'

# Block ads and images
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","blockAds":true,"blockImages":true}'
```

---

## Snapshots & DOM

### Get Accessibility Tree
```bash
curl http://localhost:9867/snapshot
```

**Query parameters:**
- `filter` — `interactive` (buttons, links, inputs only)
- `format` — `compact` or `text` (default: full JSON)
- `diff` — `true` (only changes since last snapshot)
- `selector` — CSS selector to scope to
- `maxTokens` — Truncate to ~N tokens
- `depth` — Max tree depth (default: unlimited)
- `tabId` — Target specific tab

**Examples:**
```bash
# Full snapshot
curl http://localhost:9867/snapshot

# Interactive elements only (most token-efficient)
curl http://localhost:9867/snapshot?filter=interactive

# Compact format
curl http://localhost:9867/snapshot?format=compact

# Compact + interactive
curl http://localhost:9867/snapshot?filter=interactive&format=compact

# Diff only (changes since last snapshot)
curl http://localhost:9867/snapshot?diff=true

# Scope to CSS selector
curl http://localhost:9867/snapshot?selector=main%20form

# Limit tokens
curl http://localhost:9867/snapshot?maxTokens=1000

# Specific tab
curl http://localhost:9867/snapshot?tabId=abc123

# Combine filters
curl "http://localhost:9867/snapshot?filter=interactive&format=compact&maxTokens=500"
```

**Response:**
```json
{
  "elements": [
    {
      "ref": "e0",
      "role": "heading",
      "name": "Example Domain",
      "visible": true
    },
    {
      "ref": "e1",
      "role": "link",
      "name": "More information...",
      "href": "https://www.iana.org/domains/example"
    }
  ]
}
```

---

## Interaction

### Click Element
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
```

**Query parameters:**
- `tabId` — Target specific tab

**Examples:**
```bash
# Click element
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Click on specific tab
curl -X POST "http://localhost:9867/action?tabId=abc123" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
```

### Type Text
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e12","text":"hello world"}'
```

**Examples:**
```bash
# Type into input
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e3","text":"user@example.com"}'

# Type with special characters (JSON escaped)
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e5","text":"Hello \"World\"!"}'
```

### Fill Input (Direct)
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"value"}'
```

Sets input value directly without triggering key events.

**Examples:**
```bash
# Fill input by ref
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e5","text":"hello"}'

# Fill by selector
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"input[name=email]","text":"user@example.com"}'
```

### Press Key
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}'
```

**Keys:** `Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, etc.

**Examples:**
```bash
# Press Enter
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Enter"}'

# Press Escape
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Escape"}'

# Press Tab
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"press","key":"Tab"}'
```

### Hover Element
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"hover","ref":"e5"}'
```

### Focus Element
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"focus","ref":"e5"}'
```

### Scroll
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","ref":"e5"}'
```

Or scroll by pixels:
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","pixels":500}'
```

### Select Dropdown
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","ref":"e7","value":"Option 2"}'
```

---

## Content Extraction

### Extract Text
```bash
curl http://localhost:9867/text
```

**Query parameters:**
- `mode` — `raw` (raw innerText) or default (readability extraction)
- `tabId` — Target specific tab

**Examples:**
```bash
# Extract readable text
curl http://localhost:9867/text

# Raw innerText (full page)
curl http://localhost:9867/text?mode=raw

# From specific tab
curl "http://localhost:9867/text?tabId=abc123"
```

**Response:**
```json
{
  "text": "Example Domain\nThis domain is for use in examples...",
  "length": 1234
}
```

### Evaluate JavaScript
```bash
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'
```

**Query parameters:**
- `tabId` — Target specific tab

**Examples:**
```bash
# Get page title
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'

# Count links
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.querySelectorAll(\"a\").length"}'

# Get localStorage value
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"localStorage.getItem(\"token\")"}'

# Complex expression
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"JSON.stringify({title: document.title, url: location.href})"}'
```

**Response:**
```json
{
  "result": "Example Domain"
}
```

---

## Capture & Export

### Screenshot
```bash
curl http://localhost:9867/screenshot -o screenshot.jpg
```

**Query parameters:**
- `quality` — JPEG quality 0-100 (default: 90)
- `tabId` — Target specific tab

**Examples:**
```bash
# Screenshot to file
curl http://localhost:9867/screenshot -o page.jpg

# With quality
curl "http://localhost:9867/screenshot?quality=75" -o page.jpg

# Specific tab
curl "http://localhost:9867/screenshot?tabId=abc123" -o page.jpg

# To stdout (view metadata)
curl http://localhost:9867/screenshot
```

**Response (if no output file):**
```json
{
  "data": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
  "size": 12345,
  "mimeType": "image/jpeg"
}
```

### Export PDF
```bash
curl http://localhost:9867/pdf -o output.pdf
```

**Query parameters:**
- `output` — Output format: `json` (data URL) or `file` (filename)
- `path` — Custom file path (with `output=file`)
- `landscape` — `true` for landscape
- `paperWidth` — Width in inches (default: 8.5)
- `paperHeight` — Height in inches (default: 11)
- `marginTop`, `marginBottom`, `marginLeft`, `marginRight` — Margins in inches
- `scale` — Print scale 0.1-2.0 (default: 1.0)
- `pageRanges` — Pages to export (e.g., "1-3,5")
- `preferCSSPageSize` — `true` to honor CSS @page size
- `displayHeaderFooter` — `true` to show header/footer
- `headerTemplate` — HTML template for header
- `footerTemplate` — HTML template for footer
- `generateTaggedPDF` — `true` for accessible PDF
- `generateDocumentOutline` — `true` for document outline
- `tabId` — Target specific tab

**Examples:**
```bash
# Basic PDF
curl http://localhost:9867/pdf -o output.pdf

# Landscape
curl "http://localhost:9867/pdf?landscape=true" -o output.pdf

# Custom paper size
curl "http://localhost:9867/pdf?paperWidth=8.5&paperHeight=11&marginTop=0.5&marginBottom=0.5" -o output.pdf

# Page ranges
curl "http://localhost:9867/pdf?pageRanges=1-3,5" -o output.pdf

# With header and footer
curl "http://localhost:9867/pdf?displayHeaderFooter=true&headerTemplate=%3Cdiv%3EReport%3C%2Fdiv%3E&footerTemplate=%3Cdiv%3EPage%20%3Cspan%20class=%27pageNumber%27%3E%3C%2Fspan%3E%3C%2Fdiv%3E" -o output.pdf

# Accessible PDF
curl "http://localhost:9867/pdf?generateTaggedPDF=true&generateDocumentOutline=true" -o output.pdf

# Save to server disk
curl "http://localhost:9867/pdf?output=file&path=/tmp/report.pdf" -o output.pdf

# All options combined
curl "http://localhost:9867/pdf?landscape=true&paperWidth=8.5&paperHeight=11&marginTop=0.5&marginBottom=0.5&scale=1.0&pageRanges=1-5&displayHeaderFooter=true&generateTaggedPDF=true" -o report.pdf
```

**Response (output=json):**
```json
{
  "data": "data:application/pdf;base64,JVBERi0xLjQ...",
  "size": 45678
}
```

**Response (output=file):**
```json
{
  "path": "/tmp/report.pdf",
  "size": 45678,
  "message": "PDF saved to disk"
}
```

---

## Tab Management

### List All Tabs
```bash
curl http://localhost:9867/tabs
```

**Response:**
```json
{
  "tabs": [
    {
      "id": "abc123",
      "url": "https://example.com",
      "title": "Example Domain",
      "type": "page"
    },
    {
      "id": "def456",
      "url": "https://github.com",
      "title": "GitHub",
      "type": "page"
    }
  ]
}
```

### Create New Tab
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://google.com","newTab":true}'
```

**Response:**
```json
{
  "title": "Google",
  "url": "https://www.google.com",
  "tabId": "ghi789",
  "targetId": "target-2"
}
```

### Close Tab
```bash
curl -X DELETE http://localhost:9867/tabs/abc123
```

**Query parameters:**
- `tabId` — Tab to close

**Examples:**
```bash
# Close specific tab
curl -X DELETE http://localhost:9867/tabs/abc123

# Or use query parameter
curl -X DELETE "http://localhost:9867/tabs?tabId=abc123"
```

### Tab Locking (Multi-Agent Coordination)

#### Lock Tab
```bash
curl -X POST http://localhost:9867/tab/lock \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","owner":"agent-1","ttlSeconds":60}'
```

**Response:**
```json
{
  "tabId": "abc123",
  "owner": "agent-1",
  "lockedUntil": "2024-02-27T15:40:00Z"
}
```

#### Unlock Tab
```bash
curl -X POST http://localhost:9867/tab/unlock \
  -H "Content-Type: application/json" \
  -d '{"tabId":"abc123","owner":"agent-1"}'
```

---

## Complete Workflow Examples

### Example 1: Form Filling
```bash
# Navigate to form
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/form"}'

# Get form structure
curl http://localhost:9867/snapshot?filter=interactive

# Fill first field (email)
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e1","text":"user@example.com"}'

# Fill second field (password)
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e2","text":"mypassword"}'

# Click submit button
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

# Verify submission
curl http://localhost:9867/snapshot
curl http://localhost:9867/screenshot -o result.jpg
```

### Example 2: Scraping with Text Extraction
```bash
# Navigate
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/article"}'

# Wait a moment
sleep 2

# Extract readable text
curl http://localhost:9867/text > article.json

# Parse with jq
curl http://localhost:9867/text | jq '.text'
```

### Example 3: Multi-Tab Automation
```bash
# Navigate to first site
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://google.com"}' > tab1.json

# Get tab ID from response
TAB1=$(jq -r '.tabId' tab1.json)

# Open second tab
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com","newTab":true}' > tab2.json

TAB2=$(jq -r '.tabId' tab2.json)

# Snapshot first tab
curl "http://localhost:9867/snapshot?tabId=$TAB1"

# Snapshot second tab
curl "http://localhost:9867/snapshot?tabId=$TAB2"
```

### Example 4: PDF Report Generation
```bash
# Navigate to page
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/report"}'

# Generate PDF with custom header
curl "http://localhost:9867/pdf?displayHeaderFooter=true&headerTemplate=%3Cdiv%3EMonthly%20Report%3C%2Fdiv%3E&footerTemplate=%3Cdiv%3EPage%20%3Cspan%20class=%27pageNumber%27%3E%3C%2Fspan%3E%3C%2Fdiv%3E&landscape=true&pageRanges=1-10" -o report.pdf
```

### Example 5: JavaScript Evaluation
```bash
# Navigate
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Get page metadata
curl -X POST http://localhost:9867/execute \
  -H "Content-Type: application/json" \
  -d '{"expression":"JSON.stringify({title: document.title, url: location.href, links: document.querySelectorAll(\"a\").length})"}'
```

---

## Authentication

If your server requires authentication, add the `Authorization` header:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:9867/health

curl -H "Authorization: Bearer YOUR_TOKEN" \
  -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

Set via environment variable:
```bash
TOKEN="your-secret-token"
curl -H "Authorization: Bearer $TOKEN" http://localhost:9867/health
```

Or use curl's `-u` flag (basic auth):
```bash
curl -u token:password http://localhost:9867/health
```

---

## Error Handling

### Common Error Responses

**400 Bad Request**
```json
{
  "error": "invalid request body"
}
```

**404 Not Found**
```json
{
  "error": "tab not found"
}
```

**401 Unauthorized**
```json
{
  "error": "missing or invalid token"
}
```

**500 Server Error**
```json
{
  "error": "internal server error"
}
```

### Using with jq for Error Handling
```bash
# Check response and extract error
curl -s http://localhost:9867/snapshot | jq '.error // .'

# Check HTTP status code
curl -w "%{http_code}" http://localhost:9867/health
```

---

## Tips

### URL Encoding
Remember to URL-encode query parameters:

```bash
# Selector with spaces
curl "http://localhost:9867/snapshot?selector=main%20form"

# HTML in footer template
curl "http://localhost:9867/pdf?headerTemplate=%3Cdiv%3EReport%3C%2Fdiv%3E"

# Use jq to help with JSON escaping
TEMPLATE="<div>Page <span class='pageNumber'></span></div>"
curl -X POST http://localhost:9867/pdf \
  --data-urlencode "footerTemplate=$TEMPLATE"
```

### Piping with jq
```bash
# Extract page title
curl http://localhost:9867/execute \
  -d '{"expression":"document.title"}' | jq '.result'

# Get all clickable elements
curl http://localhost:9867/snapshot?filter=interactive | jq '.elements[] | select(.role=="button" or .role=="link")'

# Count elements
curl http://localhost:9867/snapshot | jq '.elements | length'
```

### Using curl with Files
```bash
# Save response to file
curl http://localhost:9867/screenshot -o page.jpg

# Load JSON from file
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d @request.json

# Request file example (request.json):
# {"url":"https://example.com","blockAds":true}
```

### Debugging
```bash
# Show request headers
curl -v http://localhost:9867/health

# Show response headers only
curl -i http://localhost:9867/health

# Pretty-print JSON response
curl http://localhost:9867/snapshot | jq .

# Save full response with headers
curl -D response-headers.txt http://localhost:9867/health
```

---

## API Endpoints Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/navigate` | Navigate to URL |
| `GET` | `/snapshot` | Get accessibility tree |
| `POST` | `/action` | Interact (click, type, etc.) |
| `GET` | `/text` | Extract readable text |
| `POST` | `/execute` | Evaluate JavaScript |
| `GET` | `/screenshot` | Take screenshot |
| `GET` | `/pdf` | Export PDF |
| `GET` | `/tabs` | List tabs |
| `DELETE` | `/tabs/:id` | Close tab |
| `POST` | `/tab/lock` | Lock tab for agent |
| `POST` | `/tab/unlock` | Unlock tab |

---

**For CLI commands:** See [`docs/cli-commands.md`](cli-commands.md)
