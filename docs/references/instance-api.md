# Instance API Reference

Instances are running Chrome browser processes managed by PinchTab. Each instance is associated with a profile (browser user data directory) and has its own independent browser state.

## Quick Start

### List Running Instances
```bash
# CLI
pinchtab instances

# Curl
curl http://localhost:9867/instances | jq .

# Response
[
  {
    "id": "inst_0a89a5bb",
    "profileId": "prof_278be873",
    "profileName": "Pinchtab org",
    "port": "9868",
    "headless": true,
    "status": "running",
    "startTime": "2026-03-01T05:21:38.27432Z"
  }
]
```

### Start an Instance
```bash
# CLI (all parameters optional)
pinchtab instance start

# CLI with profile
pinchtab instance start --profileId 278be873adeb

# CLI with mode and port
pinchtab instance start --mode headed --port 9999

# Curl
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId": "278be873adeb", "mode": "headed", "port": 9999}'

# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "Pinchtab org",
  "port": "9868",
  "headless": false,
  "status": "starting"
}
```

### Get Instance Logs
```bash
# CLI
pinchtab instance logs inst_ea2e747f
pinchtab instance logs --id inst_ea2e747f

# Curl
curl http://localhost:9867/instances/inst_ea2e747f/logs
```

### Stop an Instance
```bash
# CLI
pinchtab instance stop inst_ea2e747f
pinchtab instance stop --id inst_ea2e747f

# Curl
curl -X POST http://localhost:9867/instances/inst_ea2e747f/stop

# Response
{
  "id": "inst_ea2e747f",
  "status": "stopped"
}
```

---

## Complete API Reference

### 1. List Instances

**Endpoint:** `GET /instances`

**CLI:**
```bash
pinchtab instances
```

**Curl:**
```bash
curl http://localhost:9867/instances | jq .
```

**Response:** Array of Instance objects
```json
[
  {
    "id": "inst_0a89a5bb",
    "profileId": "prof_278be873",
    "profileName": "Pinchtab org",
    "port": "9868",
    "headless": true,
    "status": "running",
    "startTime": "2026-03-01T05:21:38.27432Z"
  }
]
```

**Instance Status Values:**
- `starting` — Instance process spawning, Chrome initializing
- `running` — Instance healthy and ready to accept commands
- `error` — Instance failed to start
- `stopping` — Instance is shutting down

---

### 2. Start Instance

**Endpoint:** `POST /instances/start`

**CLI:**
```bash
# All parameters optional - uses defaults
pinchtab instance start

# With specific profile
pinchtab instance start --profileId 278be873adeb

# With mode and port
pinchtab instance start --profileId abc123 --mode headed --port 9999

# Combine flags
pinchtab instance start --mode headed --port 9998
```

**Curl:**
```bash
# Minimal (all defaults)
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{}'

# With profile ID
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{
    "profileId": "278be873adeb",
    "mode": "headed",
    "port": "9999"
  }'
```

**Request Body (all optional):**
```json
{
  "profileId": "278be873adeb",
  "mode": "headed",
  "port": "9999"
}
```

**Parameters:**
- `profileId` (string, optional) — Profile ID or name. If omitted, creates temporary instance
- `mode` (string, optional, default: "headless") — "headed" or "headless"
- `port` (string, optional) — Port number. If omitted, auto-allocated from available ports

**Response:** Instance object
```json
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "Pinchtab org",
  "port": "9868",
  "headless": false,
  "status": "starting",
  "startTime": "2026-03-01T05:21:38.27432Z"
}
```

**Defaults:**
- `profileId` — If omitted, auto-generates temporary profile name
- `mode` — Defaults to "headless" if not specified
- `port` — Auto-allocated if not specified (finds first available port)

---

### 3. Get Instance Logs

**Endpoint:** `GET /instances/{id}/logs`

**CLI:**
```bash
# Positional argument
pinchtab instance logs inst_ea2e747f

# With --id flag
pinchtab instance logs --id inst_ea2e747f
```

**Curl:**
```bash
curl http://localhost:9867/instances/inst_ea2e747f/logs
```

**Response:** Text (plain text log output)
```
2026-03-01 05:21:38 INFO starting instance process id=inst_ea2e747f profile=Pinchtab org port=9868
2026-03-01 05:21:40 INFO instance ready id=inst_ea2e747f
```

**Notes:**
- Returns raw text output (not JSON)
- Includes timestamps and log levels
- Useful for debugging instance startup issues

---

### 4. Stop Instance

**Endpoint:** `POST /instances/{id}/stop`

**CLI:**
```bash
# Positional argument
pinchtab instance stop inst_ea2e747f

# With --id flag
pinchtab instance stop --id inst_ea2e747f
```

**Curl:**
```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/stop
```

**Response:**
```json
{
  "id": "inst_ea2e747f",
  "status": "stopped"
}
```

**Notes:**
- Gracefully shuts down Chrome process
- Returns immediately (shutdown happens in background)
- Instance moved to stopped state
- Profile is preserved (not deleted)

---

## Complete Workflow Examples

### Example 1: Simple Start and Stop

```bash
#!/bin/bash

# Start headless instance (auto-allocated port, temporary profile)
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r .id)

echo "Started instance: $INST"

# Wait a moment for instance to be ready
sleep 2

# Get logs
echo "Instance logs:"
curl -s http://localhost:9867/instances/$INST/logs | head -5

# Stop instance
curl -s -X POST http://localhost:9867/instances/$INST/stop | jq .

echo "Stopped instance: $INST"
```

### Example 2: Start with Specific Profile

```bash
#!/bin/bash

# Get first available profile ID
PROF=$(curl -s http://localhost:9867/profiles | jq -r '.[0].id')
echo "Using profile: $PROF"

# Start instance with that profile in headed mode
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d "{
    \"profileId\": \"$PROF\",
    \"mode\": \"headed\",
    \"port\": \"9999\"
  }" | jq -r .id)

echo "Started instance: $INST on port 9999"

# List running instances
echo -e "\nRunning instances:"
curl -s http://localhost:9867/instances | jq '.[] | {id, port, mode: (if .headless then "headless" else "headed" end)}'

# Stop when done
curl -s -X POST http://localhost:9867/instances/$INST/stop
```

### Example 3: CLI-Based Workflow

```bash
#!/bin/bash

# Start instance
echo "Starting instance..."
INST=$(pinchtab instance start --mode headed --port 9998 | jq -r .id)
echo "Instance: $INST"

# Wait for ready
sleep 3

# View logs
echo -e "\nInstance logs:"
pinchtab instance logs $INST | tail -5

# List instances
echo -e "\nRunning instances:"
pinchtab instances | jq '.[] | {id, port}'

# Stop
echo -e "\nStopping..."
pinchtab instance stop $INST
```

### Example 4: Multiple Instances

```bash
#!/bin/bash

declare -a INSTANCES

# Start 3 instances with different profiles
PROFILES=$(curl -s http://localhost:9867/profiles | jq -r '.[0:3] | .[] | .id')

i=0
for prof in $PROFILES; do
  PORT=$((9868 + i))
  INST=$(curl -s -X POST http://localhost:9867/instances/start \
    -H "Content-Type: application/json" \
    -d "{\"profileId\":\"$prof\",\"port\":\"$PORT\"}" | jq -r .id)
  INSTANCES+=($INST)
  echo "Started: $INST on port $PORT"
  i=$((i + 1))
done

# Wait and verify
sleep 2
echo -e "\nRunning instances:"
curl -s http://localhost:9867/instances | jq length | xargs echo "Count:"

# Stop all
echo -e "\nStopping all instances..."
for inst in "${INSTANCES[@]}"; do
  curl -s -X POST http://localhost:9867/instances/$inst/stop > /dev/null
  echo "Stopped: $inst"
done
```

---

## CLI Examples

### Quick Start with CLI

```bash
# List instances
pinchtab instances

# Start headless instance
pinchtab instance start

# Start headed instance on specific port
pinchtab instance start --mode headed --port 9999

# Start with profile
pinchtab instance start --profileId 278be873adeb

# Get logs (both syntaxes work)
pinchtab instance logs inst_0a89a5bb
pinchtab instance logs --id inst_0a89a5bb

# Stop instance (both syntaxes work)
pinchtab instance stop inst_0a89a5bb
pinchtab instance stop --id inst_0a89a5bb
```

### Script Example

```bash
#!/bin/bash
# Simple instance management script

case "$1" in
  "start")
    pinchtab instance start --mode $2 --port $3
    ;;
  "logs")
    pinchtab instance logs $2
    ;;
  "stop")
    pinchtab instance stop $2
    ;;
  "list")
    pinchtab instances
    ;;
  *)
    echo "Usage: $0 {start|logs|stop|list} [args]"
    ;;
esac
```

---

## Integration Examples

### With Bash

```bash
# Start instance and capture ID
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -d '{}' | jq -r .id)

# Use instance
PORT=$(curl -s http://localhost:9867/instances | jq -r ".[] | select(.id == \"$INST\") | .port")
echo "Instance $INST running on port $PORT"

# Cleanup
curl -s -X POST http://localhost:9867/instances/$INST/stop
```

### With Python

```python
import requests
import json

BASE = "http://localhost:9867"

# Start instance
resp = requests.post(f"{BASE}/instances/start", json={
    "profileId": "278be873adeb",
    "mode": "headed",
    "port": "9999"
})
inst = resp.json()
print(f"Started: {inst['id']} on port {inst['port']}")

# List instances
instances = requests.get(f"{BASE}/instances").json()
print(f"Total running: {len(instances)}")

# Get logs
logs = requests.get(f"{BASE}/instances/{inst['id']}/logs").text
print("Recent logs:", logs[:200])

# Stop instance
resp = requests.post(f"{BASE}/instances/{inst['id']}/stop")
print(f"Stopped: {resp.json()}")
```

### With JavaScript/Node.js

```javascript
const BASE = "http://localhost:9867";

async function manageInstances() {
  // Start instance
  const startResp = await fetch(`${BASE}/instances/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      profileId: "278be873adeb",
      mode: "headed",
      port: "9999"
    })
  });
  const inst = await startResp.json();
  console.log(`Started: ${inst.id} on port ${inst.port}`);

  // List instances
  const listResp = await fetch(`${BASE}/instances`);
  const instances = await listResp.json();
  console.log(`Total running: ${instances.length}`);

  // Get logs
  const logsResp = await fetch(`${BASE}/instances/${inst.id}/logs`);
  const logs = await logsResp.text();
  console.log("Recent logs:", logs.substring(0, 200));

  // Stop instance
  const stopResp = await fetch(`${BASE}/instances/${inst.id}/stop`, {
    method: "POST"
  });
  const stopped = await stopResp.json();
  console.log(`Stopped: ${stopped.id}`);
}

manageInstances().catch(console.error);
```

---

## Error Handling

### Profile Not Found (404)

```bash
curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"nonexistent"}'

# Response (400 Bad Request)
{
  "error": "profile \"nonexistent\" not found",
  "statusCode": 404
}
```

### Port Already in Use (409)

```bash
curl -X POST http://localhost:9867/instances/start \
  -d '{"port":"9867"}'

# Response (409 Conflict)
{
  "error": "port 9867 already in use",
  "statusCode": 409
}
```

### Instance Not Found (404)

```bash
curl http://localhost:9867/instances/nonexistent/logs

# Response (404 Not Found)
{
  "error": "instance not found",
  "statusCode": 404
}
```

### Invalid JSON (400)

```bash
curl -X POST http://localhost:9867/instances/start \
  -d 'invalid json'

# Response (400 Bad Request)
{
  "error": "invalid JSON",
  "statusCode": 400
}
```

---

## Best Practices

### Port Management

```bash
# Let PinchTab auto-allocate ports (recommended)
pinchtab instance start

# Or manually specify if you need a specific port
pinchtab instance start --port 9999
```

### Profile Selection

```bash
# Start with specific profile to preserve state
pinchtab instance start --profileId 278be873adeb

# Or use temporary profile (good for isolated testing)
pinchtab instance start
```

### Cleanup

```bash
# Always stop instances when done
pinchtab instance stop $INSTANCE_ID

# Check for orphaned instances
pinchtab instances | jq length
```

### Monitoring

```bash
# Check instance status
pinchtab instances | jq '.[] | {id, port, status}'

# View recent logs
pinchtab instance logs $INSTANCE_ID | tail -20
```

---

## Instance Lifecycle

```
START (POST /instances/start)
  ↓
[status: "starting"] — Chrome initializing, health checks running
  ↓
[status: "running"] — Ready to accept commands
  ↓
STOP (POST /instances/{id}/stop)
  ↓
[status: "stopping"] — Graceful shutdown
  ↓
Instance removed from list, profile preserved
```

---

## Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| **200** | Success (GET) | List retrieved, logs retrieved |
| **201** | Created | Instance started successfully |
| **400** | Bad request | Invalid JSON, missing required field |
| **404** | Not found | Instance/profile not found |
| **409** | Conflict | Port already in use, launch error |
| **500** | Server error | Internal error |

---

## Summary Table

| Operation | Method | Endpoint | CLI |
|-----------|--------|----------|-----|
| List | GET | `/instances` | `pinchtab instances` |
| Start | POST | `/instances/start` | `pinchtab instance start [opts]` |
| Logs | GET | `/instances/{id}/logs` | `pinchtab instance logs <id>` |
| Stop | POST | `/instances/{id}/stop` | `pinchtab instance stop <id>` |

---

## FAQ

**Q: What's the difference between headless and headed mode?**
A: Headless runs without a GUI window (good for servers), headed shows the browser window (good for debugging).

**Q: Can I run multiple instances simultaneously?**
A: Yes, each gets its own port and Chrome process.

**Q: What happens to my profile when I stop an instance?**
A: Profile is preserved. Stop just closes the browser. You can start the same instance again.

**Q: How do I choose between temporary and named profiles?**
A: Use named profiles when you need to preserve state (logins, settings). Use temporary when testing.

**Q: Can I change the port after starting?**
A: No, port is assigned on start. Stop and restart with a new port if needed.

**Q: How do I know when an instance is ready?**
A: Status transitions from "starting" to "running" when healthy.

---

## Related Documentation

- **Profile API** (docs/references/profile-api.md) — Manage browser profiles
- **Tab API** (docs/references/tab-api.md — coming soon) — Control tabs within instances
- **CLI Design** (docs/references/cli-design.md) — CLI command patterns
