# Scripts & Tools

Utility scripts for PinchTab development, testing, and documentation.

## API Documentation Generation

### Overview

Automatically generate a complete JSON API reference from Go code.

### Quick Start

```bash
# Generate endpoint list as JSON
go run scripts/gen-api-docs/main.go > docs/references/api-reference.json

# Pretty-print
go run scripts/gen-api-docs/main.go | jq .

# Count endpoints
go run scripts/gen-api-docs/main.go | jq '.count'

# Filter by method
go run scripts/gen-api-docs/main.go | jq '.endpoints[] | select(.method == "POST")'

# Count by method
go run scripts/gen-api-docs/main.go | jq '.endpoints | group_by(.method) | map({method: .[0].method, count: length})'
```

### Tool: `gen-api-docs/main.go`

Go-based tool using AST parsing. **Outputs structured JSON.**

**Features:**
- ✅ Parses all handler files (handlers, profiles, orchestrator, dashboard)
- ✅ Extracts HTTP method, path, and handler name
- ✅ Removes duplicate endpoints
- ✅ Sorts by method then path
- ✅ Outputs clean JSON for programmatic use

**Output Format:**
```json
{
  "version": "1.0",
  "generated": "auto-generated from Go code",
  "count": 66,
  "endpoints": [
    {
      "method": "GET",
      "path": "/snapshot",
      "handler": "HandleSnapshot"
    },
    {
      "method": "POST",
      "path": "/navigate",
      "handler": "HandleNavigate"
    }
  ]
}
```

### JSON File Structure

**Location:** `docs/references/api-reference.json`

**Fields:**
- `version` (string) — API reference version (e.g., "1.0")
- `generated` (string) — Generation info
- `count` (number) — Total number of endpoints
- `endpoints` (array) — List of endpoint objects
  - `method` — HTTP method (GET, POST, PUT, DELETE, PATCH)
  - `path` — URL path pattern (e.g., `/snapshot`, `/instances/{id}/action`)
  - `handler` — Go handler function name

### Use Cases

**Programmatic Access:**
```bash
# Find all POST endpoints
jq '.endpoints[] | select(.method == "POST") | .path'

# Find endpoints by path
jq '.endpoints[] | select(.path | contains("instances"))'

# Find endpoints by handler
jq '.endpoints[] | select(.handler | contains("Handle"))'
```

**Validation:**
```bash
# Verify total count
TOTAL=$(jq '.count' < docs/references/api-reference.json)
ACTUAL=$(jq '.endpoints | length' < docs/references/api-reference.json)
[ "$TOTAL" = "$ACTUAL" ] && echo "Count matches" || echo "Mismatch"
```

**CI/CD:**
```bash
# Generate in CI pipeline
go run scripts/gen-api-docs/main.go > /tmp/api-ref.json
# Commit if changed
git diff docs/references/api-reference.json && git add . && git commit -m "docs: update api reference"
```

### Technical Details

The Go tool:
- Uses `go/parser` to parse Go source files
- Uses `go/ast` to traverse the syntax tree
- Finds all functions named `RegisterHandlers` or `RegisterRoutes`
- Extracts `mux.HandleFunc()` and `mux.Handle()` calls
- Parses route patterns with regex: `"METHOD /path"`
- Removes duplicates (same method + path)
- Sorts results for consistency
- Outputs JSON with proper formatting

### Why JSON?

- ✅ **Structured data** — Easy to parse programmatically
- ✅ **No styling** — Plain facts, no markdown formatting to maintain
- ✅ **Tooling** — Use `jq` or any JSON tool to query/filter
- ✅ **Automation** — CI/CD can validate endpoint counts, check for breaking changes
- ✅ **Flexibility** — Can generate docs in any format from JSON

### For Complete Documentation

For full endpoint documentation with parameters, examples, response types, see:
- `docs/examples/API_DOCUMENTATION_EXAMPLES.md` — Complete workflow examples
- `docs/references/endpoints.md` — Full manual API reference
- `docs/references/curl-commands.md` — cURL examples

---

## Other Scripts

Bash-based tools:
- `generate-api-docs.sh` — Regex-based endpoint extraction (fallback)
