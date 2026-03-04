# Test Failures Report

**Generated:** 2026-03-04 12:00-12:01 CET  
**Branch:** feat/allocation-strategies  
**Commit:** ff9d7a7 (refactor: organize manual tests)  
**Build Status:** ✅ Successful

---

## Summary

❌ **3/3 manual test scripts failed**

| Test | Duration | Status | Issue |
|------|----------|--------|-------|
| Automated Test | ~4 sec | ❌ FAIL | 404 on navigate endpoint |
| Stress Test (10 instances) | ~12 sec | ❌ FAIL | 404 on navigate + incomplete cleanup |
| Multi-Agent Isolation | ~20 sec | ❌ FAIL | 404 on navigate + incomplete cleanup |

---

## Detailed Failures

### Test 1: Automated Test Script

**Expected:** Navigate 2 instances via orchestrator proxy, verify isolation, cleanup  
**Actual:** Failed on first navigate request

**Failure Point:**
```
Command: curl -X POST http://localhost:9867/instances/inst_fb8b8643/navigate
Response: 404 Not Found
Error: jq: error (at <stdin>:1): Cannot index number with string "tabId"
```

**Root Cause:**
The orchestrator proxy endpoint `POST /instances/{id}/navigate` returns **404**.

**Expected Behavior:**
- Should accept navigate requests at `/instances/{id}/navigate`
- Should proxy to the instance's `/navigate` endpoint
- Should return 200 with tab details

**Log Entry:**
```
2026/03/04 12:00:32 INFO request requestId="" method=POST path=/instances/inst_fb8b8643/navigate status=404 ms=0
```

---

### Test 2: Stress Test (10 Concurrent Instances)

**Expected:** Create 10 instances, navigate all concurrently, verify isolation, cleanup completely  
**Actual:** Failed on navigate, incomplete cleanup

**Failures:**
1. **Navigate Endpoints All Return 404**
   ```
   2026/03/04 12:00:43 INFO request requestId="" method=POST path=/instances/inst_3ef63d80/navigate status=404 ms=0
   2026/03/04 12:00:43 INFO request requestId="" method=POST path=/instances/inst_9c2e2366/navigate status=404 ms=0
   ... (8 more 404s)
   ```

2. **Snapshot Endpoints All Return 404**
   ```
   2026/03/04 12:00:43 INFO request requestId="" method=GET path=/instances/inst_9c2e2366/snapshot status=404 ms=0
   ... (all 10 return 404)
   ```
   **Result:** `0/10 snapshots successful` (expected data)

3. **Incomplete Cleanup**
   ```
   After stopping all 10 instances:
   FAILED: 2 instances still running!
   ```
   **Expected:** 0 remaining instances  
   **Actual:** 2 instances left running

**Root Causes:**
1. Missing `/instances/{id}/navigate` endpoint
2. Missing `/instances/{id}/snapshot` endpoint  
3. Instance cleanup doesn't completely remove all instances

---

### Test 3: Multi-Agent Isolation Test

**Expected:** 3 agents in separate instances navigate concurrently with isolation verification  
**Actual:** Failed on navigate, incomplete cleanup

**Failures:**
1. **Navigate Endpoints All Return 404**
   ```
   2026/03/04 12:00:59 INFO request requestId="" method=POST path=/instances/inst_8e24205c/navigate status=404 ms=0
   2026/03/04 12:00:59 INFO request requestId="" method=POST path=/instances/inst_bd59c973/navigate status=404 ms=0
   2026/03/04 12:00:59 INFO request requestId="" method=POST path=/instances/inst_0da10324/navigate status=404 ms=0
   ```

2. **Snapshot Endpoints All Return 404**
   ```
   2026/03/04 12:00:59 INFO request requestId="" method=GET path=/instances/inst_8e24205c/snapshot status=404 ms=0
   ... (all 3 return 404)
   ```
   **Result:** Agents all see "unknown" instead of different content

3. **Incomplete Cleanup**
   ```
   After stopping all 3 agents:
   FAILED: 2 instances still running!
   ```
   **Expected:** 0 remaining instances  
   **Actual:** 2 instances left running

**Root Causes:**
1. Missing `/instances/{id}/navigate` endpoint
2. Missing `/instances/{id}/snapshot` endpoint  
3. Instance cleanup doesn't completely remove all instances

---

## Missing Endpoints

Based on test failures, the following endpoints are missing or broken:

### POST /instances/{id}/navigate
**Purpose:** Navigate an instance via orchestrator proxy  
**Current Status:** ❌ Returns 404  
**Used by:** All 3 tests  
**Expected Response:**
```json
{
  "tabId": "tab_XXXXXXXX",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

### GET /instances/{id}/snapshot
**Purpose:** Get page snapshot from an instance via orchestrator proxy  
**Current Status:** ❌ Returns 404  
**Used by:** Stress test, Multi-agent test  
**Expected Response:**
```json
{
  "url": "https://example.com",
  "title": "Example Domain",
  "accessibility_tree": {...}
}
```

### POST /instances/{id}/stop
**Purpose:** Stop a running instance  
**Current Status:** ✅ Works (200 OK)  
**Note:** Works but instances not completely cleaned up

---

## Instance Cleanup Issue

**Problem:** After stopping all instances, 2-5 instances remain in the running state.

**Evidence:**
- Stress Test: Created 10, stopped 10, found 2 remaining
- Multi-Agent Test: Created 3, stopped 3, found 2 remaining

**Possible Causes:**
1. Instance.Stop() doesn't fully clean up
2. Instances in "stopping" state not counted as stopped
3. Some instances don't get stop command
4. Race condition in cleanup

**Log Evidence:**
```
✓ All instances stopped
Verifying cleanup...
2026/03/04 12:00:49 INFO request requestId="" method=GET path=/instances status=200 ms=0
❌ FAILED: 2 instances still running!
```

---

## What Was Working

✅ **Instance Creation**
- Headed instances open Chrome windows
- Headless instances run silently
- Hash-based IDs generated correctly (inst_XXXXXXXX)
- Port allocation works (9868, 9869, etc.)
- Port allocator reuses released ports

✅ **Instance Listing**
- `GET /instances` returns all instances

✅ **Instance Stopping**
- `POST /instances/{id}/stop` returns 200
- Instances transition to "stopped"

---

## Test Execution Summary

| Test | Steps | Pass | Fail | Success Rate |
|------|-------|------|------|--------------|
| Automated | 5 | 2 | 3 | 40% |
| Stress | 6 | 3 | 3 | 50% |
| Multi-Agent | 6 | 3 | 3 | 50% |

---

## Next Steps (For Investigator)

### High Priority
1. ✗ Implement `POST /instances/{id}/navigate` endpoint
2. ✗ Implement `GET /instances/{id}/snapshot` endpoint
3. ✗ Fix instance cleanup (2-5 instances left running)

### Medium Priority
1. Verify instance lifecycle in /instances endpoint
2. Check orchestrator proxy implementation
3. Add logging to instance cleanup process

### Lower Priority
1. Add error handling for navigate/snapshot
2. Add response validation
3. Add timeout handling

---

## Files

- Full log: `test-results-20260304-120027.log`
- Test scripts: `tests/manual/*.sh`
- Test guide: `TESTING.md`

---

## Not Code-Changed

This report documents failures **without modifying code** because:
- Unknown whether 404 is expected (API mismatch vs missing implementation)
- Unclear if instances are "cleaning up asynchronously"
- Need domain expert to confirm expected behavior

Please investigate and fix root causes.
