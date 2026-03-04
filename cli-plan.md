# CLI Refactor Plan: cmd/pinchtab Simplification & Config Management

## Overview
Simplify the pinchtab CLI by removing browser control commands and expanding the config system to be more like `openclaw config`. Focus on monitoring and management.

## Phase 1: Inventory & Planning ✅ (This PR)

### Commands to Keep (Monitoring/Management)
- `help`
- `health` — Server health check
- `config` — Configuration management (to be expanded)
- `profiles` — List available profiles
- `instances` — List running instances
- `tabs` — List open tabs (global)
- `connect` — Get URL for running profile

### Commands to Remove (Browser Control)
These delegate to individual bridge instances. We're moving this responsibility to:
- Direct instance API calls via HTTP
- Higher-level strategies (simple, session, explicit)
- Tools/SDKs that speak to `/tabs/{id}/*` endpoints

**Removing:**
- `nav, navigate` — Navigate to URL
- `snap, snapshot` — Page structure snapshot
- `click` — Click element by ref
- `type` — Type text into element
- `fill` — Fill input field
- `press` — Press key
- `hover` — Hover element
- `scroll` — Scroll page
- `select` — Select dropdown option
- `focus` — Focus element
- `text` — Extract page text
- `screenshot, ss` — Take screenshot
- `eval, evaluate` — Run JavaScript
- `pdf` — Export page as PDF
- `quick` — Navigate + analyze (beginner-friendly)
- `tab <operation>` — Tab-scoped operations
- `instance launch, instance navigate, instance logs, instance stop` — These become admin tasks or go elsewhere

**Rationale:** Browser control is better handled by:
1. API clients calling `/tabs/{id}/*` directly
2. Higher-level orchestration frameworks
3. Playwright/Puppeteer/Cypress via PINCHTAB_URL

---

## Phase 2: Configuration System Expansion

### Current State
- **Format:** JSON only (config.json)
- **Location:** `~/.pinchtab/config.json` (with OS-specific fallbacks)
- **Methods:** Environment variables override file config
- **Commands:** `config init` and `config show`

### Target State (Like `openclaw config`)

#### 1. Interactive Configuration
```bash
# Interactive mode (TUI, guided wizard)
pinchtab configure --section server
pinchtab configure --section chrome
pinchtab configure --section orchestrator
pinchtab configure --interactive
```

#### 2. Direct Key-Value Setting
```bash
# Set scalar values
pinchtab config set server.port "9867"
pinchtab config set chrome.headless true
pinchtab config set orchestrator.strategy "session"
pinchtab config set orchestrator.allocationPolicy "round_robin"

# Set array values
pinchtab config set chrome.flags '["--no-sandbox", "--disable-gpu"]'
```

#### 3. Patch (JSON/YAML Object Merge)
```bash
# Patch with JSON
pinchtab config patch '{ "chrome": { "headless": false, "maxTabs": 50 } }'

# Patch with YAML (if supported)
pinchtab config patch '
chrome:
  headless: false
  maxTabs: 50
'
```

#### 4. Verification & Dry-Run
```bash
# Verify config (check it's valid)
pinchtab doctor

# Validate without applying
pinchtab config set server.port "9867" --dry-run

# Show current config
pinchtab config show [--format json|yaml]
```

#### 5. Config Sections

**server:** Port, bind address, state directory
```json
{
  "server": {
    "port": "9867",
    "bind": "127.0.0.1",
    "stateDir": "~/.config/pinchtab",
    "token": "..."
  }
}
```

**chrome:** Browser settings
```json
{
  "chrome": {
    "headless": true,
    "maxTabs": 20,
    "binary": "/path/to/chrome",
    "profileDir": "~/.config/pinchtab/chrome-profile",
    "blockImages": false,
    "blockAds": false,
    "blockMedia": false,
    "noAnimations": false,
    "noRestore": false,
    "stealthLevel": "light",
    "timezone": "UTC",
    "userAgent": "...",
    "flags": ["--no-sandbox"]
  }
}
```

**orchestrator:** Dashboard mode settings
```json
{
  "orchestrator": {
    "strategy": "simple|session|explicit",
    "allocationPolicy": "fcfs|round_robin|random",
    "instancePortStart": 9868,
    "instancePortEnd": 9968
  }
}
```

**timeouts:** Duration settings
```json
{
  "timeouts": {
    "actionSec": 30,
    "navigateSec": 60,
    "shutdownSec": 10,
    "waitNavDelaySec": 1
  }
}
```

---

## Phase 3: Implementation Tasks

### 3a. Add config CLI commands
- [ ] `config set <key> <value>` — Set scalar/array config
- [ ] `config patch <json|yaml>` — Merge config object
- [ ] `config show [--format json|yaml]` — Display current config
- [ ] `config validate` — Check config validity
- [ ] `config reset` — Reset to defaults
- [ ] `configure --interactive` — TUI-driven config (optional Phase 2)

### 3b. Support JSON & YAML
- [ ] Detect format by file extension or parse both
- [ ] Serialize to user's preferred format (respect existing config format)
- [ ] Ensure backward compatibility: JSON -> read, write what was there

### 3c. Update codebase
- [ ] Refactor `cmd/pinchtab/main.go` to remove browser control
- [ ] Update `cmd/pinchtab/cmd_cli.go` — remove old commands, keep monitoring
- [ ] Refactor `main.go` main() to handle only:
  - help
  - config
  - health
  - profiles
  - instances
  - tabs
  - connect
- [ ] Update help text to reflect changes
- [ ] Update CONTRIBUTING.md / docs with new CLI model

### 3d. Testing
- [ ] Unit tests for `config set`, `config patch`
- [ ] Config validation tests
- [ ] Integration tests: config persistence, env var override
- [ ] Backward compatibility: old JSON config still works

### 3e. Documentation
- [ ] Update README with new CLI examples
- [ ] Document config sections and all available keys
- [ ] Add migration guide (if breaking)
- [ ] Document env var precedence: ENV > file > defaults

---

## Format Support Decision

**Current:** JSON only
**Decision:** Start with JSON + YAML support (read both, write to existing format)
- Rationale: openclaw uses both, users expect flexibility
- Implementation: Use yaml.v3 library (standard in Go)
- Backwards compat: If config.json exists, keep writing JSON

---

## Test Plan for PR

### Unit Tests
```go
// config/config_test.go additions
func TestConfigSet(t *testing.T) { ... }
func TestConfigPatch(t *testing.T) { ... }
func TestConfigValidate(t *testing.T) { ... }
func TestConfigYAML(t *testing.T) { ... }
```

### Integration Tests
```bash
# Test 1: Set and persist
pinchtab config set server.port "9999"
pinchtab config show | grep "9999"

# Test 2: Patch and merge
pinchtab config patch '{"chrome": {"headless": false}}'
# Verify headless is false, other settings unchanged

# Test 3: Env var override still works
BRIDGE_PORT=8888 pinchtab health
# Should use 8888, not config file

# Test 4: YAML support
echo 'server:
  port: 9999' > config.yaml
# Should parse and use

# Test 5: Validation
pinchtab config set server.port "invalid" # Should error
pinchtab config validate
```

### Manual Testing Scenarios
1. Fresh install → generate default config
2. Update config with `set` → changes persist
3. Use `patch` with complex object → merges correctly
4. Show with `--format yaml` → valid YAML output
5. Env var override still beats file config
6. Doctor reports config status

---

## Next Steps

1. **Now:** Save this plan to `cli-plan.md` ✅
2. **PR:** Open draft PR with:
   - This plan file
   - Removed browser control commands from help text
   - Skeleton for new `config set` / `config patch` (not implemented yet)
3. **Review:** Get your approval on the plan
4. **Implement:** Start Phase 3 in next iteration

---

## Questions for Luigi

1. Should `configure --interactive` be in Phase 1 or deferred to Phase 2?
2. For env var override behavior: keep current (ENV > file > defaults)?
3. Should we support both `.pinchtab/config.json` and `.pinchtab/config.yaml`?
4. Any sections I'm missing from the config?
5. Timeline: Phase 1 this PR, Phases 2-3 next PR(s)?

---

## Status

**Branch:** `feat/cli-refactor` (based on `feat/allocation-strategies`)

### Phase 1: CLI Simplification ✅ Complete
- [x] Removed all browser control CLI commands (nav, snap, click, type, fill, press, hover, scroll, select, focus, text, screenshot, eval, pdf, quick, tab operations, instance launch/navigate/logs/stop)
- [x] Kept monitoring commands: help, health, config, profiles, instances, tabs, connect
- [x] Updated help text with clear guidance to HTTP API and client libraries
- [x] Simplified cmd_cli.go to ~250 lines (was ~1700)
- [x] Updated main.go to handle only monitoring commands
- [x] Updated cmd_cli_test.go with new test suite (15 tests, all passing)
- [x] Plugin folder removed

**Commit:** `9cae4f5` + `f5187a1`

### Phase 2: Configuration System Expansion ✅ Complete
- [x] Created config_editor.go with 300+ lines of functionality
- [x] Implemented `config set <key> <value>` for scalar and nested values
- [x] Implemented `config patch '<json>'` for object merging
- [x] Implemented `config show [--format json|yaml]` with dual format support
- [x] Implemented `config validate` with validation rules
- [x] Implemented `config init` with proper path resolution
- [x] Support for config sections:
  - `server.*` (port, stateDir, profileDir, token, cdpUrl)
  - `chrome.*` (headless, maxTabs, noRestore)
  - `orchestrator.*` (strategy, allocationPolicy, instancePortStart/End)
  - `timeouts.*` (actionSec, navigateSec)
- [x] Config validation:
  - Port required
  - Port range validation (start < end)
  - Timeout non-negative
  - Strategy must be: simple, session, explicit
  - AllocationPolicy must be: fcfs, round_robin, random
- [x] Support BRIDGE_CONFIG environment variable
- [x] Comprehensive test suite (18 config tests + 15 CLI tests = 33 total)
- [x] All tests passing
- [x] Pre-commit checks passing

**Commit:** `44215d8`

### What's Left
Phase 3: (Optional, deferred)
- Interactive config mode with `configure --interactive` or `configure --section`
- This can be added later if needed
