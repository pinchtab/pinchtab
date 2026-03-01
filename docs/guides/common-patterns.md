# Common Patterns

Real-world patterns for using Pinchtab's multi-instance architecture effectively.

---

## Pattern 1: Headed vs Headless

### Headed (Visible Window)

Use when you need to **see what's happening**:

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}'
```

**Use cases:**
- Interactive debugging and testing
- Visual QA/regression testing
- Manual workflow verification
- Step-by-step debugging
- Learning/exploration

**Characteristics:**
- Chrome window opens and is visible
- Slightly slower (rendering overhead)
- More memory (~100-150 MB)
- Can interact manually if needed
- Good for dev/test environments

**Example: Interactive Testing Session**

```bash
#!/bin/bash

# Create a headed instance for manual testing
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}' | jq -r '.id')

echo "Manual testing instance: $INST"
echo "Chrome window is open - interact manually or use API calls"

# Wait for Chrome to initialize
sleep 2

# Optionally navigate to starting URL (via orchestrator proxy)
curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com"}'

echo "Navigate the app manually, then run automated tests..."
sleep 300

# Keep instance running for 5 minutes
curl -X POST "http://localhost:9867/instances/$INST/stop"
```

### Headless (Background)

Use when you need **speed and efficiency**:

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
```

**Use cases:**
- Automated testing (CI/CD)
- Data scraping
- High-throughput operations
- Batch processing
- Production workloads

**Characteristics:**
- No visible window (runs in background)
- Faster execution (~20% speedup)
- Less memory (~50-80 MB)
- No manual interaction
- Perfect for automation

**Example: Headless Testing Pipeline**

```bash
#!/bin/bash

# Create 5 headless instances for parallel testing
INSTANCES=()

for i in {1..5}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq -r '.id')
  
  INSTANCES+=("$INST")
done

echo "Created 5 parallel test workers"

# Run tests in parallel
TEST_URLS=(
  "https://app.example.com/test1"
  "https://app.example.com/test2"
  "https://app.example.com/test3"
  "https://app.example.com/test4"
  "https://app.example.com/test5"
)

for i in "${!INSTANCES[@]}"; do
  INST="${INSTANCES[$i]}"
  URL="${TEST_URLS[$i]}"
  
  # Run test in parallel (via orchestrator proxy)
  (
    curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"$URL\"}" > /dev/null
    
    # Verify page loaded
    TITLE=$(curl -s "http://localhost:9867/tabs/$TAB_ID/snapshot" | jq -r '.title')
    echo "Test $i: $TITLE"
  ) &
done

wait

# Cleanup
for INST in "${INSTANCES[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$INST/stop" > /dev/null
done

echo "Tests complete"
```

### Mixed Fleet

Typical production setup:

```bash
# 1 headed for interactive debugging
HEADED=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}' | jq -r '.id')

# 10 headless for bulk operations
for i in {1..10}; do
  curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' > /dev/null &
done
wait

echo "Setup: 1 headed (debug) + 10 headless (workers)"
```

---

## Pattern 2: Load Distribution

### Round-Robin Distribution

Distribute requests evenly across instances via orchestrator:

```bash
#!/bin/bash

# Create 5 instances
INSTANCES=()
for i in {1..5}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq -r '.id')
  INSTANCES+=("$INST")
done

# Wait for Chrome initialization
sleep 3

URLS=(
  "https://example.com/page1"
  "https://example.com/page2"
  "https://example.com/page3"
  "https://example.com/page4"
  "https://example.com/page5"
  "https://example.com/page6"
  "https://example.com/page7"
  "https://example.com/page8"
)

# Distribute evenly via orchestrator proxy
for i in "${!URLS[@]}"; do
  URL="${URLS[$i]}"
  INST="${INSTANCES[$((i % ${#INSTANCES[@]}))]}"
  
  curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}" &
done

wait
```

**Result:**
Each instance handles roughly equal work (round-robin).

### Weighted Distribution

Send more work to better instances (e.g., faster systems get more requests):

```bash
#!/bin/bash

# Create weighted instance pool
INSTANCES=()

# Add 3 copies of fast instance (handles 3/6 = 50% of work)
for i in {1..3}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq -r '.id')
  INSTANCES+=("$INST")
done

# Add 2 copies of standard instance (handles 2/6 = 33% of work)
for i in {1..2}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq -r '.id')
  INSTANCES+=("$INST")
done

# Add 1 copy of slow instance (handles 1/6 = 17% of work)
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')
INSTANCES+=("$INST")

# Distribute using weighted list
for i in "${!URLS[@]}"; do
  URL="${URLS[$i]}"
  INST="${INSTANCES[$((i % ${#INSTANCES[@]}))]}"
  
  curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}" &
done

wait
```

**Result:**
Fast instances handle more load, slower instances handle less.

### Queue-Based Distribution

Process a work queue dynamically, keeping a fixed number of concurrent jobs:

```bash
#!/bin/bash

# Create instance pool (3 workers)
INSTANCES=()
for i in {1..3}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq -r '.id')
  INSTANCES+=("$INST")
done

# Work queue (100 URLs)
QUEUE=(
  "https://example.com/1"
  "https://example.com/2"
  "https://example.com/3"
  # ... up to ...
  "https://example.com/100"
)

# Process queue with max concurrent jobs (use 'wait -n' for job-level control)
MAX_JOBS=3
CURRENT_JOB=0

# Start initial batch
for i in $(seq 0 $((MAX_JOBS - 1))); do
  [ $CURRENT_JOB -lt ${#QUEUE[@]} ] || break
  
  URL="${QUEUE[$CURRENT_JOB]}"
  INST="${INSTANCES[$((CURRENT_JOB % MAX_JOBS))]}"
  
  (
    curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"$URL\"}" > /dev/null
    echo "Processed: $URL"
  ) &
  
  ((CURRENT_JOB++))
done

# Process remaining queue
while [ $CURRENT_JOB -lt ${#QUEUE[@]} ]; do
  URL="${QUEUE[$CURRENT_JOB]}"
  INST="${INSTANCES[$((CURRENT_JOB % MAX_JOBS))]}"
  
  (
    curl -s -X POST "http://localhost:9867/instances/$INST/tabs/open" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"$URL\"}" > /dev/null
    echo "Processed: $URL"
  ) &
  
  # Wait for a background job to complete
  wait -n
  
  ((CURRENT_JOB++))
done

# Wait for remaining jobs
wait

echo "Queue processing complete"
```

---

## Pattern 3: State Isolation

### Per-User Profiles

Each user gets their own instance with persistent state:

```bash
#!/bin/bash

# Create persistent profiles for each user
pinchtab profile create user-1
pinchtab profile create user-2
pinchtab profile create user-3

# Get profile IDs
U1_PROF=$(pinchtab profiles | jq -r '.[] | select(.name=="user-1") | .id')
U2_PROF=$(pinchtab profiles | jq -r '.[] | select(.name=="user-2") | .id')
U3_PROF=$(pinchtab profiles | jq -r '.[] | select(.name=="user-3") | .id')

# Create instances with profiles
U1=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$U1_PROF'","mode":"headless"}' | jq -r '.id')

U2=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$U2_PROF'","mode":"headless"}' | jq -r '.id')

U3=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$U3_PROF'","mode":"headless"}' | jq -r '.id')

# Each user has:
# - Separate Chrome process
# - Persistent profile (survives restarts)
# - Separate cookies (login sessions preserved)
# - Separate history, local storage
# - No interference between users

# User 1 navigates (via orchestrator proxy)
curl -s -X POST "http://localhost:9867/instances/$U1/tabs/open" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com/login?user=1"}'

# User 2 navigates independently
PORT_U2=$(curl -s http://localhost:9867/instances/$U2 | jq -r '.port')
curl -s -X POST "http://localhost:$PORT_U2/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com/login?user=2"}'

# User 3 navigates independently
PORT_U3=$(curl -s http://localhost:9867/instances/$U3 | jq -r '.port')
curl -s -X POST "http://localhost:$PORT_U3/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com/login?user=3"}'
```

### Session Persistence

Resume browser state from previous session:

```bash
#!/bin/bash

# Session 1: Create instance, navigate, set up state
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"persistent-session","headless":false}' | jq -r '.id')

PORT=$(curl -s http://localhost:9867/instances/$INST | jq -r '.port')

# Navigate and set up state
curl -s -X POST "http://localhost:$PORT/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://app.example.com"}'

# User interacts manually...
sleep 3600  # 1 hour session

# Stop instance (state saved to disk)
curl -X POST "http://localhost:9867/instances/$INST/stop"

# ... later ...

# Session 2: Create new instance with same profile
# Restores cookies, cache, history from disk
INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"persistent-session","headless":false}' | jq -r '.id')

# Instance 2 has all the state from Instance 1
# Same cookies, same history, same preferences
```

---

## Pattern 4: Batch Operations

### Sequential Processing (Simple)

Process items one at a time:

```bash
ITEMS=("item1" "item2" "item3" ... "item1000")

for ITEM in "${ITEMS[@]}"; do
  curl -s -X POST "http://localhost:9868/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?item=$ITEM\"}"
  
  sleep 1  # Rate limiting
done
```

**Pros:** Simple, easy to debug
**Cons:** Slow (1 item/sec = 1000 seconds for 1000 items)

### Parallel Processing (Faster)

Use multiple instances:

```bash
INSTANCES=()

# Create 10 instances
for i in {1..10}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"batch-worker-$i\",\"headless\":true}" | jq -r '.id')
  
  PORT=$(curl -s http://localhost:9867/instances/$INST | jq -r '.port')
  INSTANCES+=("$INST:$PORT")
done

# Process items in parallel
ITEM_INDEX=0

for ITEM in "${ITEMS[@]}"; do
  IFS=: read INST PORT <<< "${INSTANCES[$((ITEM_INDEX % 10))]}"
  
  curl -s -X POST "http://localhost:$PORT/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?item=$ITEM\"}" &
  
  ((ITEM_INDEX++))
done

wait
```

**Performance:** 1000 items with 10 instances ÷ 10 = 100 seconds (10x faster!)

### Adaptive Batch Processing

Process items, track results, retry failures:

```bash
#!/bin/bash

BATCH_SIZE=10
FAILED=()
RESULTS=()

for i in "${!ITEMS[@]}"; do
  ITEM="${ITEMS[$i]}"
  
  # Skip to next batch every BATCH_SIZE items
  if [ $((($i + 1) % BATCH_SIZE)) -eq 0 ]; then
    wait  # Wait for batch to complete
  fi
  
  # Process item
  if curl -s -X POST "http://localhost:9868/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?item=$ITEM\"}" > /dev/null 2>&1; then
    RESULTS+=("$ITEM: OK")
  else
    FAILED+=("$ITEM")
  fi &
done

wait

# Retry failures
echo "Retrying ${#FAILED[@]} failed items..."

for ITEM in "${FAILED[@]}"; do
  curl -s -X POST "http://localhost:9868/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?item=$ITEM\"}"
done

echo "Results: ${#RESULTS[@]} OK, ${#FAILED[@]} failed"
```

---

## Pattern 5: Error Handling

### Health-Check Before Operations

```bash
function safe_navigate() {
  local PORT=$1
  local URL=$2
  
  # Check health
  if ! curl -s "http://localhost:$PORT/health" | jq -e '.status == "ok"' > /dev/null; then
    echo "ERROR: Instance on port $PORT not healthy"
    return 1
  fi
  
  # Navigate
  curl -s -X POST "http://localhost:$PORT/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}"
}

safe_navigate 9868 "https://example.com" || echo "Navigation failed"
```

### Timeout Handling

```bash
function navigate_with_timeout() {
  local PORT=$1
  local URL=$2
  local TIMEOUT=$3
  
  timeout $TIMEOUT curl -s -X POST "http://localhost:$PORT/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}"
  
  if [ $? -eq 124 ]; then
    echo "ERROR: Navigation timed out after ${TIMEOUT}s"
    return 1
  fi
}

navigate_with_timeout 9868 "https://slow-site.com" 30
```

### Graceful Degradation

```bash
function navigate_with_fallback() {
  local URL=$1
  
  # Try instance 1
  if curl -s "http://localhost:9868/health" | jq -e '.status == "ok"' > /dev/null; then
    curl -s -X POST "http://localhost:9868/navigate" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"$URL\"}"
    return 0
  fi
  
  # Fall back to instance 2
  if curl -s "http://localhost:9869/health" | jq -e '.status == "ok"' > /dev/null; then
    echo "Instance 1 down, using fallback (9869)"
    curl -s -X POST "http://localhost:9869/navigate" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"$URL\"}"
    return 0
  fi
  
  # All instances down
  echo "ERROR: All instances down"
  return 1
}
```

---

## Pattern 6: Monitoring & Cleanup

### Active Instance Monitoring

```bash
#!/bin/bash

while true; do
  echo "=== $(date) ==="
  
  # Get all instances
  INSTANCES=$(curl -s http://localhost:9867/instances)
  
  # Count by status
  RUNNING=$(echo "$INSTANCES" | jq '[.[] | select(.status=="running")] | length')
  STOPPED=$(echo "$INSTANCES" | jq '[.[] | select(.status=="stopped")] | length')
  TOTAL=$(echo "$INSTANCES" | jq 'length')
  
  echo "Total: $TOTAL | Running: $RUNNING | Stopped: $STOPPED"
  
  # List each instance
  echo "$INSTANCES" | jq '.[] | "\(.id) (\(.profileName)) - port \(.port) - \(.status)"'
  
  sleep 30
done
```

### Cleanup Stopped Instances

```bash
#!/bin/bash

INSTANCES=$(curl -s http://localhost:9867/instances)

# Stop all instances
echo "$INSTANCES" | jq -r '.[] | .id' | while read INST_ID; do
  echo "Stopping $INST_ID..."
  curl -s -X POST "http://localhost:9867/instances/$INST_ID/stop" > /dev/null
done

echo "Cleanup complete. All ports released."
```

### Resource Monitoring

```bash
#!/bin/bash

while true; do
  echo "=== Instance Resource Usage ==="
  
  # Get active instances
  INSTANCES=$(curl -s http://localhost:9867/instances | jq -r '.[] | select(.status=="running") | .id')
  
  # Rough estimate: each headless ~60MB, headed ~120MB
  HEADLESS=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.headless==true)] | length')
  HEADED=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.headless==false)] | length')
  
  EST_MEMORY=$(( (HEADLESS * 60) + (HEADED * 120) ))
  
  echo "Headless instances: $HEADLESS (~${HEADLESS}*60MB = $((HEADLESS*60))MB)"
  echo "Headed instances: $HEADED (~${HEADED}*120MB = $((HEADED*120))MB)"
  echo "Estimated memory: ~${EST_MEMORY}MB"
  
  # Alert if over limit
  if [ $EST_MEMORY -gt 8000 ]; then
    echo "⚠️ WARNING: High memory usage!"
  fi
  
  sleep 60
done
```

---

## Summary

| Pattern | Use Case | Throughput | Memory |
|---------|----------|-----------|--------|
| **Headed** | Interactive debugging | Low | High |
| **Headless** | Automation, scraping | Medium | Low |
| **Round-robin** | Balanced load | Medium-High | Medium |
| **Weighted** | Heterogeneous instances | High | Medium-High |
| **Queue-based** | Dynamic work distribution | High | Low-Medium |
| **Per-user profiles** | Multi-tenant | Variable | Linear with users |
| **Session persistence** | Long-running workflows | Low | Depends on state |

Choose the pattern that matches your workload!
