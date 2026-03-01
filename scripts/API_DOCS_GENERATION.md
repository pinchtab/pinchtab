# Generating API Documentation

This guide explains how to automatically generate API reference documentation from Go code.

## Overview

We have two tools to help generate API documentation:

1. **Go tool** (`scripts/gen-api-docs/main.go`) — More reliable, parses Go AST
2. **Bash script** (`scripts/generate-api-docs.sh`) — Simple pattern matching

## Using the Go Tool (Recommended)

### Generate Endpoint Summary

```bash
# Run from repo root
go run scripts/gen-api-docs/main.go
```

**Output:**
- Endpoint summary table (Method, Path, Handler)
- Detailed endpoint list
- Handler names

### Redirect to File

```bash
go run scripts/gen-api-docs/main.go > docs/references/endpoints-auto.md
```

### What It Extracts

- HTTP method (GET, POST, PUT, DELETE, PATCH)
- Path pattern (e.g., `/snapshot`, `/action`)
- Handler function name (e.g., `HandleSnapshot`)
- Sorts by method, then path

## How It Works

The Go tool:

1. **Parses** `internal/handlers/handlers.go`
2. **Uses Go AST** to find `RegisterRoutes()` function
3. **Extracts** all `mux.HandleFunc()` and `mux.Handle()` calls
4. **Parses** route strings with pattern: `"METHOD /path"`
5. **Generates** markdown documentation

### Limitations

Currently extracts:
- ✅ HTTP method and path
- ✅ Handler function name
- ❌ Query parameters (would need to parse handler functions)
- ❌ Request body structure (would need struct tag analysis)
- ❌ Response types (would need return type analysis)

## Enhancing the Tool

To add more features, extend `scripts/gen-api-docs/main.go`:

### Extract Query Parameters

```go
// Look for r.URL.Query().Get("paramName") in handler code
// Parse parameters from handler functions
```

### Extract Request Body Structure

```go
// Look for json.Unmarshal patterns
// Parse request struct definitions
// Extract field names and types
```

### Extract Response Types

```go
// Analyze handler return types
// Look for json.Marshal patterns
// Extract response struct definitions
```

## Current Usage

The manually-written `docs/references/endpoints.md` is the source of truth because it includes:
- Full request/response examples
- Query parameters and defaults
- Error responses
- Usage examples

**Auto-generation tool is useful for:**
- Verifying endpoints are documented
- Creating a quick reference table
- Tracking which endpoints exist in code
- Finding undocumented endpoints

## Next Steps

To make this fully automated:

1. Add structured comments above handler functions:
   ```go
   // HandleSnapshot returns the accessibility tree
   // Query params: tabId, filter, interactive
   func (h *Handlers) HandleSnapshot(w http.ResponseWriter, r *http.Request) {
   ```

2. Use the `go/parser` with structured comments to extract documentation

3. Parse request/response types using `go/types` for full type information

4. Generate complete API docs automatically (with parameter docs, types, examples)

## Related Files

- `internal/handlers/handlers.go` — Route definitions
- `internal/handlers/*.go` — Handler implementations
- `docs/references/endpoints.md` — Manually-written API docs (source of truth)
