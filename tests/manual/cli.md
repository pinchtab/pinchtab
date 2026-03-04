# Manual CLI Testing Plan

This document outlines manual testing procedures for the PinchTab CLI. These tests verify the management commands and configuration system work correctly.

## Prerequisites

- PinchTab binary built and available (`go build ./cmd/pinchtab`)
- Separate terminal for server operations
- Basic Unix tools (curl, jq optional)

---

## Configuration Commands

### Test 1: Initialize Config File

**Command:**
```bash
pinchtab config init
```

**Expected Result:**
- ✅ Creates config file at `~/.config/pinchtab/config.json` (macOS/Linux) or `%APPDATA%\pinchtab\config.json` (Windows)
- ✅ Contains default values (port 9867, headless true, etc.)
- ✅ Output: `✅ Config file created at ...`

**Manual Verification:**
```bash
cat ~/.config/pinchtab/config.json | jq .
# Should see: port, headless, stateDir, profileDir, etc.
```

---

### Test 2: Display Config (JSON Format)

**Command:**
```bash
pinchtab config show
```

**Expected Result:**
- ✅ Outputs config as formatted JSON
- ✅ All fields visible: port, headless, maxTabs, etc.
- ✅ No errors

**Manual Verification:**
```bash
pinchtab config show | jq . | head -10
# Verify JSON is valid and readable
```

---

### Test 3: Display Config (YAML Format)

**Command:**
```bash
pinchtab config show --format yaml
```

**Expected Result:**
- ✅ Outputs config as YAML
- ✅ Same fields as JSON, different format
- ✅ YAML syntax valid (key: value)

**Manual Verification:**
```bash
pinchtab config show --format yaml
# Verify YAML indentation and formatting
```

---

### Test 4: Set Single Config Value

**Command:**
```bash
pinchtab config set server.port 9999
```

**Expected Result:**
- ✅ Output: `✅ Set server.port = 9999`
- ✅ Config file updated with new port
- ✅ Next config show displays new port

**Manual Verification:**
```bash
pinchtab config show | jq '.port'
# Should output: "9999"
```

---

### Test 5: Set Multiple Config Values

**Commands:**
```bash
pinchtab config set chrome.headless false
pinchtab config set chrome.maxTabs 50
pinchtab config set orchestrator.strategy session
pinchtab config set timeouts.actionSec 30
```

**Expected Result:**
- ✅ Each command outputs `✅ Set <key> = <value>`
- ✅ All values persist in config file
- ✅ Next config show displays all updates

**Manual Verification:**
```bash
pinchtab config show | jq '{headless, maxTabs, strategy, timeoutSec}'
# Should show updated values
```

---

### Test 6: Patch Config with JSON

**Command:**
```bash
pinchtab config patch '{"port": "8888", "maxTabs": 100}'
```

**Expected Result:**
- ✅ Output: `✅ Config patched successfully`
- ✅ Multiple values updated in one operation
- ✅ Other values unchanged

**Manual Verification:**
```bash
pinchtab config show | jq '{port, maxTabs}'
# Should show: port: "8888", maxTabs: 100
# Other fields unchanged
```

---

### Test 7: Validate Config

**Command:**
```bash
pinchtab config validate
```

**Expected Result (valid config):**
- ✅ Output: `✅ Config is valid`
- ✅ Exit code 0

**Test Invalid Config:**
```bash
pinchtab config set orchestrator.strategy invalid
pinchtab config validate
```

**Expected Result (invalid config):**
- ❌ Output: `❌ Config validation failed:` with error list
- ❌ Exit code 1
- ❌ Error message: invalid strategy option

---

### Test 8: Config with Environment Variables

**Command:**
```bash
BRIDGE_CONFIG=/tmp/test-config.json pinchtab config init
```

**Expected Result:**
- ✅ Creates config at custom location
- ✅ All subsequent commands use custom location

**Manual Verification:**
```bash
ls -la /tmp/test-config.json
# Should exist with valid content
```

---

## Management Commands (with Running Server)

### Setup: Start Server

**Terminal 1:**
```bash
pinchtab
# Server starts on http://localhost:9867
```

---

### Test 9: Health Check

**Terminal 2:**
```bash
pinchtab health
```

**Expected Result:**
- ✅ Output: `✅ Server is healthy`
- ✅ Shows mode (dashboard or bridge)
- ✅ Exit code 0

**If server is down:**
- ❌ Output: `❌ Connection error...`
- ❌ Suggests starting the server
- ❌ Exit code 1

---

### Test 10: List Profiles

**Command:**
```bash
pinchtab profiles
```

**Expected Result:**
- ✅ Lists available profiles (at least 1)
- ✅ Format: `👤 profile-name`
- ✅ Shows all profiles in dashboard

---

### Test 11: List Instances

**Command:**
```bash
pinchtab instances
```

**Expected Result (no instances running):**
- ✅ Output: `No instances running`
- ✅ Suggestions to launch instances

**Expected Result (with instances):**
- ✅ Lists all running instances
- ✅ Shows: ID, port, mode (headless/headed), status
- ✅ Example: `▶️ inst-abc123 (port 9868, headless)`

---

### Test 12: List Tabs

**Command:**
```bash
pinchtab tabs
```

**Expected Result (no tabs):**
- ✅ Output: `No tabs open across all instances`

**Expected Result (with tabs):**
- ✅ Lists all tabs across all instances
- ✅ Shows: tab ID, title, URL
- ✅ Format: `[tab-1] Page Title` → `https://example.com`

---

### Test 13: Connect to Instance

**Prerequisites:**
- Instance running with a profile (from dashboard)

**Command:**
```bash
pinchtab connect myprofile
```

**Expected Result:**
- ✅ Outputs instance URL: `http://localhost:9868`
- ✅ Can use URL for HTTP API calls
- ✅ Exit code 0

**If profile not running:**
- ❌ Error: `profile "myprofile" not running...`
- ❌ Exit code 1

---

## Edge Cases & Error Handling

### Test 14: Invalid Command

**Command:**
```bash
pinchtab invalid
```

**Expected Result:**
- ❌ Unknown command message
- ❌ Shows help
- ❌ Exit code 1

---

### Test 15: Config Set with Invalid Key

**Command:**
```bash
pinchtab config set invalid.key value
```

**Expected Result:**
- ❌ Output: `❌ unknown section: invalid`
- ❌ Exit code 1

---

### Test 16: Config Patch with Invalid JSON

**Command:**
```bash
pinchtab config patch 'not-valid-json'
```

**Expected Result:**
- ❌ Output: `❌ invalid JSON: ...`
- ❌ Exit code 1

---

### Test 17: Config Show with Invalid Format

**Command:**
```bash
pinchtab config show --format xml
```

**Expected Result:**
- ❌ Output: `❌ unknown format: xml (use json or yaml)`
- ❌ Exit code 1

---

### Test 18: Missing Required Args

**Commands:**
```bash
pinchtab config set server.port        # Missing value
pinchtab config patch                  # Missing JSON
```

**Expected Result:**
- ❌ Usage error with instructions
- ❌ Exit code 1

---

## Automated Test Coverage

The following tests are covered by unit tests in `internal/config/config_editor_test.go` and `cmd/pinchtab/cmd_cli_test.go`:

- ✅ `TestSetConfigValue` — All config sections
- ✅ `TestPatchConfigJSON` — JSON merging
- ✅ `TestValidateConfig` — Validation rules
- ✅ `TestDisplayConfigJSON` — JSON output
- ✅ `TestDisplayConfigYAML` — YAML output
- ✅ `TestCLIHealth` — Health check
- ✅ `TestCLIProfiles` — Profile listing
- ✅ `TestCLIInstances` — Instance listing
- ✅ `TestCLITabs` — Tab listing
- ✅ `TestAuthHeader` — Token authentication

Run with:
```bash
go test ./internal/config ./cmd/pinchtab -v
```

---

## Configuration Integration

### Test 19: Config Persistence Across Restarts

**Steps:**
1. Set a config value: `pinchtab config set server.port 9999`
2. Verify: `pinchtab config show | jq .port`
3. Close terminal (if running server)
4. Start new session
5. Verify: `pinchtab config show | jq .port` still shows 9999

**Expected Result:**
- ✅ Config persists in file
- ✅ Value unchanged after restart

---

### Test 20: Environment Variable Override

**Steps:**
1. Set in config: `pinchtab config set server.port 9999`
2. Start with env var: `BRIDGE_PORT=8888 pinchtab`
3. Check startup messages or use `pinchtab health`

**Expected Result:**
- ✅ Server starts on port 8888 (env var wins)
- ✅ Config file still shows 9999 (not modified)

---

## Summary

| Test | Manual | Automated | Status |
|------|--------|-----------|--------|
| Config init | ✅ | ✅ | ✓ |
| Config show (JSON) | ✅ | ✅ | ✓ |
| Config show (YAML) | ✅ | ✅ | ✓ |
| Config set | ✅ | ✅ | ✓ |
| Config patch | ✅ | ✅ | ✓ |
| Config validate | ✅ | ✅ | ✓ |
| Health check | ✅ | ✅ | ✓ |
| List profiles | ✅ | ✅ | ✓ |
| List instances | ✅ | ✅ | ✓ |
| List tabs | ✅ | ✅ | ✓ |
| Connect instance | ✅ | — | ✓ |
| Error handling | ✅ | ✅ | ✓ |
| Persistence | ✅ | — | ✓ |
| Env var override | ✅ | — | ✓ |

---

## Checklist for Release

- [ ] All 20 manual tests pass
- [ ] All automated tests pass (`go test ./...`)
- [ ] No regressions in existing functionality
- [ ] Help text is clear and up-to-date
- [ ] BREAKING_CHANGES.md documents removed commands
- [ ] README reflects new CLI model
- [ ] Environment variable docs updated

---

## Common Issues & Troubleshooting

### Config file not found
```bash
# Check location
echo $HOME/.config/pinchtab/config.json
ls -la ~/.config/pinchtab/config.json

# Or use custom location
BRIDGE_CONFIG=/custom/path pinchtab config init
```

### Server won't start
```bash
# Check port in use
lsof -i :9867

# Use different port
pinchtab config set server.port 9868
```

### Config validation fails
```bash
# Check what's wrong
pinchtab config validate

# Reset to defaults
pinchtab config init
rm ~/.config/pinchtab/config.json
```

---

## Notes

- All tests should pass with exit code 0 (or expected error code)
- Timestamps in this plan: 2026-03-04
- Browser automation tests use the HTTP API, not CLI commands
- See `/docs/references/cli-quick-reference.md` for API examples
