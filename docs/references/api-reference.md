# API Reference (Auto-Generated)

Generated: 2026-03-01T00:38:44Z

---

## Endpoints Summary

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| DELETE | `/profiles/{name}` | handleDeleteByPath |  |
| GET | `/` |  |  |
| GET | `/cookies` | HandleGetCookies |  |
| GET | `/dashboard` |  |  |
| GET | `/dashboard/` |  |  |
| GET | `/dashboard/agents` | handleAgents |  |
| GET | `/dashboard/events` | handleSSE |  |
| GET | `/download` | HandleDownload | fetches a URL using the browser's session (cookies, stealth) |
| GET | `/health` | HandleHealth |  |
| GET | `/instances` | handleList |  |
| GET | `/instances/tabs` | handleAllTabs |  |
| GET | `/instances/{id}/cookies` | proxyToInstance |  |
| GET | `/instances/{id}/download` | proxyToInstance |  |
| GET | `/instances/{id}/logs` | handleLogsByID |  |
| GET | `/instances/{id}/pdf` | proxyToInstance |  |
| GET | `/instances/{id}/proxy/screencast` | handleProxyScreencast |  |
| GET | `/instances/{id}/screencast` | proxyToInstance |  |
| GET | `/instances/{id}/screenshot` | proxyToInstance |  |
| GET | `/instances/{id}/snapshot` | proxyToInstance |  |
| GET | `/instances/{id}/tabs` | proxyToInstance |  |
| GET | `/instances/{id}/text` | proxyToInstance |  |
| GET | `/pdf` | HandlePDF |  |
| GET | `/profiles` | handleList |  |
| GET | `/profiles/{id}/instance` | handleProfileInstance |  |
| GET | `/profiles/{name}/analytics` | handleAnalyticsByPath |  |
| GET | `/profiles/{name}/logs` | handleLogsByPath |  |
| GET | `/screencast` | HandleScreencast | upgrades to WebSocket and streams screencast frames for a ta... |
| GET | `/screencast/tabs` | HandleScreencastAll | returns info for building a multi-tab screencast view. |
| GET | `/screenshot` | HandleScreenshot |  |
| GET | `/snapshot` | HandleSnapshot | returns the accessibility tree of a tab. |
| GET | `/stealth/status` | HandleStealthStatus |  |
| GET | `/tabs` | HandleTabs |  |
| GET | `/text` | HandleText |  |
| GET | `/welcome` |  |  |
| PATCH | `/profiles/meta` | handleUpdateMeta |  |
| PATCH | `/profiles/{name}` | handleUpdateByPath |  |
| POST | `/action` | HandleAction | performs a single action on a tab (click, type, fill, etc). |
| POST | `/actions` | HandleActions |  |
| POST | `/cookies` | HandleSetCookies |  |
| POST | `/ensure-chrome` | HandleEnsureChrome |  |
| POST | `/evaluate` | HandleEvaluate |  |
| POST | `/fingerprint/rotate` | HandleFingerprintRotate |  |
| POST | `/instances/launch` | handleLaunchByName |  |
| POST | `/instances/{id}/action` | proxyToInstance |  |
| POST | `/instances/{id}/actions` | proxyToInstance |  |
| POST | `/instances/{id}/cookies` | proxyToInstance |  |
| POST | `/instances/{id}/ensure-chrome` | proxyToInstance |  |
| POST | `/instances/{id}/evaluate` | proxyToInstance |  |
| POST | `/instances/{id}/navigate` | proxyToInstance |  |
| POST | `/instances/{id}/stop` | handleStopByInstanceID |  |
| POST | `/instances/{id}/tab` | proxyToInstance |  |
| POST | `/instances/{id}/tab/lock` | proxyToInstance |  |
| POST | `/instances/{id}/tab/unlock` | proxyToInstance |  |
| POST | `/instances/{id}/upload` | proxyToInstance |  |
| POST | `/navigate` | HandleNavigate | navigates a tab to a URL or creates a new tab. |
| POST | `/profiles/create` | handleCreate |  |
| POST | `/profiles/import` | handleImport |  |
| POST | `/profiles/{id}/start` | handleStartByID |  |
| POST | `/profiles/{id}/stop` | handleStopByID |  |
| POST | `/profiles/{name}/reset` | handleResetByPath |  |
| POST | `/start/{id}` | handleStartByID |  |
| POST | `/stop/{id}` | handleStopByID |  |
| POST | `/tab` | HandleTab |  |
| POST | `/tab/lock` | HandleTabLock |  |
| POST | `/tab/unlock` | HandleTabUnlock |  |
| POST | `/upload` | HandleUpload | sets files on an <input type="file"> element via CDP. |

---

## Detailed Endpoints

### DELETE /profiles/{name}

**Handler:** `handleDeleteByPath`

### GET /

### GET /cookies

**Handler:** `HandleGetCookies`

### GET /dashboard

### GET /dashboard/

### GET /dashboard/agents

**Handler:** `handleAgents`

### GET /dashboard/events

**Handler:** `handleSSE`

### GET /download

HandleDownload fetches a URL using the browser's session (cookies, stealth)
and returns the content. This preserves authentication and fingerprint.
//
GET /download?url=<url>[&tabId=<id>][&output=file&path=/tmp/file][&raw=true]

**Handler:** `HandleDownload`

### GET /health

**Handler:** `HandleHealth`

### GET /instances

**Handler:** `handleList`

### GET /instances/tabs

**Handler:** `handleAllTabs`

### GET /instances/{id}/cookies

**Handler:** `proxyToInstance`

### GET /instances/{id}/download

**Handler:** `proxyToInstance`

### GET /instances/{id}/logs

**Handler:** `handleLogsByID`

### GET /instances/{id}/pdf

**Handler:** `proxyToInstance`

### GET /instances/{id}/proxy/screencast

**Handler:** `handleProxyScreencast`

### GET /instances/{id}/screencast

**Handler:** `proxyToInstance`

### GET /instances/{id}/screenshot

**Handler:** `proxyToInstance`

### GET /instances/{id}/snapshot

**Handler:** `proxyToInstance`

### GET /instances/{id}/tabs

**Handler:** `proxyToInstance`

### GET /instances/{id}/text

**Handler:** `proxyToInstance`

### GET /pdf

**Handler:** `HandlePDF`

### GET /profiles

**Handler:** `handleList`

### GET /profiles/{id}/instance

**Handler:** `handleProfileInstance`

### GET /profiles/{name}/analytics

**Handler:** `handleAnalyticsByPath`

### GET /profiles/{name}/logs

**Handler:** `handleLogsByPath`

### GET /screencast

HandleScreencast upgrades to WebSocket and streams screencast frames for a tab.
Query params: tabId (required), quality (1-100, default 40), maxWidth (default 800), fps (1-30, default 5)

**Handler:** `HandleScreencast`

### GET /screencast/tabs

HandleScreencastAll returns info for building a multi-tab screencast view.

**Handler:** `HandleScreencastAll`

### GET /screenshot

**Handler:** `HandleScreenshot`

### GET /snapshot

HandleSnapshot returns the accessibility tree of a tab.
//
@Endpoint GET /snapshot
@Description Returns the page structure with clickable elements, form fields, and text content
//
@Param tabId string query Tab ID (required)
@Param filter string query Filter type: "interactive" for clickable/inputs only, "all" for everything (optional, default: "all")
@Param interactive bool query Alias for filter=interactive (optional)
@Param compact bool query Compact output (shorter ref names) (optional, default: false)
@Param depth int query Max nesting depth (optional, default: -1 for full tree)
@Param text bool query Include text content (optional, default: true)
@Param format string query Output format: "json" or "yaml" (optional, default: "json")
@Param diff bool query Include diff with previous snapshot (optional, default: false)
@Param output string query Write to file instead of response (optional)
//
@Response 200 application/json Returns accessibility tree with refs
@Response 400 application/json Invalid tabId or parameters
@Response 404 application/json Tab not found
//
@Example curl all elements:
  curl "http://localhost:9867/snapshot?tabId=abc123"
//
@Example curl interactive only:
  curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive"
//
@Example curl compact:
  curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive&compact=true"
//
@Example cli:
  pinchtab snap -i -c
//
@Example python:
  import requests
  r = requests.get("http://localhost:9867/snapshot", params={"tabId": "abc123", "filter": "interactive"})
  tree = r.json()

**Handler:** `HandleSnapshot`

### GET /stealth/status

**Handler:** `HandleStealthStatus`

### GET /tabs

**Handler:** `HandleTabs`

### GET /text

**Handler:** `HandleText`

### GET /welcome

### PATCH /profiles/meta

**Handler:** `handleUpdateMeta`

### PATCH /profiles/{name}

**Handler:** `handleUpdateByPath`

### POST /action

HandleAction performs a single action on a tab (click, type, fill, etc).
//
@Endpoint POST /action
@Description Interact with page elements: click, type text, fill inputs, press keys, hover, focus, scroll, select
//
@Param tabId string body Tab ID (required)
@Param kind string body Action type: "click", "type", "fill", "press", "hover", "focus", "scroll", "select" (required)
@Param ref string body Element reference from snapshot (e.g., "e5") (required)
@Param text string body Text to type or fill (for "type"/"fill" actions)
@Param value string body Value for "select" action (e.g., option index)
@Param key string body Key to press (for "press" action, e.g., "Enter", "Tab")
@Param x int body X coordinate for "scroll" action (optional)
@Param y int body Y coordinate for "scroll" action (optional)
//
@Response 200 application/json Returns {success: true}
@Response 400 application/json Invalid action or parameters
@Response 404 application/json Tab or element not found
@Response 500 application/json Chrome error
//
@Example curl click:
  curl -X POST http://localhost:9867/action \
    -H "Content-Type: application/json" \
    -d '{"tabId":"abc123","kind":"click","ref":"e5"}'
//
@Example curl type:
  curl -X POST http://localhost:9867/action \
    -H "Content-Type: application/json" \
    -d '{"tabId":"abc123","kind":"type","ref":"e3","text":"user@example.com"}'
//
@Example curl fill form:
  curl -X POST http://localhost:9867/action \
    -H "Content-Type: application/json" \
    -d '{"tabId":"abc123","kind":"fill","ref":"e3","text":"John Doe"}'
//
@Example curl press key:
  curl -X POST http://localhost:9867/action \
    -H "Content-Type: application/json" \
    -d '{"tabId":"abc123","kind":"press","ref":"e7","key":"Enter"}'
//
@Example cli click:
  pinchtab click e5
//
@Example cli type:
  pinchtab type e3 "user@example.com"
//
@Example cli fill:
  pinchtab fill e3 "John Doe"

**Handler:** `HandleAction`

### POST /actions

**Handler:** `HandleActions`

### POST /cookies

**Handler:** `HandleSetCookies`

### POST /ensure-chrome

**Handler:** `HandleEnsureChrome`

### POST /evaluate

**Handler:** `HandleEvaluate`

### POST /fingerprint/rotate

**Handler:** `HandleFingerprintRotate`

### POST /instances/launch

**Handler:** `handleLaunchByName`

### POST /instances/{id}/action

**Handler:** `proxyToInstance`

### POST /instances/{id}/actions

**Handler:** `proxyToInstance`

### POST /instances/{id}/cookies

**Handler:** `proxyToInstance`

### POST /instances/{id}/ensure-chrome

**Handler:** `proxyToInstance`

### POST /instances/{id}/evaluate

**Handler:** `proxyToInstance`

### POST /instances/{id}/navigate

**Handler:** `proxyToInstance`

### POST /instances/{id}/stop

**Handler:** `handleStopByInstanceID`

### POST /instances/{id}/tab

**Handler:** `proxyToInstance`

### POST /instances/{id}/tab/lock

**Handler:** `proxyToInstance`

### POST /instances/{id}/tab/unlock

**Handler:** `proxyToInstance`

### POST /instances/{id}/upload

**Handler:** `proxyToInstance`

### POST /navigate

HandleNavigate navigates a tab to a URL or creates a new tab.
//
@Endpoint POST /navigate
@Description Navigate to a URL in an existing tab or create a new tab and navigate
//
@Param tabId string body Tab ID to navigate in (optional - creates new if omitted)
@Param url string body URL to navigate to (required)
@Param newTab bool body Force create new tab (optional, default: false)
@Param waitTitle float64 body Wait for title change (ms) (optional, default: 0)
@Param timeout float64 body Timeout for navigation (ms) (optional, default: 30000)
//
@Response 200 application/json Returns {tabId, url, title}
@Response 400 application/json Invalid URL or parameters
@Response 500 application/json Chrome error
//
@Example curl navigate new:
  curl -X POST http://localhost:9867/navigate \
    -H "Content-Type: application/json" \
    -d '{"url":"https://example.com"}'
//
@Example curl navigate existing:
  curl -X POST http://localhost:9867/navigate \
    -H "Content-Type: application/json" \
    -d '{"tabId":"abc123","url":"https://google.com"}'
//
@Example cli:
  pinchtab nav https://example.com

**Handler:** `HandleNavigate`

### POST /profiles/create

**Handler:** `handleCreate`

### POST /profiles/import

**Handler:** `handleImport`

### POST /profiles/{id}/start

**Handler:** `handleStartByID`

### POST /profiles/{id}/stop

**Handler:** `handleStopByID`

### POST /profiles/{name}/reset

**Handler:** `handleResetByPath`

### POST /start/{id}

**Handler:** `handleStartByID`

### POST /stop/{id}

**Handler:** `handleStopByID`

### POST /tab

**Handler:** `HandleTab`

### POST /tab/lock

**Handler:** `HandleTabLock`

### POST /tab/unlock

**Handler:** `HandleTabUnlock`

### POST /upload

HandleUpload sets files on an <input type="file"> element via CDP.
//
POST /upload?tabId=<id>
//
//	{
//	  "selector": "input[type=file]",
//	  "files": ["data:image/png;base64,...", "base64:..."],
//	  "paths": ["/tmp/photo.jpg"]
//	}
//
Either "files" (base64 data) or "paths" (local file paths) must be provided.
Both can be combined. Files are written to a temp dir and passed to CDP.

**Handler:** `HandleUpload`

---

## Notes

- This documentation is auto-generated from Go code
- For full implementation details, see `internal/handlers/*.go`
- Query parameters and request bodies are defined in each handler
