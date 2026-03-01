# Scripts & Tools

Utility scripts for PinchTab development, testing, and documentation.

## API Documentation Generation

### Overview

Keep the API reference in sync with code changes **without losing user metadata**.

### Quick Start

```bash
# Check what changed (dry-run)
go run scripts/update-api-reference.go --dry-run

# Apply changes (adds new endpoints, removes old ones, keeps metadata)
go run scripts/update-api-reference.go

# Query the result
jq '.endpoints | map(select(.cli == true)) | length' docs/references/api-reference.json
jq '.endpoints[] | select(.method == "POST") | .path' docs/references/api-reference.json
```

### Tools

#### `update-api-reference.go` (Main Tool - USE THIS)

**Smart merge tool** that updates the API reference without overwriting user-added metadata.

**Features:**
- ✅ Scans code for current endpoints
- ✅ Reads existing `api-reference.json`
- ✅ **Merges intelligently:**
  - Keeps all user-added fields (description, examples, params, implementation status)
  - Adds new endpoints found in code
  - Removes endpoints no longer in code
  - Updates only method/path/handler
- ✅ Shows what changed (added/removed)
- ✅ Preserves all manual metadata

**Usage:**
```bash
# Dry-run first (show changes, no write)
go run scripts/update-api-reference.go --dry-run

# Apply changes
go run scripts/update-api-reference.go
```

**Example Output:**
```
Found 66 endpoints in code
Found 66 endpoints in existing file
Merged to 66 endpoints
✓ No changes needed

Wrote docs/references/api-reference.json
```

#### `gen-api-docs/main.go` (Reference)

Go-based tool using AST parsing. Use this **only** if you want to regenerate from scratch.

**Output:**
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
    }
  ]
}
```

### Workflow: Keep API Reference in Sync

**Scenario 1: New endpoint added to code**
```bash
go run scripts/update-api-reference.go --dry-run
# Output:
# ✨ New endpoints:
#   + POST /new-endpoint

go run scripts/update-api-reference.go
# Adds /new-endpoint with basic fields, preserves all existing metadata
```

**Scenario 2: Endpoint removed from code**
```bash
go run scripts/update-api-reference.go --dry-run
# Output:
# ❌ Removed endpoints:
#   - DELETE /old-endpoint

go run scripts/update-api-reference.go
# Removes /old-endpoint, preserves all other data
```

**Scenario 3: User adds metadata manually**
Edit `docs/references/api-reference.json` to add:
```json
{
  "method": "GET",
  "path": "/snapshot",
  "handler": "HandleSnapshot",
  "description": "Get accessibility tree",
  "cli": true,
  "curl": true,
  "examples": {
    "curl": "curl http://localhost:9867/snapshot?tabId=abc"
  }
}
```

Then run:
```bash
go run scripts/update-api-reference.go
# Preserves all your edits, updates code-derived fields only
```

### JSON File Structure

**Location:** `docs/references/api-reference.json`

**Fields per endpoint:**
- `method` — HTTP method (GET, POST, PUT, DELETE, PATCH)
- `path` — URL path pattern (e.g., `/snapshot`, `/instances/{id}/action`)
- `handler` — Go handler function name
- `description` (optional) — What this endpoint does
- `cli` (optional) — CLI support flag
- `curl` (optional) — Curl support flag
- `params` (optional) — Parameter list with types/descriptions
- `examples` (optional) — curl/cli/payload examples
- `implemented` (optional) — Implementation status

### Query Examples

```bash
# List all endpoints with CLI support
jq '.endpoints[] | select(.cli == true) | {method, path}' docs/references/api-reference.json

# Count endpoints by method
jq '.endpoints | group_by(.method) | map({method: .[0].method, count: length})' docs/references/api-reference.json

# Find endpoints with documented examples
jq '.endpoints[] | select(.examples.curl != null) | .path' docs/references/api-reference.json

# Validate total count
TOTAL=$(jq '.count' docs/references/api-reference.json)
ACTUAL=$(jq '.endpoints | length' docs/references/api-reference.json)
[ "$TOTAL" = "$ACTUAL" ] && echo "✓ Count matches" || echo "✗ Mismatch"
```

### Why This Approach?

- ✅ **Automatic sync** — Code changes reflected without manual updates
- ✅ **User data preserved** — Descriptions, examples, status stay intact
- ✅ **Conflict-free** — Smart merge avoids overwriting metadata
- ✅ **Auditable** — Shows what changed with dry-run
- ✅ **Git-friendly** — Only changed endpoints in commits

---

## Other Scripts

- `gen-api-docs/main.go` — Generate endpoints from scratch
- `generate-api-docs.sh` — Bash alternative (legacy)
- `ENDPOINT_METADATA.md` — Guide for adding structured endpoint docs
