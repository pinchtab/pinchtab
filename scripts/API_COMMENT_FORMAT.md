# API Documentation Comment Format

This guide shows how to add documentation to handler functions that will be auto-extracted.

## Format

Add structured comments above handler functions using this format:

```go
// HandleSnapshot returns the accessibility tree of the current tab.
//
// @Endpoint GET /snapshot
// @Description Returns the page structure with clickable elements
//
// @Param tabId string query The tab ID (required)
// @Param filter string query Filter: "interactive" or "all" (optional, default: "all")
// @Param interactive bool query Show only interactive elements (optional)
//
// @Response 200 application/json Returns accessibility tree
//
// @Example curl:
//   curl "http://localhost:9867/snapshot?tabId=abc123&filter=interactive"
//
// @Example cli:
//   pinchtab snap -i -c
//
// @Example python:
//   import requests
//   response = requests.get("http://localhost:9867/snapshot", params={"tabId": "abc123"})
func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
```

## Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `@Endpoint` | HTTP method and path | `@Endpoint GET /snapshot` |
| `@Description` | Brief description | `@Description Returns page structure` |
| `@Param` | Query/body parameter | `@Param tabId string query Tab ID (required)` |
| `@Response` | Response format | `@Response 200 application/json Returns data` |
| `@Example curl:` | cURL example | `curl "http://localhost:9867/snapshot?..."` |
| `@Example cli:` | CLI example | `pinchtab snap -i` |
| `@Example python:` | Python example | `import requests; ...` |
| `@Example javascript:` | JavaScript example | `fetch("http://...").then(...)` |

## Parameter Format

```
@Param {name} {type} {location} {description}
```

- `name`: Parameter name
- `type`: Data type (string, int, bool, json, etc.)
- `location`: Where parameter comes from (query, body, path)
- `description`: Human-readable description + (required/optional, default value)

## Example: Full Handler

```go
// HandleAction performs an action on the current tab (click, type, etc).
//
// @Endpoint POST /action
// @Description Click, type, fill, press, or other actions on page elements
//
// @Param tabId string query The tab ID (required)
// @Param kind string body Action type: "click", "type", "fill", "press", "hover", "focus", "scroll", "select" (required)
// @Param ref string body Element reference from snapshot (e.g., "e5") (required)
// @Param text string body Text to type or fill (for "type"/"fill" actions)
// @Param value string body Value for "select" action
//
// @Response 200 application/json Returns result
// @Response 400 application/json Invalid parameters
//
// @Example curl click:
//   curl -X POST http://localhost:9867/action?tabId=abc123 \
//     -H "Content-Type: application/json" \
//     -d '{"kind":"click","ref":"e5"}'
//
// @Example curl type:
//   curl -X POST http://localhost:9867/action?tabId=abc123 \
//     -H "Content-Type: application/json" \
//     -d '{"kind":"type","ref":"e3","text":"user@example.com"}'
//
// @Example cli click:
//   pinchtab click e5
//
// @Example cli type:
//   pinchtab type e3 "user@example.com"
func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
```

## How It Works

The doc generator will:
1. Parse Go files looking for handler functions
2. Extract comments using `@` tags
3. Generate complete API documentation with:
   - Parameters and defaults
   - Examples in multiple formats (curl, CLI, Python, JavaScript)
   - Response types
   - Error codes

## Benefits

- ✅ Documentation lives in code (single source of truth)
- ✅ Easy to update when API changes
- ✅ Auto-generates curl examples
- ✅ Auto-generates CLI examples
- ✅ Shows default values
- ✅ Marks required vs optional
- ✅ Multiple language examples

## Starting Points

For now, add comments to these high-value endpoints first:

```
Priority 1 (core operations):
- HandleNavigate (POST /navigate)
- HandleSnapshot (GET /snapshot)
- HandleAction (POST /action)
- HandleText (GET /text)

Priority 2 (instance/profile management):
- handleList (GET /instances, GET /profiles)
- handleLaunchByName (POST /instances/launch)
- handleCreate (POST /profiles/create)

Priority 3 (everything else)
```

## Example: How to Add to HandleNavigate

```go
// HandleNavigate navigates a tab to a URL.
//
// @Endpoint POST /navigate
// @Description Navigate to a URL in a tab or create new tab
//
// @Param tabId string query Tab ID (optional - creates new tab if omitted)
// @Param url string query URL to navigate to (required)
// @Param blockAds bool query Block ads (optional, default: false)
// @Param blockImages bool query Block images (optional, default: false)
// @Param blockMedia bool query Block media (optional, default: false)
//
// @Response 200 application/json Returns {tabId, url, title}
//
// @Example curl navigate existing:
//   curl -X POST "http://localhost:9867/navigate?tabId=abc123&url=https://example.com"
//
// @Example curl navigate new:
//   curl -X POST "http://localhost:9867/navigate?url=https://example.com"
//
// @Example cli:
//   pinchtab nav https://example.com
func (h *Handlers) HandleNavigate(w http.ResponseWriter, r *http.Request) {
```

This way, all documentation is version-controlled in the code itself.
