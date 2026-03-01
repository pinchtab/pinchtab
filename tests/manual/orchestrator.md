# Pinchtab Orchestrator & Multi-Instance Manual Testing

**Purpose:** Verify aspects of the multi-instance orchestrator that cannot be easily automated (visual UI, real-time monitoring, resource inspection).

**Note:** Automated tests for instance creation, isolation, port allocation, proxy routing, and ID formats are in `tests/integration/orchestrator_test.go`. This document covers _manual_ validation only.

---

## 1. Visual Verification: Headed Instances

### MH1: Headed Instance Shows Chrome Window

**Goal:** Verify that instances created with `headless=false` open a visible Chrome window.

**Steps:**
1. Start dashboard: `go build -o pinchtab ./cmd/pinchtab && ./pinchtab`
2. In another terminal, create a headed instance:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"test-headed","headless":false}'
   ```
   Note the returned `id` and `port`.

3. Within 5 seconds, a Chrome window should appear on screen.
4. Navigate the instance:
   ```bash
   TAB_ID=$(curl -s -X POST http://localhost:9867/instances/{id}/tabs/open \
     -H "Content-Type: application/json" \
     -d '{"action":"new","url":"about:blank"}' | jq -r '.tabId')
   curl -X POST http://localhost:9867/tabs/$TAB_ID/navigate \
     -H "Content-Type: application/json" \
     -d '{"url":"https://example.com"}'
   ```
   The Chrome window should load and display example.com.

5. Stop the instance:
   ```bash
   curl -X POST http://localhost:9867/instances/{id}/stop
   ```
   The Chrome window should close.

**Expected:** Chrome window visible, responsive to navigation, closes cleanly.

**Criteria:** ✓ Chrome appears within 5s | ✓ Window responsive | ✓ Window closes on stop

---

### MH2: Headless Instance Does NOT Show Chrome Window

**Goal:** Verify that headless instances run in background without visible window.

**Steps:**
1. Create a headless instance:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"test-headless","headless":true}'
   ```

2. Wait 5 seconds — NO Chrome window should appear.

3. Verify instance is working:
   ```bash
   curl http://localhost:{port}/health
   # Should return {"status":"ok"}
   ```

4. Verify navigation works:
   ```bash
   curl -X POST http://localhost:{port}/navigate \
     -H "Content-Type: application/json" \
     -d '{"url":"https://example.com"}'
   ```

5. Stop the instance.

**Expected:** No visible window, but instance fully functional.

**Criteria:** ✓ No window appears | ✓ Health check works | ✓ Navigation works

---

## 2. Real-Time Monitoring

### MM1: Monitor Instance Memory Growth

**Goal:** Verify instance memory stays reasonable under navigation load.

**Setup:**
- Start dashboard
- Create a headless instance and get its port

**Steps:**
1. On Linux/macOS, monitor memory of the instance process:
   ```bash
   # Get instance PID (starts pinchtab as subprocess on port 9868, etc.)
   PID=$(lsof -i :9868 | grep pinchtab | awk '{print $2}')

   # Monitor RSS (resident set size) every 2 seconds
   while true; do
     ps -p $PID -o pid,rss,vsz,comm
     sleep 2
   done
   ```

2. Run navigation loop in another terminal:
   ```bash
   for i in {1..20}; do
     curl -X POST http://localhost:9868/navigate \
       -H "Content-Type: application/json" \
       -d "{\"url\":\"https://example.com/$i\"}" > /dev/null
     sleep 0.5
   done
   ```

3. Observe memory output (RSS column, in KB).

**Expected Values:**
- Idle: ~150-200 MB (150000-200000 KB)
- After 20 navigations: ~200-250 MB
- No sustained growth after navigations stop

**Criteria:** ✓ Starts < 200 MB | ✓ Grows < 100 MB per 20 navigations | ✓ Stable after load

---

### MM2: Monitor CPU Usage During Navigation

**Goal:** Verify CPU usage is reasonable (spikes during nav, drops after).

**Setup:**
- Same as MM1

**Steps:**
1. Start continuous CPU monitoring:
   ```bash
   PID=$(lsof -i :9868 | grep pinchtab | awk '{print $2}')
   top -p $PID -b -n 100 | grep "pinchtab"
   ```

2. Run navigation load in another terminal:
   ```bash
   for i in {1..10}; do
     curl -X POST http://localhost:9868/navigate \
       -H "Content-Type: application/json" \
       -d "{\"url\":\"https://example.com/$i\"}" > /dev/null
   done
   ```

3. Observe CPU % in `top` output.

**Expected Behavior:**
- CPU idle: 0-1%
- During navigation: 20-50% (single core usage on multi-core system)
- After navigation: back to 0-1% within 2-3 seconds

**Criteria:** ✓ Idle CPU low | ✓ Navigation uses moderate CPU | ✓ Returns to idle quickly

---

## 3. Port Management Verification

### MP1: Verify Port Allocation Range

**Goal:** Verify instances are allocated ports in the configured range.

**Setup:**
- Default range: 9868-9968 (100 instances max)
- Check config: `INSTANCE_PORT_START` / `INSTANCE_PORT_END` env vars

**Steps:**
1. Create 5 instances:
   ```bash
   for i in {1..5}; do
     curl -X POST http://localhost:9867/instances/launch \
       -H "Content-Type: application/json" \
       -d "{\"name\":\"test-$i\",\"headless\":true}" | jq '.port'
   done
   ```

2. Verify all returned ports are in range 9868-9968.

3. Check which ports are actually listening:
   ```bash
   lsof -i :9868-9968 | grep LISTEN
   ```

**Criteria:** ✓ All ports in range | ✓ All advertised ports actually listening | ✓ No duplicates

---

### MP2: Verify Port Cleanup and Reuse

**Goal:** Verify ports are released when instances stop, enabling reuse.

**Steps:**
1. Create instance, note port:
   ```bash
   INST1=$(curl -s -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"reuse-1","headless":true}' | jq -r '.id,.port')
   # Extract ID and port
   PORT1=$(echo $INST1 | awk '{print $2}')
   ```

2. Verify port is listening:
   ```bash
   lsof -i :$PORT1 | grep LISTEN
   ```

3. Stop instance:
   ```bash
   curl -X POST http://localhost:9867/instances/{id}/stop
   ```

4. Wait 2 seconds and verify port is no longer listening:
   ```bash
   lsof -i :$PORT1 | grep -c LISTEN || echo "Port freed"
   ```

5. Create new instance:
   ```bash
   INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"reuse-2","headless":true}' | jq -r '.port')
   ```

6. Verify new instance reused the port:
   ```bash
   [ "$PORT1" = "$INST2" ] && echo "Port reused!" || echo "Different port"
   ```

**Criteria:** ✓ Port freed within 2s | ✓ New instance reuses freed port | ✓ No hanging processes

---

## 4. Chrome Initialization Verification

### MC1: Verify Lazy Chrome Initialization

**Goal:** Verify Chrome starts on first request, not at instance creation.

**Steps:**
1. Create instance and time how long until Chrome is ready:
   ```bash
   START=$(date +%s%N)

   INST=$(curl -s -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"lazy-test","headless":true}')

   ID=$(echo $INST | jq -r '.id')

   # Poll health until OK
   while true; do
     STATUS=$(curl -s http://localhost:9867/instances/$ID/health 2>/dev/null | jq -r '.status')
     if [ "$STATUS" = "ok" ]; then break; fi
     sleep 0.1
   done

   END=$(date +%s%N)
   ELAPSED=$(( (END - START) / 1000000 ))
   echo "Chrome ready in ${ELAPSED}ms"
   ```

**Expected:** Chrome should be ready within 5-10 seconds (longer for headed instances due to window rendering).

**Criteria:** ✓ Chrome ready in reasonable time | ✓ No hanging processes

---

### MC2: Verify Headed Instance Window Opens Quickly

**Goal:** Verify headed Chrome windows appear without delay.

**Steps:**
1. Create a headed instance and start stopwatch:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"window-test","headless":false}'
   ```

2. Start timing when curl returns (should be < 1s).

3. Watch for Chrome window to appear (should be < 5s from curl return).

4. Window should display immediately upon appearance (not blank/loading).

**Criteria:** ✓ Window appears < 5s | ✓ Window displays properly | ✓ Responsive to further navigation

---

## 5. Dashboard UI Verification

### MU1: Instance List in Dashboard UI

**Goal:** Verify dashboard shows created instances with status.

**Steps:**
1. Create 3 instances via API:
   ```bash
   for i in {1..3}; do
     curl -X POST http://localhost:9867/instances/launch \
       -H "Content-Type: application/json" \
       -d "{\"name\":\"ui-test-$i\",\"headless\":true}" > /dev/null
   done
   ```

2. Open dashboard: http://localhost:9867/dashboard

3. Click on "Instances" tab.

4. Verify:
   - All 3 instances listed
   - Each shows: ID, port, profile name, status (running/initializing/stopped)
   - No 404 errors in browser console

**Criteria:** ✓ All instances visible | ✓ Status accurate | ✓ No JS errors

---

### MU2: Instance Creation from Dashboard

**Goal:** Verify dashboard UI can create new instances.

**Steps:**
1. Open dashboard: http://localhost:9867/dashboard

2. Navigate to "Instances" tab.

3. Click "Create Instance" or similar button.

4. Enter profile name (e.g., "dashboard-test").

5. Select headless/headed.

6. Submit.

7. Verify instance appears in list and reaches "running" status.

8. Verify port is allocated and instance responds to `/health`.

**Criteria:** ✓ Instance created | ✓ Appears in list | ✓ Responds to requests | ✓ Port valid

---

### MU3: Instance Deletion from Dashboard

**Goal:** Verify dashboard UI can stop instances.

**Steps:**
1. Open dashboard with running instances.

2. Click "Stop" or "Terminate" on an instance.

3. Verify:
   - Instance disappears from list (or shows "stopped" status)
   - Port becomes available (check with `lsof`)
   - Chrome process exits (check with `ps`)

**Criteria:** ✓ Instance stops | ✓ Port freed | ✓ Process exits | ✓ No orphaned processes

---

## 6. Error Conditions & Edge Cases

### ME1: Create Instance with Invalid Profile Name

**Goal:** Verify error handling for bad profile names.

**Steps:**
1. Try to create instance with empty name:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"","headless":true}'
   ```

2. Verify error response (400 or 422).

3. Try with special characters or very long name.

**Criteria:** ✓ Error response (not 500 crash) | ✓ Clear error message

---

### ME2: Exceed Port Range

**Goal:** Verify behavior when port range exhausted.

**Setup:**
- Set `INSTANCE_PORT_START=19000 INSTANCE_PORT_END=19005` (6 ports only)
- Start dashboard with these limits

**Steps:**
1. Create 7 instances.

2. Verify:
   - First 6 succeed with ports 19000-19005
   - 7th fails with clear error (not a crash or hang)

3. Stop instance 1, which frees port 19000.

4. Create instance 8 — verify it reuses port 19000.

**Criteria:** ✓ First 6 succeed | ✓ 7th fails gracefully | ✓ Reuse works | ✓ No 500 errors

---

### ME3: Concurrent Instance Creation

**Goal:** Verify thread-safety of port allocation under concurrent requests.

**Steps:**
1. Send 10 concurrent instance creation requests:
   ```bash
   for i in {1..10}; do
     (curl -X POST http://localhost:9867/instances/launch \
       -H "Content-Type: application/json" \
       -d "{\"name\":\"concurrent-$i\",\"headless\":true}" | jq '.port') &
   done
   wait
   ```

2. Collect all returned ports.

3. Verify:
   - All 10 creation requests succeed
   - All ports unique (no duplicates)
   - All ports in valid range

**Criteria:** ✓ All succeed | ✓ No duplicate ports | ✓ All unique

---

## 7. Integration with Existing Features

### MI1: Proxy Routing Works with Multiple Instances

**Goal:** Verify proxy routes forward to correct instance.

**Steps:**
1. Create 3 instances:
   ```bash
   IDS=()
   for i in {1..3}; do
     ID=$(curl -s -X POST http://localhost:9867/instances/launch \
       -H "Content-Type: application/json" \
       -d "{\"name\":\"proxy-$i\",\"headless\":true}" | jq -r '.id')
     IDS+=($ID)
   done
   ```

2. Navigate on each via orchestrator proxy:
   ```bash
   for i in 0 1 2; do
     TAB_ID=$(curl -s -X POST http://localhost:9867/instances/${IDS[$i]}/tabs/open \
       -H "Content-Type: application/json" \
       -d '{"action":"new","url":"about:blank"}' | jq -r '.tabId')
     curl -X POST http://localhost:9867/tabs/$TAB_ID/navigate \
       -H "Content-Type: application/json" \
       -d '{"url":"https://example.com"}' | jq '.tabId'
   done
   ```

3. Verify:
   - All navigation requests succeed
   - Each returns a valid tab ID
   - No cross-instance interference

**Criteria:** ✓ All navigate succeed | ✓ Correct tab IDs | ✓ No 404 errors

---

### MI2: Snapshot & Screenshot Work on Instance Proxy

**Goal:** Verify proxy routes work for snapshot/screenshot.

**Steps:**
1. Create instance and navigate:
   ```bash
   ID=$(curl -s -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"snap-test","headless":true}' | jq -r '.id')

   TAB_ID=$(curl -s -X POST http://localhost:9867/instances/$ID/tabs/open \
     -H "Content-Type: application/json" \
     -d '{"action":"new","url":"about:blank"}' | jq -r '.tabId')
   curl -X POST http://localhost:9867/tabs/$TAB_ID/navigate \
     -H "Content-Type: application/json" \
     -d '{"url":"https://example.com"}' > /dev/null
   ```

2. Request snapshot via orchestrator:
   ```bash
   curl http://localhost:9867/tabs/$TAB_ID/snapshot | jq '.nodes | length'
   ```

3. Request screenshot via orchestrator:
   ```bash
   curl http://localhost:9867/tabs/$TAB_ID/screenshot > /tmp/test.jpg
   file /tmp/test.jpg
   ```

4. Verify both succeed and return expected formats.

**Criteria:** ✓ Snapshot JSON valid | ✓ Screenshot is JPEG | ✓ Both proxied correctly

---

## Checklist for Manual Testing

- [ ] **MH1:** Headed instance shows Chrome window
- [ ] **MH2:** Headless instance doesn't show window
- [ ] **MM1:** Memory stays reasonable (< 250 MB idle)
- [ ] **MM2:** CPU usage spikes during nav, returns to idle
- [ ] **MP1:** Ports allocated in correct range
- [ ] **MP2:** Ports released and reused
- [ ] **MC1:** Chrome initializes lazily within 10s
- [ ] **MC2:** Headed window appears < 5s
- [ ] **MU1:** Dashboard displays instances correctly
- [ ] **MU2:** Dashboard can create instances
- [ ] **MU3:** Dashboard can stop instances
- [ ] **ME1:** Bad profile names handled gracefully
- [ ] **ME2:** Port exhaustion handled gracefully
- [ ] **ME3:** Concurrent creation thread-safe
- [ ] **MI1:** Proxy routing works for navigation
- [ ] **MI2:** Proxy routing works for snapshot/screenshot

---

## 10. Security: Path Traversal & SSRF Prevention

### MSE1: Profile Name Path Traversal Blocked

**Goal:** Verify that profile names with "..", "/", or "\" are rejected.

**Steps:**
1. Start dashboard
2. Try to create a profile with traversal pattern:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"../../../etc/passwd","headless":true}'
   ```
   Should get **400 Bad Request** (invalid profile name)

3. Try with "/" separator:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"test/malicious","headless":true}'
   ```
   Should get **400 Bad Request**

4. Try with empty name:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"","headless":true}'
   ```
   Should get **400 Bad Request**

5. Create valid profile (control):
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"valid-profile","headless":true}'
   ```
   Should get **201 Created**

**Expected:** Malicious names rejected with 400, valid names accepted with 201.

**Criteria:** ✓ ".." rejected | ✓ "/" rejected | ✓ "\" rejected | ✓ "" rejected | ✓ valid names accepted

---

### MSE2: SSRF Prevention — Proxy Localhost Only

**Goal:** Verify that proxy endpoints only target localhost.

**Steps:**
1. Create an instance:
   ```bash
   curl -X POST http://localhost:9867/instances/launch \
     -H "Content-Type: application/json" \
     -d '{"name":"ssrf-test","headless":true}'
   ```
   Note the `id` and `port`.

2. Navigate via proxy (should work):
   ```bash
   TAB_ID=$(curl -s -X POST http://localhost:9867/instances/{id}/tabs/open \
     -H "Content-Type: application/json" \
     -d '{"action":"new","url":"about:blank"}' | jq -r '.tabId')
   curl -X POST http://localhost:9867/tabs/$TAB_ID/navigate \
     -H "Content-Type: application/json" \
     -d '{"url":"https://example.com"}'
   ```
   Should get **200 OK** (proxy routes to localhost instance)

3. Stop instance:
   ```bash
   curl -X POST http://localhost:9867/instances/{id}/stop
   ```

**Expected:** Proxy routes to localhost instance without error.

**Criteria:** ✓ Proxy navigates successfully | ✓ URL constructed safely

---

## Notes

- Tests should run on the same machine as the dashboard (needed for window verification and process monitoring).
- Some tests (MM1, MM2, MP1) require Linux/macOS tools (`lsof`, `ps`, `top`).
- All tests should clean up instances via API after completing (stop them).
- If running in CI/headless environment, skip visual tests (MH1, MH2, MU1, MU2) and window tests (MC2).
