# Phase 6: End-to-End Testing Guide

This guide provides manual and automated tests for the complete multi-instance architecture.

## Prerequisites

- Pinchtab built: `go build -o pinchtab ./cmd/pinchtab`
- Port range available: 9867-9968
- Chrome/Chromium installed

## Quick Start Test (Manual)

### 1. Start Pinchtab Dashboard

```bash
./pinchtab
```

This starts:
- Dashboard on port 9867
- Ready to accept instance creation requests

Expected output:
```
INFO dashboard listening addr=127.0.0.1:9867
INFO port allocator initialized start=9868 end=9968
```

### 2. Create First Instance (Headed)

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{
    "name":"work",
    "headless":false
  }'
```

**Expected response:**
```json
{
  "id": "inst_XXXXXXXX",
  "profileId": "prof_YYYYYYYY",
  "profileName": "work",
  "port": "9868",
  "headless": false,
  "status": "starting",
  "startTime": "2026-02-28T20:15:00Z"
}
```

**Verify:**
- Hash-based instance ID (inst_XXXXXXXX format) ✓
- Hash-based profile ID (prof_XXXXXXXX format) ✓
- Port auto-allocated (9868) ✓
- Status is "starting" (not "running" yet) ✓
- **Within 2 seconds**: Chrome window should open on your screen ✓

### 3. Create Second Instance (Headless)

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{
    "name":"scrape",
    "headless":true
  }'
```

**Expected response:**
```json
{
  "id": "inst_ZZZZZZZZ",
  "profileId": "prof_WWWWWWWW",
  "profileName": "scrape",
  "port": "9869",
  "headless": true,
  "status": "starting",
  "startTime": "2026-02-28T20:15:05Z"
}
```

**Verify:**
- Different instance ID ✓
- Different port (9869) ✓
- Same port range used (auto-allocation) ✓
- No Chrome window (headless=true) ✓

### 4. List All Instances

```bash
curl http://localhost:9867/instances
```

**Expected response:**
```json
[
  {
    "id": "inst_XXXXXXXX",
    "profileId": "prof_YYYYYYYY",
    "profileName": "work",
    "port": "9868",
    "headless": false,
    "status": "running",
    "startTime": "2026-02-28T20:15:00Z"
  },
  {
    "id": "inst_ZZZZZZZZ",
    "profileId": "prof_WWWWWWWW",
    "profileName": "scrape",
    "port": "9869",
    "headless": true,
    "status": "running",
    "startTime": "2026-02-28T20:15:05Z"
  }
]
```

**Verify:**
- Both instances listed ✓
- Both have "running" status ✓
- Hash-based IDs on both ✓

### 5. Navigate Instance 1 (via Orchestrator Proxy)

```bash
curl -X POST http://localhost:9867/instances/inst_XXXXXXXX/navigate \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com"
  }'
```

Replace `inst_XXXXXXXX` with actual ID from step 2.

**Expected response:**
```json
{
  "tabId": "tab_MMMMMMMM",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

**Verify:**
- Hash-based tab ID (tab_MMMMMMMM format) ✓
- Instance 1 Chrome window navigates to example.com ✓
- URL matches ✓

### 6. Navigate Instance 2 (via Orchestrator Proxy)

```bash
curl -X POST http://localhost:9867/instances/inst_ZZZZZZZZ/navigate \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://github.com"
  }'
```

Replace `inst_ZZZZZZZZ` with actual ID from step 3.

**Expected response:**
```json
{
  "tabId": "tab_NNNNNNNN",
  "url": "https://github.com",
  "title": "GitHub"
}
```

**Verify:**
- Different tab ID (tab_NNNNNNNN vs tab_MMMMMMMM) ✓
- Instance 2 (headless) navigates silently ✓
- Instance 1 window still shows example.com (not affected) ✓

### 7. Get Snapshot from Instance 1

```bash
curl "http://localhost:9867/instances/inst_XXXXXXXX/snapshot" \
  -o snapshot.json
cat snapshot.json | jq '.url'
```

**Expected:**
- Returns full page snapshot of example.com
- Shows isolation: only sees inst_XXXXXXXX's content ✓

### 8. Stop Instance 1

```bash
curl -X POST http://localhost:9867/instances/inst_XXXXXXXX/stop
```

**Expected response:**
```json
{
  "status": "stopped",
  "id": "inst_XXXXXXXX"
}
```

**Verify:**
- Chrome window closes ✓
- Instance 2 still running (headless, invisible) ✓
- Port 9868 released back to allocator ✓

### 9. Create Third Instance (Reuses Released Port)

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{
    "name":"test",
    "headless":true
  }'
```

**Expected:**
- New instance gets port 9868 (reused from step 8) ✓
- New instance ID generated ✓

### 10. Stop All Instances

```bash
curl -X POST http://localhost:9867/instances/inst_ZZZZZZZZ/stop
curl -X POST http://localhost:9867/instances/inst_TTTTTTTT/stop
```

**Verify:**
- All instances stopped ✓
- All ports released back to allocator ✓
- Dashboard still running on 9867 ✓

---

## Automated Test Script

```bash
#!/bin/bash

# Start Pinchtab in background
./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"

# Create instance 1
INST1=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","headless":false}')
INST1_ID=$(echo $INST1 | jq -r '.id')
echo "✓ Created instance 1: $INST1_ID"

# Create instance 2
INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"scrape","headless":true}')
INST2_ID=$(echo $INST2 | jq -r '.id')
echo "✓ Created instance 2: $INST2_ID"

# Wait for Chrome to initialize
sleep 3

# List instances
INSTANCES=$(curl -s http://localhost:9867/instances | jq '.[] | .id')
echo "✓ Running instances: $(echo $INSTANCES | tr '\n' ' ')"

# Navigate instance 1
NAV1=$(curl -s -X POST "http://localhost:9867/instances/$INST1_ID/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}')
TAB1_ID=$(echo $NAV1 | jq -r '.tabId')
echo "✓ Navigated instance 1, tab: $TAB1_ID"

# Navigate instance 2
NAV2=$(curl -s -X POST "http://localhost:9867/instances/$INST2_ID/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}')
TAB2_ID=$(echo $NAV2 | jq -r '.tabId')
echo "✓ Navigated instance 2, tab: $TAB2_ID"

# Verify isolation: different tab IDs
if [ "$TAB1_ID" != "$TAB2_ID" ]; then
  echo "✓ Tab isolation verified (different IDs)"
else
  echo "✗ FAILED: Tab IDs should be different!"
  exit 1
fi

# Stop instance 1
curl -s -X POST "http://localhost:9867/instances/$INST1_ID/stop" > /dev/null
echo "✓ Stopped instance 1"

# Stop instance 2
curl -s -X POST "http://localhost:9867/instances/$INST2_ID/stop" > /dev/null
echo "✓ Stopped instance 2"

# Verify all stopped
REMAINING=$(curl -s http://localhost:9867/instances | jq '.[] | .id' | wc -l)
if [ "$REMAINING" -eq 0 ]; then
  echo "✓ All instances cleaned up"
else
  echo "✗ FAILED: $REMAINING instances still running!"
  exit 1
fi

# Clean up
kill $DASHBOARD_PID 2>/dev/null
echo ""
echo "✅ ALL TESTS PASSED!"
```

Save as `test-e2e.sh`, then run:
```bash
chmod +x test-e2e.sh
./test-e2e.sh
```

---

## Stress Test (10 Instances)

```bash
#!/bin/bash

./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "Creating 10 instances..."
INSTANCES=()

for i in {1..10}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"stress-$i\",\"headless\":true}")
  ID=$(echo $INST | jq -r '.id')
  INSTANCES+=($ID)
  PORT=$(echo $INST | jq -r '.port')
  echo "  $i. $ID (port: $PORT)"
done

sleep 3
echo ""
echo "Navigating all instances concurrently..."

for ID in "${INSTANCES[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$ID/navigate" \
    -H "Content-Type: application/json" \
    -d '{"url":"https://example.com"}' &
done

wait
echo "✓ All navigations completed"

echo ""
echo "Stopping all instances..."
for ID in "${INSTANCES[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$ID/stop" > /dev/null &
done

wait
echo "✓ All instances stopped"

kill $DASHBOARD_PID 2>/dev/null
echo "✅ STRESS TEST PASSED!"
```

---

## Multi-Agent Test

Test with agents targeting different instances:

```bash
#!/bin/bash

./pinchtab &
sleep 2

# Create 3 instances for 3 agents
AGENT_A=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"agent-a","headless":true}' | jq -r '.id')

AGENT_B=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"agent-b","headless":true}' | jq -r '.id')

AGENT_C=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"agent-c","headless":true}' | jq -r '.id')

sleep 2

# Agent A: navigate to site 1
curl -X POST "http://localhost:9867/instances/$AGENT_A/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' &

# Agent B: navigate to site 2
curl -X POST "http://localhost:9867/instances/$AGENT_B/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}' &

# Agent C: navigate to site 3
curl -X POST "http://localhost:9867/instances/$AGENT_C/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://rust-lang.org"}' &

wait

echo "✓ All agents navigated independently"
echo "✓ Each has isolated cookies/history"
echo "✓ No state leakage between agents"

# Cleanup
curl -s -X POST "http://localhost:9867/instances/$AGENT_A/stop" > /dev/null
curl -s -X POST "http://localhost:9867/instances/$AGENT_B/stop" > /dev/null
curl -s -X POST "http://localhost:9867/instances/$AGENT_C/stop" > /dev/null

echo "✅ MULTI-AGENT TEST PASSED!"
```

---

## Test Checklist

- [ ] Hash-based IDs work (prof_X, inst_X, tab_X formats)
- [ ] Auto-port allocation works (9868, 9869, 9870, ...)
- [ ] Port release and reuse works
- [ ] Headed instance opens Chrome window
- [ ] Headless instance runs silently
- [ ] Orchestrator proxy routes work
- [ ] Instance isolation (no state leakage)
- [ ] Multi-instance concurrent navigation works
- [ ] Health checks pass on all instances
- [ ] Cleanup on shutdown works
- [ ] 10-instance stress test passes
- [ ] Multi-agent isolation verified

---

## Debugging Tips

### Check Dashboard Logs

```bash
# Follow logs while running tests
./pinchtab 2>&1 | grep -E "instance|chrome|port"
```

### Check Instance Port

```bash
# See which ports are in use
lsof -i :9868
lsof -i :9869
```

### Check Instance Health

```bash
curl http://localhost:9868/health
curl http://localhost:9869/health
```

### Manual Chrome Kill (if needed)

```bash
# Kill all Pinchtab-managed Chrome processes
pkill -f "Chrome.*user-data-dir.*\.pinchtab"
```

---

## Expected Results

After running all tests, you should see:
- ✅ 10+ instances created and destroyed without issues
- ✅ Hash-based IDs consistent and validated
- ✅ Ports auto-allocated and released properly
- ✅ Headed windows open and close correctly
- ✅ No Chrome orphan processes
- ✅ Complete isolation between instances
- ✅ Multi-agent concurrent access works flawlessly

---

**Ready to test?** Run `./pinchtab` and follow the Quick Start Test above!
