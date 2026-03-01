# Scripts & Tools

Utility scripts for PinchTab development, testing, and documentation.

## API Documentation Generation

### Overview

Generate API reference documentation automatically from Go code.

Two approaches:

1. **Auto-extraction from code routes** — Fast, no comments needed
2. **Structured comments** — Rich documentation with examples

### Quick Start

```bash
# Generate endpoint list (basic)
go run scripts/gen-api-docs/main.go > docs/references/api-reference.md

# Or with bash
./scripts/generate-api-docs.sh > docs/references/api-reference-bash.md
```

### Tools

#### `gen-api-docs/main.go` (Recommended)

Go-based tool using AST parsing.

**Features:**
- ✅ Parses `internal/handlers/handlers.go`
- ✅ Parses `internal/profiles/handlers.go`
- ✅ Parses `internal/orchestrator/handlers.go`
- ✅ Parses `internal/dashboard/dashboard.go`
- ✅ Generates sorted endpoint table
- ✅ Extracts handler names
- ✅ (Ready for) Structured comment parsing

**Usage:**
```bash
# Generate and print to stdout
go run scripts/gen-api-docs/main.go

# Generate to file
go run scripts/gen-api-docs/main.go > docs/references/api-reference.md

# Count endpoints
go run scripts/gen-api-docs/main.go | grep "^\| (GET\|POST\|DELETE\|PATCH\|PUT)" | wc -l
```

#### `generate-api-docs.sh`

Bash-based tool using regex patterns.

**Features:**
- ✅ Simple pattern matching
- ✅ No dependencies (pure bash)
- ✅ Quick for CI/CD

**Usage:**
```bash
./scripts/generate-api-docs.sh > docs/references/api-reference-bash.md
```

### Adding Documentation to Handlers

#### Step 1: Review the Format

See `scripts/API_COMMENT_FORMAT.md` for the structured comment format.

#### Step 2: Add Comments to Handlers

Example: Adding documentation to `HandleSnapshot`

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
func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
  // implementation...
}
```

#### Step 3: Regenerate Docs

```bash
go run scripts/gen-api-docs/main.go > docs/references/api-reference.md
```

The tool will automatically extract your comments (once parsing is implemented).

### Priority Endpoints to Document

Add comments in this order:

**Phase 1 (Core Operations):**
- `HandleNavigate` (POST /navigate)
- `HandleSnapshot` (GET /snapshot)
- `HandleAction` (POST /action)
- `HandleText` (GET /text)

**Phase 2 (Instance/Profile Management):**
- `handleList` in orchestrator (GET /instances, GET /profiles)
- `handleLaunchByName` in orchestrator (POST /instances/launch)
- `handleCreate` in profiles (POST /profiles/create)

**Phase 3 (Complete Coverage):**
- All remaining endpoints

### Documentation Files

| File | Purpose |
|------|---------|
| `API_COMMENT_FORMAT.md` | Guide for structured comments |
| `API_DOCS_GENERATION.md` | How the generator works |
| `gen-api-docs/main.go` | Go AST parser |
| `generate-api-docs.sh` | Bash fallback |

### Next Steps

1. **Add comments** to high-value handlers using the format in `API_COMMENT_FORMAT.md`
2. **Run generator:** `go run scripts/gen-api-docs/main.go > docs/references/api-reference.md`
3. **Commit:** Both code changes and generated docs
4. **Iterate:** Add more comments to remaining endpoints

### Example Output

Current generated docs in `docs/references/api-reference.md`:

- **72 endpoints** extracted from code
- **Sorted by method then path**
- **Ready for enrichment** with structured comments

### Technical Details

The Go tool:
- Uses `go/parser` to parse Go source files
- Uses `go/ast` to traverse the syntax tree
- Finds all functions named `RegisterHandlers` or `RegisterRoutes`
- Extracts `mux.HandleFunc()` and `mux.Handle()` calls
- Parses route patterns with regex: `"METHOD /path"`
- Sorts results for consistency

Future enhancements:
- Parse `@Param` tags from comments
- Extract parameter types and defaults
- Generate complete examples (curl, CLI, Python, JS)
- Validate against actual handler signatures

---

For questions, see:
- `scripts/API_COMMENT_FORMAT.md` — How to document handlers
- `scripts/API_DOCS_GENERATION.md` — Technical details
- `docs/references/api-reference.md` — Current generated output
