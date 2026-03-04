# Breaking Changes - CLI Refactor (feat/cli-refactor branch)

## Summary

This refactor removes browser control from the CLI and expands the configuration system. The HTTP API remains unchanged; only CLI commands are affected.

## Removed CLI Commands

The following CLI commands have been **removed** and functionality moved to the HTTP API:

| Command | Replacement |
|---------|-------------|
| `pinchtab nav <url>` | `POST /navigate` or `POST /tabs/{id}/navigate` |
| `pinchtab snap` | `GET /snapshot` or `GET /tabs/{id}/snapshot` |
| `pinchtab click <ref>` | `POST /action` with `{"kind":"click","ref":"..."}` |
| `pinchtab type <ref> <text>` | `POST /action` with `{"kind":"type","ref":"...","text":"..."}` |
| `pinchtab press <key>` | `POST /action` with `{"kind":"press","key":"..."}` |
| `pinchtab fill <ref> <text>` | `POST /action` with `{"kind":"fill","ref":"...","text":"..."}` |
| `pinchtab hover <ref>` | `POST /action` with `{"kind":"hover","ref":"..."}` |
| `pinchtab scroll <ref\|pixels>` | `POST /action` with `{"kind":"scroll","ref":...}` or `{"scrollY":...}` |
| `pinchtab select <ref> <value>` | `POST /action` with `{"kind":"select","ref":"...","value":"..."}` |
| `pinchtab focus <ref>` | `POST /action` with `{"kind":"focus","ref":"..."}` |
| `pinchtab text` | `GET /text` |
| `pinchtab screenshot` / `pinchtab ss` | `GET /screenshot` |
| `pinchtab eval` | `POST /evaluate` |
| `pinchtab pdf` | `POST /tabs/{id}/pdf` |
| `pinchtab quick` | Use HTTP API directly |
| `pinchtab tab <operation>` | Use tab-scoped endpoints (`/tabs/{id}/*`) |
| `pinchtab instance launch` | Use dashboard or `/instances/launch` endpoint |
| `pinchtab instance navigate` | Use `/tabs/{id}/navigate` endpoint |
| `pinchtab instance logs` | Use `/instances/{id}/logs` endpoint |
| `pinchtab instance stop` | Use `/instances/{id}/stop` endpoint |

## Kept CLI Commands

The following commands are **kept** and work as before:

- `pinchtab` — Start dashboard server
- `pinchtab help` — Show help (updated)
- `pinchtab health` — Server health check
- `pinchtab profiles` — List available profiles
- `pinchtab instances` — List running instances
- `pinchtab tabs` — List open tabs across all instances
- `pinchtab connect <name>` — Get URL for a profile instance
- `pinchtab config init` — Create default config file
- `pinchtab config show` — Display current configuration

## New CLI Commands

The following commands are **new** in this refactor:

- `pinchtab config set <key> <value>` — Set individual config values
- `pinchtab config patch '<json>'` — Merge JSON object into config
- `pinchtab config show --format yaml` — Display config as YAML
- `pinchtab config validate` — Validate configuration file

## Migration Guide

### For Browser Automation

**Before:**
```bash
pinchtab nav https://example.com
pinchtab snap -i -c
pinchtab click e5
pinchtab text
```

**After (using curl):**
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

curl "http://localhost:9867/snapshot?filter=interactive&compact=true"

curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

curl http://localhost:9867/text
```

**After (using Playwright):**
```python
from pinchtab import Pinchtab

ptab = Pinchtab("http://localhost:9867")
ptab.navigate("https://example.com")
snapshot = ptab.snapshot(interactive=True, compact=True)
ptab.click("e5")
text = ptab.text()
```

### For Configuration

**Before:**
```bash
# Only env vars and config init
export BRIDGE_PORT=9999
export BRIDGE_HEADLESS=false
pinchtab config init
```

**After:**
```bash
# Use new config commands
pinchtab config init
pinchtab config set server.port 9999
pinchtab config set chrome.headless false
pinchtab config show --format yaml
pinchtab config validate
```

## Plugin Removal

The `/plugin` folder has been removed. This will be re-added in a future version with improved structure.

## Config Structure

New config sections available:
- `server.*` — Server settings (port, stateDir, profileDir, token, cdpUrl)
- `chrome.*` — Browser settings (headless, maxTabs, noRestore)
- `orchestrator.*` — Instance management (strategy, allocationPolicy, ports)
- `timeouts.*` — Timing settings (actionSec, navigateSec)

## Rationale

1. **Separation of concerns**: CLI handles management, HTTP API handles browser control
2. **Simpler CLI**: Easier to document and maintain
3. **Better config system**: Similar to `openclaw config` - consistent with other tools
4. **HTTP API untouched**: No breaking changes to automation workflows
5. **Upgrade path**: Users can continue using HTTP API or migrate to client libraries

## Testing

- All existing HTTP API tests pass ✅
- New CLI management commands tested ✅
- New config system tested with 18 dedicated tests ✅
- Total: 33 tests passing

## Affected Users

- **Scripts using CLI commands**: Migrate to HTTP API or use client libraries
- **Configuration management**: Switch to `config set/patch` commands (optional; env vars still work)
- **OpenClaw integration**: No changes; use HTTP API

## Questions?

See `/docs` or reach out on GitHub Issues.
