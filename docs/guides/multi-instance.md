# Multi-Instance Workflows

Pinchtab's multi-instance architecture enables running multiple isolated Chrome browsers simultaneously, each with their own cookies, history, profiles, and configurations.

## Getting Started

### 1. Start the Dashboard

```bash
go build -o pinchtab ./cmd/pinchtab
./pinchtab
```

The dashboard runs on **port 9867** and manages all instances.

### 2. Create Your First Instance

Create a headed instance (visible window) for interactive work:

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","headless":false}'
```

**Response:**
```json
{
  "id": "inst_a365262a",
  "profileId": "prof_7f3d1e8b",
  "profileName": "work",
  "port": "9868",
  "headless": false,
  "status": "starting",
  "startTime": "2026-02-28T21:00:00Z"
}
```

The instance gets:
- **Auto-allocated port** (9868, 9869, ...)
- **Hash-based ID** (inst_XXXXX) - stable and portable
- **Profile isolation** - separate cookies, history, cache
- **Chrome process** - running independently

### 3. Navigate and Interact

Navigate the instance to a URL:

```bash
curl -X POST http://localhost:9868/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

**Response:**
```json
{
  "tabId": "tab_19949f62",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

Each navigation automatically creates a new tab. Use the `tabId` for subsequent operations on that specific tab.

### 4. Get Page Content

Retrieve page snapshot (DOM, text, structured data):

```bash
curl http://localhost:9868/snapshot?tabId=tab_19949f62
```

Or get just the text:

```bash
curl http://localhost:9868/text?tabId=tab_19949f62
```

### 5. Stop Instance

Release the instance and its allocated port:

```bash
curl -X POST http://localhost:9867/instances/inst_a365262a/stop
```

Port 9868 becomes available for the next instance.

---

## Common Workflows

### Parallel Scraping with Multiple Instances

Create 5 headless instances for concurrent scraping:

```bash
#!/bin/bash

URLS=(
  "https://example.com/page1"
  "https://example.com/page2"
  "https://example.com/page3"
  "https://example.com/page4"
  "https://example.com/page5"
)

INSTANCES=()

# Create instances
for i in {1..5}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"scraper-$i\",\"headless\":true}")
  
  ID=$(echo "$INST" | jq -r '.id')
  PORT=$(echo "$INST" | jq -r '.port')
  INSTANCES+=("$ID:$PORT")
  
  echo "Created instance $i: $ID on port $PORT"
done

sleep 2  # Wait for Chrome initialization

# Navigate all instances concurrently
for i in "${!INSTANCES[@]}"; do
  IFS=: read ID PORT <<< "${INSTANCES[$i]}"
  URL="${URLS[$i]}"
  
  curl -s -X POST "http://localhost:$PORT/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}" &
done

wait

echo "All pages loaded. Getting snapshots..."

# Get snapshots
for i in "${!INSTANCES[@]}"; do
  IFS=: read ID PORT <<< "${INSTANCES[$i]}"
  
  curl -s "http://localhost:$PORT/snapshot" > "snapshot-$i.json"
done

# Cleanup
for INST in "${INSTANCES[@]}"; do
  IFS=: read ID PORT <<< "$INST"
  curl -s -X POST "http://localhost:9867/instances/$ID/stop" > /dev/null
done

echo "Done! Scraped ${#URLS[@]} pages in parallel."
```

**Benefits:**
- 5 headless instances = 5x faster scraping than sequential
- No cookie/history interference between sites
- Each instance gets its own port and Chrome process
- Ports automatically reused when instances stop

### Mixed Headed/Headless Testing

Test web app with multiple browser profiles:

```bash
# Headed instance for interactive debugging
WORK=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"dev","headless":false}' | jq -r '.id')

# Headless instance for automated testing
TEST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test","headless":true}' | jq -r '.id')

# Debug instance with preserved state
DEBUG=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"debug","headless":false}' | jq -r '.id')

echo "Dev (headed):     $WORK on port 9868"
echo "Test (headless):  $TEST on port 9869"
echo "Debug (headed):   $DEBUG on port 9870"
```

You now have:
- **dev instance** - visible window for manual testing, inspect elements, debug
- **test instance** - headless for automated test suite, faster
- **debug instance** - separate visible window to preserve state while testing

### Load Distribution with Headless Fleet

Run a fleet of headless instances for high-throughput work:

```bash
#!/bin/bash

FLEET_SIZE=10
INSTANCES=()

echo "Launching fleet of $FLEET_SIZE instances..."

for i in $(seq 1 $FLEET_SIZE); do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"fleet-$i\",\"headless\":true}")
  
  ID=$(echo "$INST" | jq -r '.id')
  PORT=$(echo "$INST" | jq -r '.port')
  INSTANCES+=("$ID:$PORT")
done

echo "Fleet ready on ports 9868-9877"

# Distribute work across instances
WORK_ITEMS=("task1" "task2" "task3" ... "task100")
INSTANCE_INDEX=0

for TASK in "${WORK_ITEMS[@]}"; do
  # Round-robin distribution
  IFS=: read ID PORT <<< "${INSTANCES[$INSTANCE_INDEX % $FLEET_SIZE]}"
  
  # Process task on this instance
  curl -s -X POST "http://localhost:$PORT/navigate" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?task=$TASK\"}" &
  
  ((INSTANCE_INDEX++))
done

wait

# Cleanup fleet
for INST in "${INSTANCES[@]}"; do
  IFS=: read ID PORT <<< "$INST"
  curl -s -X POST "http://localhost:9867/instances/$ID/stop" > /dev/null &
done

wait
echo "Fleet cleaned up"
```

**Performance:**
- 10 instances × 10 concurrent requests = 100 req/sec throughput
- Each instance isolated = no performance degradation
- Easy to scale: add more instances for more throughput

### Multi-Agent Coordination

Have multiple agents work on different instances:

```bash
# Agent A: LinkedIn scraper
AGENT_A_INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"linkedin","headless":true}' | jq -r '.id')

# Agent B: Twitter monitor
AGENT_B_INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"twitter","headless":true}' | jq -r '.id')

# Agent C: News aggregator
AGENT_C_INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"news","headless":true}' | jq -r '.id')

# Each agent has its own isolated browser
# They can run concurrently without interference
```

**Isolation Benefits:**
- Agent A's LinkedIn cookies never touch Agent B's Twitter session
- Agent B's JavaScript errors don't affect Agent C's news scraping
- Each agent can have different profiles, user-agents, timezones
- Clear audit trail: which agent ran on which instance

---

## Instance Lifecycle

### Port Allocation

Ports are automatically allocated from the configured range:

```
Default range: 9868-9968 (100 instances)
```

Configure via environment variables:

```bash
export INSTANCE_PORT_START=9900
export INSTANCE_PORT_END=10000
./pinchtab
```

Now instances will use ports 9900-10000 instead of 9868-9968.

### Port Reuse

When an instance stops, its port is released and can be reused:

```bash
# Create instance
INST1=$(curl -s -X POST http://localhost:9867/instances/launch ... | jq -r '.id')
# Gets port 9868

# Stop instance
curl -X POST http://localhost:9867/instances/$INST1/stop

# Create new instance
INST2=$(curl -s -X POST http://localhost:9867/instances/launch ... | jq -r '.id')
# Gets port 9868 (reused!)
```

This enables:
- Long-running services with dynamic instance pools
- Cost efficiency: only allocate ports as needed
- Graceful scaling: stop old instances, create new ones

### Chrome Initialization

Chrome starts automatically when the instance is created:

```bash
POST /instances/launch → Chrome starts in background
  ↓ (instance ready immediately, no waiting)
  ↓ (Chrome initialization happens concurrently)
  ↓ (1-2 seconds later, Chrome is fully ready)
→ Ready for navigation requests
```

**No lazy initialization:** Instance creation includes full Chrome startup, so first navigation is fast.

### Instance State

Each instance maintains:
- **Profile directory** - Chrome user data (cookies, cache, bookmarks)
- **Browser context** - Open tabs, session storage, local storage
- **History** - Browsing history (unless disabled)
- **Cookies** - Session and persistent cookies per domain
- **IndexedDB/LocalStorage** - Web app state

When instance stops, all state is persisted to disk. Profiles can be reused to resume state.

---

## Monitoring

### List All Instances

```bash
curl http://localhost:9867/instances
```

**Response:**
```json
[
  {
    "id": "inst_a365262a",
    "profileId": "prof_7f3d1e8b",
    "profileName": "work",
    "port": "9868",
    "headless": false,
    "status": "running",
    "startTime": "2026-02-28T21:00:00Z"
  },
  {
    "id": "inst_b471d93c",
    "profileId": "prof_5e2c7f1a",
    "profileName": "test",
    "port": "9869",
    "headless": true,
    "status": "running",
    "startTime": "2026-02-28T21:00:05Z"
  }
]
```

### Instance Health

```bash
# Check specific instance
curl http://localhost:9868/health

# Response
{"status":"ok","tabs":2}
```

### Dashboard Web UI

Open http://localhost:9867/dashboard in a browser to see:
- Live instance list
- Instance status and uptime
- Profile information
- Tab count per instance
- Quick actions (create, stop, delete)

---

## Best Practices

### 1. Name Instances Descriptively

```bash
# Good
{"name":"linkedin-scraper-v1","headless":true}
{"name":"user-test-headed","headless":false}

# Not ideal
{"name":"instance1","headless":true}
{"name":"test","headless":false}
```

Helps with monitoring and debugging.

### 2. Use Profiles for State Management

```bash
# Create with specific profile
POST /instances/launch
{"name":"customer-123","headless":true}

# Profile persists across instance restarts
# Same customer always gets same cookies, history, preferences
```

### 3. Stop Unused Instances

```bash
# Check instance list
curl http://localhost:9867/instances | jq '.[] | select(.status=="stopped")'

# Stop idle instances
for INST in ...; do
  curl -X POST http://localhost:9867/instances/$INST/stop
done
```

Frees ports and resources.

### 4. Batch Operations for Performance

```bash
# Launch 10 instances in parallel
for i in {1..10}; do
  curl -s -X POST http://localhost:9867/instances/launch ... &
done
wait
```

Much faster than sequential creation.

### 5. Handle Errors Gracefully

```bash
# Check instance status before operations
if curl -s http://localhost:9868/health | jq -e '.status == "ok"' > /dev/null; then
  echo "Instance ready, proceeding..."
else
  echo "Instance not ready, waiting..."
  sleep 2
fi
```

---

## Architecture

```
Dashboard (9867)
├─ Orchestrator: manages instances
├─ Profiles manager: manages Chrome profiles
└─ Instance list: view all running instances

Instance 1 (9868, profile=work, headless=false)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process
└─ Tabs: multiple tabs in same browser

Instance 2 (9869, profile=test, headless=true)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process
└─ Tabs: multiple tabs in same browser

Instance 3 (9870, profile=scraping, headless=true)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process
└─ Tabs: multiple tabs in same browser
```

Each instance is **completely isolated**: separate port, separate Chrome process, separate profile.

---

## Troubleshooting

### Instance not starting

```bash
# Check logs
tail -f /var/log/pinchtab.log

# Verify port is free
lsof -i :9868

# Check Chrome availability
which google-chrome chromium-browser chrome
```

### Chrome window not visible (headed mode)

```bash
# Ensure DISPLAY is set (Linux)
export DISPLAY=:0

# Or use Xvfb (Linux headless display)
xvfb-run -a ./pinchtab
```

### Port conflicts

```bash
# If default range is in use, change it
export INSTANCE_PORT_START=10000
export INSTANCE_PORT_END=10100
./pinchtab
```

### Memory usage

Each headless instance: ~50-80 MB
Each headed instance: ~100-150 MB (includes window server overhead)

For 100 instances: ~5-10 GB RAM

Monitor with: `top`, `htop`, or systemd memory limits
