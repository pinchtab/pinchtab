# Endpoint Metadata Format

Add rich metadata to endpoints using structured comments.

## Format

Add comments above handler functions with metadata:

```go
// HandleNavigate navigates a tab to a URL or creates a new tab.
// @Description Navigate to a URL in an existing tab or create a new tab
// @Implemented true
// @Param tabId string body optional Tab ID (creates new if omitted)
// @Param url string body required URL to navigate to
// @Param newTab bool body optional Force create new tab (default: false)
// @Param timeout float64 body optional Timeout in ms (default: 30000)
// @Curl curl -X POST http://localhost:9867/navigate -H "Content-Type: application/json" -d '{"url":"https://example.com"}'
// @CLI pinchtab nav https://example.com
// @Example {"tabId":"tab-123","url":"https://example.com","timeout":30000}
func (h *Handlers) HandleNavigate(w http.ResponseWriter, r *http.Request) {
```

## Tags

| Tag | Format | Example | Purpose |
|-----|--------|---------|---------|
| `@Description` | Text | `@Description Navigate to URL or create new tab` | Brief endpoint description |
| `@Implemented` | true/false | `@Implemented true` | Is this endpoint fully implemented? |
| `@Param` | name type location required/optional description | `@Param url string body required URL to navigate` | Parameter definition |
| `@Curl` | Full curl command | `@Curl curl -X POST http://localhost:9867/...` | Example curl command |
| `@CLI` | CLI command | `@CLI pinchtab nav https://example.com` | Example CLI command |
| `@Example` | JSON | `@Example {"url":"https://example.com"}` | Example request payload |

## Example: Full Handler Documentation

```go
// HandleAction performs a single action on a tab.
// @Description Click, type, fill, hover, focus, scroll, select on page elements
// @Implemented true
// @Param tabId string body required Tab ID
// @Param kind string body required Action: "click", "type", "fill", "press", "hover", "focus", "scroll", "select"
// @Param ref string body required Element ref from snapshot (e.g., "e5")
// @Param text string body optional Text to type or fill
// @Param key string body optional Key for "press" (e.g., "Enter", "Tab")
// @Curl curl -X POST http://localhost:9867/action -H "Content-Type: application/json" -d '{"tabId":"abc","kind":"click","ref":"e5"}'
// @CLI pinchtab click e5
// @Example {"tabId":"abc123","kind":"click","ref":"e5"}
func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
```

## Parameter Format

```
@Param {name} {type} {location} {required|optional} {description}
```

**Types:** `string`, `int`, `bool`, `float64`, `json`
**Location:** `query`, `body`, `path`
**Requirement:** `required` or `optional (with defaults)`

## Examples (By Endpoint Type)

### Query Parameters (GET)

```go
// @Param tabId string query required Tab ID
// @Param filter string query optional Filter: "interactive" or "all" (default: "all")
// @Param compact bool query optional Compact output (default: false)
```

### Body Parameters (POST)

```go
// @Param tabId string body required Tab ID
// @Param url string body required URL to navigate to
// @Param timeout float64 body optional Timeout in ms (default: 30000)
```

### Path Parameters

```go
// @Param id string path required Instance ID
// @Param name string path required Profile name
```

## Priority Endpoints to Document

Phase 1 (Core):
- `HandleNavigate` (POST /navigate)
- `HandleSnapshot` (GET /snapshot)
- `HandleAction` (POST /action)
- `HandleText` (GET /text)

Phase 2 (Instance Management):
- `handleList` in orchestrator (GET /instances)
- `handleLaunchByName` in orchestrator (POST /instances/launch)

Phase 3 (Profile Management):
- `handleCreate` in profiles (POST /profiles/create)
- `handleImport` in profiles (POST /profiles/import)

## Current Implementation Status

Check `docs/references/api-reference.json` for the full list. Mark endpoints as:
- `@Implemented true` — Fully working
- `@Implemented false` — Planned/WIP
- (No tag) — Unknown status
