# Pinchtab npm Package — Test Plan

## Scope
Test that the npm package correctly:
1. Finds and executes the Pinchtab binary
2. Manages process lifecycle (start/stop)
3. Communicates with the API via HTTP
4. Handles errors gracefully

## Test Cases

### 1. Binary Discovery
- ✓ Binary exists at `~/.pinchtab/bin/pinchtab-<os>-<arch>`
- ✓ CLI wrapper finds and executes binary
- ✗ Graceful error if binary missing

### 2. Process Lifecycle
- ✓ `start()` spawns server on correct port
- ✓ Server responds to health check (any endpoint)
- ✓ `stop()` kills process cleanly
- ✗ `start()` twice throws error
- ✗ `stop()` when not running is no-op

### 3. API Requests
- ✓ `snapshot()` returns valid response
- ✓ `click()` executes without error
- ✓ `lock()` / `unlock()` work
- ✓ `createTab()` returns tab ID
- ✗ Request timeout after 30s

### 4. Error Handling
- ✗ Invalid endpoint returns 404
- ✗ Malformed params rejected
- ✗ Server crashes → clear error message

## Environment
- Binary must exist: `~/.pinchtab/bin/pinchtab-darwin-arm64` (or platform equivalent)
- Port 9867 must be available
- Test runner: Node.js native `test` module

## Execution
```bash
npm test
```

---

*Add new test cases as you discover edge cases.*
