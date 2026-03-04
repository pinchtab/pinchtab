# Quick Start Manual Test (6 Steps)

Manual step-by-step test to verify orchestrator functionality: instance creation and shorthand endpoints.

**Duration:** ~5 minutes  
**Requirements:** PinchTab built, ports 9867-9968 available, Chrome installed  
**Updated:** 2026-03-04 (uses orchestrator shorthand endpoints)

---

## Prerequisites

```bash
cd ~/dev/pinchtab
go build -o pinchtab ./cmd/pinchtab
```

---

## Test Steps

### 1. Start PinchTab

```bash
./pinchtab &
sleep 2
```

**Verify:**
- ✓ Dashboard logs: "🦀 PinchTab port=9867"
- ✓ Can access http://localhost:9867/health

---

### 2. Create Instance 1 (Headed)

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test-headed","headless":false}' | jq '.'
```

**Expected response:**
```json
{
  "id": "inst_XXXXXXXX",
  "profileId": "prof_YYYYYYYY",
  "profileName": "test-headed",
  "port": "9868",
  "headless": false,
  "status": "starting"
}
```

**Verify:**
- ✓ Hash-based instance ID (inst_XXXXXXXX format)
- ✓ Port allocated (9868)
- ✓ Chrome window should open on your screen

---

### 3. Create Instance 2 (Headless)

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test-headless","headless":true}' | jq '.'
```

**Expected response:** Similar to step 2, but different port (9869)

**Verify:**
- ✓ Different instance ID
- ✓ Port incremented (9869)
- ✓ No window opens (headless)

---

### 4. Use Orchestrator Shorthand: Navigate

Navigate automatically creates a tab on the current instance:

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq '.'
```

**Expected response:**
```json
{
  "tabId": "tab_MMMMMMMM",
  "url": "https://example.com"
}
```

**Verify:**
- ✓ Tab created automatically
- ✓ Instance 1 (headed) navigates to example.com
- ✓ Tab ID returned

---

### 5. Use Find Endpoint

Find searches for elements on the current tab:

```bash
curl -X POST http://localhost:9867/find \
  -H "Content-Type: application/json" \
  -d '{"text":"example"}' | jq '.'
```

**Expected response:**
```json
{
  "refs": [
    {"ref": "e1", "text": "..."},
    ...
  ]
}
```

**Verify:**
- ✓ Find endpoint responds with element references
- ✓ Can search for elements by text

---

### 6. Stop Instances & Verify Cleanup

```bash
# Get instance IDs
curl -s http://localhost:9867/instances | jq '.'

# Stop instance 1 (replace with actual ID)
curl -X POST http://localhost:9867/instances/inst_XXXXXXXX/stop

# Stop instance 2
curl -X POST http://localhost:9867/instances/inst_YYYYYYYY/stop

# Verify cleanup
curl -s http://localhost:9867/instances | jq '.'
```

**Verify:**
- ✓ Both instances stopped
- ✓ Instance list empty (or cleaning up)
- ✓ All ports released

---

## Summary

✅ If all steps pass:
- Instance creation works (headed + headless)
- Port allocation works
- Orchestrator shorthand endpoints work (/navigate, /find)
- Automatic tab creation works
- Instance isolation verified
- Cleanup works correctly

**Expected total time:** ~5 minutes

---

## Orchestrator Shorthand Endpoints

These endpoints automatically manage instances and tabs for you:

- **POST /navigate** — Navigate to URL (auto-creates tab, allocates instance)
- **POST /find** — Find elements on current tab
- **GET /snapshot** — Get page snapshot
- **GET /screenshot** — Get page screenshot
- **POST /action** — Interact with element
- **POST /tab** — Create/close tab ({action: "new"|"close"})
- **GET /tabs** — List all tabs
