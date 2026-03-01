# Multi-Instance Workflows

Pinchtab's multi-instance architecture enables running multiple isolated Chrome browsers simultaneously, each with their own cookies, history, profiles, and configurations.

## Getting Started

### 1. Start the Orchestrator

```bash
pinchtab
```

The orchestrator runs on **port 9867** and manages all instances.

### 2. Create Your First Instance

Create a headed instance (visible window) for interactive work:

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}'
```

**Response:**
```json
{
  "id": "inst_a365262a",
  "profileId": "prof_7f3d1e8b",
  "profileName": "instance-1672531200",
  "port": "9868",
  "headless": false,
  "status": "starting",
  "startTime": "2026-03-01T06:30:00Z"
}
```

The instance gets:
- **Auto-allocated port** (9868, 9869, ...) - unique per instance
- **Hash-based ID** (`inst_XXXXXXXX`) - stable and portable
- **Temporary profile** (auto-generated, deleted on stop)
- **Chrome process** - running independently on its port
- **Lazy initialization** - Chrome starts on first request

### 3. Navigate and Interact

Navigate the instance via the orchestrator (recommended):

```bash
curl -X POST http://localhost:9867/instances/inst_a365262a/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

**Response:**
```json
{
  "id": "tab_19949f62",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

Each navigation automatically creates a new tab. Use the `id` (tabId) for subsequent operations on that specific tab.

**Note:** You can also access the instance directly on its port (9868), but using the orchestrator proxy is recommended for consistency.

### 4. Get Page Content

Retrieve page snapshot using the tab ID:

```bash
TAB=$(curl -s -X POST http://localhost:9867/instances/inst_a365262a/tabs/open \
  -d '{"url":"https://example.com"}' | jq -r '.id')

curl http://localhost:9867/tabs/$TAB/snapshot
```

Or get just the text:

```bash
curl http://localhost:9867/tabs/$TAB/text
```

### 5. Stop Instance

Release the instance and its allocated port:

```bash
curl -X POST http://localhost:9867/instances/inst_a365262a/stop
```

Port 9868 becomes available for reuse by the next instance. The temporary profile is automatically deleted.

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

echo "Creating 5 headless instances..."

# Create instances (in parallel for speed)
for i in {1..5}; do
  curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' > /tmp/inst_$i.json &
done
wait

# Parse instance IDs
for i in {1..5}; do
  ID=$(jq -r '.id' /tmp/inst_$i.json)
  INSTANCES+=("$ID")
  echo "Created instance $i: $ID"
done

# Wait for instances to initialize (lazy Chrome start)
sleep 2

echo "Navigating all instances concurrently..."

# Navigate all instances and capture tab IDs
TABS=()
for i in "${!INSTANCES[@]}"; do
  ID="${INSTANCES[$i]}"
  URL="${URLS[$i]}"

  RESPONSE=$(curl -s -X POST "http://localhost:9867/instances/$ID/tabs/open" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"$URL\"}")
  TAB_ID=$(echo "$RESPONSE" | jq -r '.id')
  TABS+=("$TAB_ID")
done

echo "All pages loaded. Getting snapshots..."

# Get snapshots using tab IDs
for i in "${!TABS[@]}"; do
  TAB="${TABS[$i]}"

  curl -s "http://localhost:9867/tabs/$TAB/snapshot" > "snapshot-$i.json" &
done
wait

# Cleanup
echo "Stopping instances..."
for ID in "${INSTANCES[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$ID/stop" &
done
wait

echo "Done! Scraped ${#URLS[@]} pages in parallel."
```

**Benefits:**
- 5 headless instances = 5x faster scraping than sequential
- No cookie/history interference between sites
- Each instance gets isolated Chrome process and temporary profile
- Ports automatically allocated (9868-9872) and reused on stop
- All API calls go through orchestrator proxy for consistency

### Mixed Headed/Headless Testing

Test web app with multiple instances in different modes:

```bash
echo "Creating mixed instance setup for testing..."

# Headed instance for interactive debugging
WORK=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}' | jq -r '.id')

# Headless instance for automated testing (faster)
TEST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

# Another headed instance with persistent profile for state preservation
pinchtab profile create testing-profile
PROFILE_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="testing-profile") | .id')

DEBUG=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$PROFILE_ID'","mode":"headed"}' | jq -r '.id')

echo "Dev (headed):     $WORK (temporary profile, no persistence)"
echo "Test (headless):  $TEST (temporary profile, fast)"
echo "Debug (headed):   $DEBUG (persistent profile, state saved)"
```

You now have three independent instances:
- **dev instance** - Visible window, temporary profile (good for quick testing)
- **test instance** - Headless, temporary profile (best for automated tests, no display overhead)
- **debug instance** - Visible window, persistent profile (login state preserved, can reconnect later)

### Load Distribution with Headless Fleet

Run a fleet of headless instances for high-throughput work:

```bash
#!/bin/bash

FLEET_SIZE=10
INSTANCES=()

echo "Launching fleet of $FLEET_SIZE headless instances..."

# Create instances in parallel
for i in $(seq 1 $FLEET_SIZE); do
  curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' > /tmp/inst_$i.json &
done
wait

# Collect instance IDs
for i in $(seq 1 $FLEET_SIZE); do
  ID=$(jq -r '.id' /tmp/inst_$i.json)
  INSTANCES+=("$ID")
done

echo "Fleet ready: ${#INSTANCES[@]} instances allocated (ports 9868-907$((9868 + FLEET_SIZE - 1)))"

# Wait for Chrome initialization
sleep 3

echo "Processing 100 tasks with round-robin distribution..."

# Distribute work across instances
TASKS=($(seq 1 100))
INSTANCE_INDEX=0

# Process tasks in parallel (10 at a time)
for TASK in "${TASKS[@]}"; do
  # Round-robin: assign task to next instance in rotation
  ID="${INSTANCES[$((INSTANCE_INDEX % $FLEET_SIZE))]}"

  # Send task to instance via orchestrator proxy
  curl -s -X POST "http://localhost:9867/instances/$ID/tabs/open" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://api.example.com?task=$TASK\"}" > /dev/null &

  # Keep 10 requests in flight at a time
  if (( (INSTANCE_INDEX + 1) % 10 == 0 )); then
    wait
  fi

  ((INSTANCE_INDEX++))
done

wait

# Cleanup fleet
echo "Cleaning up fleet..."
for ID in "${INSTANCES[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$ID/stop" > /dev/null &
done
wait

echo "Fleet cleaned up"
```

**Performance:**
- 10 instances = 10x parallelism (10 concurrent navigations)
- Round-robin distribution = balanced load across fleet
- Each instance isolated = no resource contention
- Easy to scale: increase FLEET_SIZE for more throughput

**Scaling tips:**
- For 100 instances: Allocate 5-10 GB RAM + adjust ulimits
- Monitor: `curl http://localhost:9867/instances | jq 'length'`
- Cleanup: Remove instances when done to free resources

### Multi-Agent Coordination

Have multiple agents work on different instances with isolated state:

```bash
# Create profiles for each agent (optional but recommended)
pinchtab profile create agent-linkedin
pinchtab profile create agent-twitter
pinchtab profile create agent-news

# Get profile IDs
LINKEDIN_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="agent-linkedin") | .id')
TWITTER_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="agent-twitter") | .id')
NEWS_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="agent-news") | .id')

echo "Starting agents..."

# Agent A: LinkedIn scraper with persistent profile
AGENT_A=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$LINKEDIN_ID'","mode":"headless"}' | jq -r '.id')

# Agent B: Twitter monitor with persistent profile
AGENT_B=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$TWITTER_ID'","mode":"headless"}' | jq -r '.id')

# Agent C: News aggregator with persistent profile
AGENT_C=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$NEWS_ID'","mode":"headless"}' | jq -r '.id')

echo "Agents running:"
echo "  LinkedIn agent: $AGENT_A"
echo "  Twitter agent:  $AGENT_B"
echo "  News agent:     $AGENT_C"

# Each agent has its own isolated browser with persistent state
# They can run concurrently without any interference
```

**Isolation Benefits:**
- Agent A's LinkedIn login state saved to `agent-linkedin` profile
- Agent B's Twitter cookies never touch Agent A's LinkedIn cookies
- Agent C can run in parallel without affecting others
- State persists: agents can reconnect later and resume work
- Clear audit trail: which agent ran on which instance, when

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

Chrome uses **lazy initialization**: it starts when first needed, not at instance creation.

```bash
POST /instances/launch → Instance created immediately
  ↓ status: "starting" (orchestrator monitor probes /health)
  ↓ First /health probe triggers Chrome initialization (8-20 seconds)
  ↓ Chrome starts, TabManager initializes
  ↓ Monitor's health check succeeds
  ↓ status: "running" (instance ready for requests)
→ Ready for navigation requests
```

**Why lazy init?** Reduces memory overhead for ephemeral instances, initialization happens automatically on first request.

**Wait for ready state:**
```bash
# Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch | jq -r '.id')

# Poll until running (Chrome will initialize during health checks)
while [ "$(curl -s http://localhost:9867/instances/$INST | jq -r '.status')" != "running" ]; do
  sleep 0.5
done

# Now safe to use
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}'
```

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
curl http://localhost:9867/instances | jq .
```

**Response:**
```json
[
  {
    "id": "inst_a365262a",
    "profileId": "prof_7f3d1e8b",
    "profileName": "instance-1672531200",
    "port": "9868",
    "headless": false,
    "status": "running",
    "startTime": "2026-03-01T06:30:00Z"
  },
  {
    "id": "inst_b471d93c",
    "profileId": "prof_5e2c7f1a",
    "profileName": "instance-1672531205",
    "port": "9869",
    "headless": true,
    "status": "running",
    "startTime": "2026-03-01T06:30:05Z"
  }
]
```

### Get Specific Instance Info

```bash
# Check status of specific instance
curl http://localhost:9867/instances/inst_a365262a | jq .

# Response
{
  "id": "inst_a365262a",
  "port": "9868",
  "headless": false,
  "status": "running",
  "startTime": "2026-03-01T06:30:00Z",
  "profileName": "instance-1672531200"
}
```

### Instance Health and Tabs

```bash
# Check health and tab count for specific instance
curl http://localhost:9867/instances/inst_a365262a/health

# Response
{
  "status": "ok",
  "tabs": 2
}

# List tabs in instance
curl http://localhost:9867/instances/inst_a365262a/tabs | jq .
```

### Get Instance Logs

```bash
# View logs for debugging
curl http://localhost:9867/instances/inst_a365262a/logs | head -50
```

### Dashboard Web UI

Open http://localhost:9867 in a browser to see:
- Live instance list (real-time updates)
- Instance status, port, mode, uptime
- Profile information per instance
- Tab count per instance
- Quick actions (view logs, stop instance)

---

## Best Practices

### 1. Use Persistent Profiles for Important Sessions

```bash
# For agents that need persistent login state
pinchtab profile create linkedin-scraper

PROFILE_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="linkedin-scraper") | .id')

# Start instance with profile
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$PROFILE_ID'","mode":"headless"}' | jq -r '.id')

# Login once, reuse session on next run
```

Persistent profiles preserve cookies, history, login state across restarts.

### 2. Use Ephemeral (Temporary) Profiles for Isolation

```bash
# For one-off tasks, temporary profiles auto-delete on stop
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

# ... do work ...

# Stop instance -> profile automatically deleted
curl -X POST http://localhost:9867/instances/$INST/stop
```

Temporary profiles are auto-generated and deleted, good for clean slates.

### 3. Wait for Instance Ready Before Using

```bash
# Always poll status before first request
INST=$(curl -s -X POST http://localhost:9867/instances/launch | jq -r '.id')

# Wait for Chrome initialization
while [ "$(curl -s http://localhost:9867/instances/$INST | jq -r '.status')" != "running" ]; do
  sleep 0.5
done

# Now safe to navigate
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}'
```

Chrome lazy-initializes on first health check (~8-20 seconds).

### 4. Batch Operations for Performance

```bash
# Create 10 instances in parallel (faster than sequential)
for i in {1..10}; do
  curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' > /tmp/inst_$i.json &
done
wait

# Extract IDs
for i in {1..10}; do
  jq -r '.id' /tmp/inst_$i.json
done
```

Much faster than sequential instance creation.

### 5. Clean Up Stopped Instances

```bash
# List all instances
curl http://localhost:9867/instances | jq '.[] | {id, status}'

# Stop idle instances to free resources
curl http://localhost:9867/instances | jq -r '.[] | select(.status=="running") | .id' | while read ID; do
  curl -s -X POST http://localhost:9867/instances/$ID/stop
done
```

Frees ports and RAM for new instances.

### 6. Route All Requests Through Orchestrator

```bash
# DO: Use orchestrator proxy (recommended)
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}'

# AVOID: Direct port access (less consistent)
curl -X POST http://localhost:$PORT/tabs/open \
  -d '{"url":"https://example.com"}'
```

Orchestrator proxy ensures consistent routing and better monitoring.

### 7. Monitor Resource Usage

```bash
# Check instance count
INST_COUNT=$(curl -s http://localhost:9867/instances | jq 'length')
echo "Running instances: $INST_COUNT"

# Estimate RAM (headless ~80MB, headed ~150MB each)
HEADLESS=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.headless==true)] | length')
HEADED=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.headless==false)] | length')
echo "Headless: $HEADLESS (~$((HEADLESS * 80))MB)"
echo "Headed: $HEADED (~$((HEADED * 150))MB)"
```

Keep an eye on resource usage to avoid exhausting system limits.

---

## Architecture

```
Orchestrator (port 9867)
├─ Instance manager: lifecycle (create, monitor, stop)
├─ Profile manager: persistent browser state directories
├─ Port allocator: assigns ports 9868-9968
└─ Dashboard UI: monitor instances, create/stop via web

Instance 1 (port 9868, temp profile)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process (lazy-init)
├─ Profile: temporary, deleted on stop
└─ Tabs: multiple tabs in same browser

Instance 2 (port 9869, profile=work)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process (lazy-init)
├─ Profile: persistent (work), survives restarts
└─ Tabs: multiple tabs in same browser

Instance 3 (port 9870, profile=linkedin)
├─ Bridge: HTTP API for this instance only
├─ Chrome: independent browser process (lazy-init)
├─ Profile: persistent (linkedin), survives restarts
└─ Tabs: multiple tabs in same browser
```

Each instance is **completely isolated**:
- **Separate port** (9868-9968, auto-allocated)
- **Separate Chrome process** (independent browser, no cross-contamination)
- **Separate profile** (cookies, history, cache all isolated)
- **Separate HTTP API** (Bridge server on instance port)

**Orchestrator forwards requests** from `9867/instances/{id}/*` to the appropriate instance port for clean routing.

---

## Troubleshooting

### Instance Not Starting (Status: "error")

```bash
# Check instance error details
curl http://localhost:9867/instances/$INST | jq '.error'

# Get full logs
curl http://localhost:9867/instances/$INST/logs

# Common issues:
# - "Chrome binary not found" → Set CHROME_BIN env var
# - "Port already in use" → Change INSTANCE_PORT_START/END
# - "process exited early" → Check logs for Chrome crash reason
```

### Instance Stuck in "starting"

```bash
# Wait longer (Chrome init takes 8-20 seconds)
sleep 10

# Check status again
curl http://localhost:9867/instances/$INST | jq '.status'

# If still "starting" after 30s, likely Chrome init failed
# Check logs:
curl http://localhost:9867/instances/$INST/logs | tail -20
```

### Chrome Window Not Visible (Headed Mode)

```bash
# On Linux, ensure DISPLAY is set
echo $DISPLAY  # Should show :0 or :1, etc.

# If not set, enable X11 forwarding
export DISPLAY=:0

# For SSH access, use X11 forwarding
ssh -X user@server
pinchtab instance launch --mode headed
```

### Port Conflicts

```bash
# Check which ports are in use
lsof -i :9868  # Check specific port
netstat -tlnp | grep 986  # Check range

# If port conflict, change instance port range
export INSTANCE_PORT_START=10000
export INSTANCE_PORT_END=10100
pinchtab

# Or specify port for specific instance
curl -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headless","port":"10000"}'
```

### Chrome Binary Not Found

```bash
# Check if Chrome is installed
which google-chrome
which chromium
which chromium-browser

# If not found, install Chrome/Chromium

# Or set explicit path
export CHROME_BIN=/path/to/chrome
pinchtab
```

### Memory Usage Growing

```bash
# Monitor instance count and resource usage
curl http://localhost:9867/instances | jq 'length'

# Estimate RAM per instance
# Headless: ~80 MB each
# Headed: ~150 MB each

# Stop old instances to free memory
curl -X POST http://localhost:9867/instances/$OLD_INST/stop

# For limits, set systemd cgroup or Docker memory limit
# docker run --memory=8g pinchtab
```

### Can't Access Instance After Stop/Start

```bash
# Instance IDs are stable, but ports change on restart
# Always use orchestrator proxy:
curl http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}'

# Don't rely on cached port numbers
PORT=$(curl http://localhost:9867/instances/$INST | jq -r '.port')
```
