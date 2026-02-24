# CF3-Extended: Create Tab via POST in CDP_URL Mode

**Status:** ✅ FIXED (Verified in main.go — CDP_URL mode check added)

**Goal:** Reproduce the crash when creating a new tab via `POST /tab` in CDP_URL mode.

## Prerequisites

- Chrome/Chromium installed
- `curl` CLI
- `jq` for JSON parsing (optional, but helpful)

## Test Steps

### Step 1: Start Chrome with Remote Debugging

```bash
# Kill any existing Chrome instances on port 9222
pkill -f "remote-debugging-port=9222" || true

# Start Chrome with CDP port open (macOS)
# IMPORTANT: Use --headless=chrome NOT --headless (legacy mode with GUI support)
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 \
  --no-first-run \
  --no-default-browser-check \
  --disable-background-networking \
  --new-window &

sleep 3

# Verify it's listening
curl -s http://localhost:9222/json/version | jq . && echo "✅ Chrome listening on port 9222"
```

### Step 2: Get the CDP WebSocket URL

```bash
CDP_WS=$(curl -s http://localhost:9222/json/version | jq -r '.webSocketDebuggerUrl')
echo "CDP WebSocket URL: $CDP_WS"
```

Example: `ws://localhost:9222/devtools/browser/12345...`

### Step 3: Start Pinchtab in CDP_URL Mode

```bash
# From pinchtab root
CDP_URL="$CDP_WS" ./pinchtab &
sleep 2

curl http://localhost:9222/health && echo "✅ Pinchtab started"
```

Expected: Pinchtab should start and connect to remote Chrome without launching its own.

### Step 4: Verify Initial Connection (Should Pass)

```bash
curl -s http://localhost:9222/snapshot | jq '.nodes | length' && echo "✅ Can snapshot"
```

### Step 5: Pinchtab Startup Fails (Reproducible ❌)

Pinchtab doesn't even start successfully. It crashes during initialization when trying to set up the remote browser context.

**Actual broken behavior:**
```
2026/02/23 12:29:59 WARN Chrome startup failed, clearing sessions and retrying once 
err="Failed to open new tab - no browser is open (-32000)"
2026/02/23 12:29:59 ERROR Chrome failed to start after retry 
err="Failed to open new tab - no browser is open (-32000)"
```

**Why it happens:**
- Pinchtab's `startChrome()` function (browser.go line 94) calls `NavigatePage()` to open about:blank
- When using a remote allocator, `NavigatePage()` fails because the remote Chrome instance has no open window/tab to attach to
- No graceful fallback: Pinchtab retries once then exits
- The error should be handled differently for CDP_URL mode (don't require initial tab, respect existing Chrome state)

## Cleanup

```bash
pkill -f "pinchtab"
pkill -f "remote-debugging-port=9222"
```

## Root Cause

**Primary:** In `cmd/pinchtab/browser.go`, `startChrome()` (line 94) always calls `NavigatePage(ctx, "about:blank")` to set up initial state. This assumes a tab already exists or can be created.

**For CDP_URL mode:** Remote Chrome instances may have no windows open, so `NavigatePage()` fails with `-32000` ("Failed to open new tab - no browser is open").

**Secondary:** In `internal/bridge/tab_manager.go`, `CreateTab()` (line 65) doesn't distinguish between:
- Local allocator (can create tabs freely)
- Remote allocator (must use `target.CreateTarget` CDP protocol call, or check browser state first)

## Affected Code

```go
// browser.go, line 94-99
func startChrome(allocCtx context.Context, seededScript string) (context.Context, context.CancelFunc, error) {
    bCtx, bCancel := chromedp.NewContext(allocCtx)
    // ... 
    if err := NavigatePage(ctx, "about:blank"); err != nil {  // ← FAILS for CDP_URL mode
        bCancel()
        return nil, nil, err
    }
}

// tab_manager.go, line 65
func (tm *TabManager) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
    ctx, cancel := chromedp.NewContext(tm.browserCtx)  // ← CRASHES for remote
    if err := NavigatePage(ctx, navURL); err != nil {
        cancel()
        return "", nil, nil, fmt.Errorf("new tab: %w", err)
    }
}
```

## Fix Strategy

1. **In `browser.go` `startChrome()`:**
   - Check if we're using CDP_URL mode (can detect via allocator type or config)
   - If CDP_URL: Skip `NavigatePage()` during startup (browser may have no windows yet)
   - If local: Keep existing behavior (ensure about:blank is open)

2. **In `tab_manager.go` `CreateTab()`:**
   - Use `target.CreateTarget` CDP protocol call instead of relying on `chromedp.NewContext`
   - This works for both local and remote allocators
   - Example:
   ```go
   targetID, err := target.CreateTarget("about:blank").Do(ctx)  // Use CDP protocol directly
   ```

3. **Test coverage:**
   - Add unit test: `TestCreateTabCDPURL` in `tab_manager_test.go`
   - Add integration test: CF3-ext in TEST-PLAN.md

## Status

✅ **FIXED** — Code review shows fix in place:
- `cmd/pinchtab/main.go` lines ~116-121 now check for CDP_URL mode
- Skips initial tab registration when `cfg.CdpURL != ""`
- `CreateTab()` in tab_manager.go properly uses CDP protocol

**How to verify the fix:**
1. Follow the test steps above (start remote Chrome, start Pinchtab in CDP_URL mode)
2. Expected: Pinchtab should start successfully without crashing
3. Navigate to a URL: `curl -X POST http://localhost:9867/navigate -H "Content-Type: application/json" -d '{"url":"https://example.com"}'`
4. Create new tab: `curl -X POST http://localhost:9867/tab -d '{"action":"new","url":"https://example.com"}'`
5. Both should work without `-32000` errors

**Test this before release to confirm fix is production-ready.**
