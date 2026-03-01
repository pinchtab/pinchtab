# CLI Design Guide

## Principles

1. **Hierarchical path mirroring** — CLI command structure mirrors HTTP endpoints
2. **Resource-oriented** — `pinchtab <resource> <action>` not `pinchtab <action>`
3. **Consistent argument order** — `<resource> <id> <sub-resource> <sub-id> <action> [flags] [payload]`
4. **Smart payload handling** — Flags for simple cases, stdin/JSON for complex payloads
5. **No breaking changes** — Always maintain backward compatibility

## Command Structure

### Pattern 1: List Resources
```
pinchtab <resource>              List all resources
```

**Examples:**
```bash
pinchtab instances               # GET /instances
pinchtab profiles                # GET /profiles
```

**Output:** JSON array (auto-formatted)

### Pattern 2: Manage Resource (Orchestrator)
```
pinchtab <resource> <action>     Orchestrator-level actions
pinchtab <resource> <id> <action> Instance-scoped actions
```

**Examples:**
```bash
pinchtab instance launch --mode headed --port 9869
pinchtab instance <id> logs
pinchtab instance <id> stop
```

**HTTP mapping:**
- `pinchtab instance launch [flags]` → `POST /instances/launch {mode, port}`
- `pinchtab instance <id> logs` → `GET /instances/<id>/logs`
- `pinchtab instance <id> stop` → `POST /instances/<id>/stop`

### Pattern 3: Browser Control (Instance-scoped)
```
pinchtab [--instance <id>] <action> [args] [flags]
```

**Examples:**
```bash
# Default instance (auto-detected from PINCHTAB_INSTANCE env or from running instance)
pinchtab nav https://example.com
pinchtab snap -i -c
pinchtab click e5

# Explicit instance
pinchtab --instance inst_abc123 nav https://example.com
pinchtab --instance inst_abc123 snap
```

**HTTP mapping:**
- `pinchtab nav <url>` → `POST /navigate {url}`
- `pinchtab --instance <id> snap` → `POST /instances/<id>/snapshot`
- `pinchtab --instance <id> click <ref>` → `POST /instances/<id>/action {kind: "click", ref}`

### Pattern 4: Nested Resources (Tabs within Instance)
```
pinchtab --instance <id> tab <action> [args]      List/manage tabs
pinchtab --instance <id> tab <tabId> <action>     Operate on specific tab
```

**Examples:**
```bash
# List tabs
pinchtab --instance inst_abc123 tabs              # GET /instances/<id>/tabs

# Create tab
pinchtab --instance inst_abc123 tab create https://example.com
                                                   # POST /instances/<id>/tab {url}

# Navigate tab
pinchtab --instance inst_abc123 tab <tabId> navigate https://example.com
                                                   # POST /instances/<id>/tab/navigate {tabId, url}

# Close tab
pinchtab --instance inst_abc123 tab <tabId> close
                                                   # POST /instances/<id>/tab/close {tabId}

# Lock tab
pinchtab --instance inst_abc123 tab <tabId> lock --owner agent1 --ttl 60
                                                   # POST /instances/<id>/tab/lock {tabId, owner, ttl}
```

### Pattern 5: Complex Payloads

#### Option A: Flags (simple cases)
```bash
pinchtab --instance <id> navigate https://example.com --block-images --block-ads --timeout 30
```

Maps to:
```json
POST /instances/<id>/navigate
{
  "url": "https://example.com",
  "blockImages": true,
  "blockAds": true,
  "timeout": 30
}
```

#### Option B: JSON stdin (complex cases)
```bash
cat << 'EOF' | pinchtab --instance <id> action
{
  "kind": "select",
  "ref": "e5",
  "value": "option2",
  "waitNav": true
}
EOF
```

Or:
```bash
pinchtab --instance <id> action -f payload.json
pinchtab --instance <id> action --json '{...}'
```

#### Option C: Shell-friendly formats
```bash
# String payload (auto-quoted)
pinchtab --instance <id> evaluate 'document.title'

# File reference
pinchtab --instance <id> upload --file screenshot.png --to /form/input

# Multi-line (heredoc)
pinchtab --instance <id> action << 'EOF'
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e5"},
    {"kind": "type", "ref": "e12", "text": "hello"}
  ]
}
EOF
```

## Mapping: Endpoints → CLI Commands

### Instance Management
| Endpoint | HTTP | CLI Command |
|----------|------|-------------|
| `/instances` | GET | `pinchtab instances` |
| `/instances/launch` | POST | `pinchtab instance launch --mode headed --port 9869` |
| `/instances/{id}/logs` | GET | `pinchtab instance <id> logs` |
| `/instances/{id}/stop` | POST | `pinchtab instance <id> stop` |

### Browser Control (Single Instance)
| Endpoint | HTTP | CLI Command |
|----------|------|-------------|
| `/navigate` | POST | `pinchtab nav https://example.com` |
| `/snapshot` | GET | `pinchtab snap -i -c` |
| `/screenshot` | GET | `pinchtab ss -o out.png` |
| `/action` | POST | `pinchtab click e5` |
| `/actions` | POST | `pinchtab action -f actions.json` |
| `/tab` | POST | `pinchtab tab create https://example.com` |
| `/tabs` | GET | `pinchtab tabs` |
| `/evaluate` | POST | `pinchtab eval "document.title"` |
| `/tabs/{id}/pdf` | GET | `pinchtab pdf --tab <tabId> -o out.pdf --landscape` |
| `/text` | GET | `pinchtab text` |
| `/text` | GET | `pinchtab text --raw` |
| `/cookies` | GET | `pinchtab cookies` |
| `/cookies` | POST | `pinchtab cookies --set 'name=value; domain=.example.com'` |

### Orchestrator Proxying (Instance-scoped)
| Endpoint | HTTP | CLI Command |
|----------|------|-------------|
| `/instances/{id}/navigate` | POST | `pinchtab --instance <id> nav https://example.com` |
| `/instances/{id}/snapshot` | GET | `pinchtab --instance <id> snap -i` |
| `/instances/{id}/screenshot` | GET | `pinchtab --instance <id> ss` |
| `/instances/{id}/action` | POST | `pinchtab --instance <id> action -k click -r e5` |
| `/instances/{id}/tab` | POST | `pinchtab --instance <id> tab create https://example.com` |
| `/instances/{id}/tabs` | GET | `pinchtab --instance <id> tabs` |
| `/instances/{id}/evaluate` | POST | `pinchtab --instance <id> eval 'code'` |
| `/instances/{id}/tabs/{tabId}/pdf` | GET | `pinchtab --instance <id> pdf --tab <tabId> -o out.pdf` |

## Real-World Examples

### Example 1: Launch instance and navigate
```bash
# Start dashboard (separate terminal)
pinchtab

# In another terminal:
INST=$(pinchtab instance launch --mode headed --port 9869 | jq -r .id)
echo "Started instance: $INST"

# Navigate in that instance
pinchtab --instance $INST nav https://github.com/pinchtab/pinchtab
```

### Example 2: Snapshot and click
```bash
# See page structure
pinchtab --instance inst_abc123 snap -i -c | jq .

# Click a button
pinchtab --instance inst_abc123 click e5

# See what changed
pinchtab --instance inst_abc123 snap -d
```

### Example 3: Multi-step workflow
```bash
# Get instances
pinchtab instances

# Check instance status
pinchtab instance inst_abc123 logs

# Run actions on instance
cat << 'EOF' | pinchtab --instance inst_abc123 action
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e1"},
    {"kind": "type", "ref": "e2", "text": "search query"},
    {"kind": "press", "key": "Enter"},
    {"kind": "wait", "time": 2000}
  ]
}
EOF

# Capture result
pinchtab --instance inst_abc123 snap -c > page.json
```

### Example 4: Tab management
```bash
# List tabs
pinchtab --instance inst_abc123 tabs

# Create new tab
TAB_ID=$(pinchtab --instance inst_abc123 tab create https://example.com | jq -r .id)

# Lock tab (prevent other agents from using it)
pinchtab --instance inst_abc123 tab $TAB_ID lock --owner my-agent --ttl 60

# Navigate in locked tab
pinchtab --instance inst_abc123 tab $TAB_ID navigate https://google.com

# Unlock
pinchtab --instance inst_abc123 tab $TAB_ID unlock --owner my-agent

# Close tab
pinchtab --instance inst_abc123 tab $TAB_ID close
```

## Environment Variables

```bash
# Default instance for commands without --instance flag
export PINCHTAB_INSTANCE=inst_abc123

# Server URL/port
export PINCHTAB_URL=http://localhost:9867
export BRIDGE_PORT=9867

# Authentication
export PINCHTAB_TOKEN=sk_...

# Default behavior
export PINCHTAB_FORMAT=json          # json, text, table
export PINCHTAB_NO_COLOR=1           # Disable colored output
export PINCHTAB_TIMEOUT=30           # Request timeout in seconds
```

## Flag Conventions

### Common Flags (all commands)
```
--instance <id>    Target instance (overrides PINCHTAB_INSTANCE)
--tab <id>         Target tab within instance
--timeout N        Request timeout in seconds (default: 30)
--raw              Raw output (no formatting)
--json             Output as JSON
```

### Snapshot Flags
```
-i, --interactive      Interactive elements only
-c, --compact          Compact format (most token-efficient)
-d, --diff             Only changes since last snapshot
-s, --selector CSS     Scope to CSS selector
--max-tokens N         Truncate to ~N tokens
--depth N              Max tree depth
```

### Screenshot Flags
```
-o, --output FILE      Output filename
-q, --quality 0-100    JPEG quality (default: 80)
--format png|jpeg      Output format
```

### PDF Flags
```
-o, --output FILE           Output filename
--landscape                 Landscape orientation
--paper-width N             Width in inches (default: 8.5)
--paper-height N            Height in inches (default: 11)
--margin-top/bottom/left/right N
--scale N                   Print scale 0.1-2.0
--page-ranges "1-3,5"       Pages to export
```

### Navigation Flags
```
--block-images              Skip loading images
--block-ads                 Block ad requests
--block-media               Skip media (video, audio)
--timeout N                 Navigation timeout in seconds
--new-tab                   Open in new tab (for orchestrator)
```

## Error Handling

All commands follow this pattern:
```
exit 0   — Success
exit 1   — User error (bad args, file not found)
exit 2   — Server error (500, connection refused)
exit 3   — Timeout
exit 4   — Instance/resource not found
```

Error messages always printed to stderr:
```bash
pinchtab --instance nonexistent snap 2>&1
# Error: instance not found: nonexistent
# exit: 4
```

## Future Extensibility

This design scales to:
- **Three-level nesting**: `pinchtab instance <id> tab <tabId> screencast <action>`
- **Bulk operations**: `pinchtab instances exec 'snap -c' --all`
- **Piping**: `pinchtab instances | jq '.[] | .id' | xargs -I {} pinchtab instance {} snap`
- **Streaming**: `pinchtab --instance <id> screencast --stream` (server-sent events)
- **REPL mode**: `pinchtab repl --instance <id>` (interactive shell)

## Implementation Checklist

- [ ] Implement `pinchtab instance <id> navigate <url>` (proxy to instance)
- [ ] Implement `pinchtab --instance <id> tab <tabId> navigate <url>`
- [ ] Add `--instance` flag to all browser control commands
- [ ] Support stdin JSON for complex payloads
- [ ] Support `-f` flag for reading payload from file
- [ ] Add error handling with proper exit codes
- [ ] Add `PINCHTAB_INSTANCE` environment variable support
- [ ] Document in help (`pinchtab help`)
- [ ] Add shell completion (`_bash`, `_zsh`)
