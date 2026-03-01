# Core Concepts

PinchTab is an HTTP server that controls four key entities: **PinchTab itself**, **Instances**, **Profiles**, and **Tabs**.

**See also:**
- [Instance API Reference](references/instance-api.md) — Complete instance endpoints
- [Tabs API Reference](references/tabs-api.md) — Tab management endpoints
- [Profile API Reference](references/profile-api.md) — Profile management endpoints

---

## PinchTab

The **HTTP server controller** (orchestrator) that manages all instances, profiles, and tabs.

- Listens on port `9867` (configurable, dashboard + API)
- Routes requests to the appropriate instance
- Manages instance lifecycle (launch, monitor, stop)
- Provides unified HTTP API for all operations
- No Chrome process itself — purely orchestrator

```bash
# Start PinchTab orchestrator (default: port 9867)
pinchtab
# Listening on http://localhost:9867

# Or specify port
BRIDGE_PORT=9870 pinchtab
# Listening on http://localhost:9870
```

---

## Instance

A **running Chrome process** with an optional profile, auto-allocated to a unique port (9868-9968 by default).

- One Chrome browser per instance
- Optional profile (see [Profile](#profile) below)
- Can host multiple tabs
- Completely isolated from other instances
- Identified by instance ID: `inst_XXXXXXXX` (hash-based, stable)
- Auto-allocated to unique port in 9868-9968 range
- Lazy Chrome initialization (starts on first request, not at creation)

**Key constraint:** One instance = one Chrome process = zero or one profile.

### Creating Instances

Instances are managed by the orchestrator via the API (not by running separate processes).

```bash
# CLI: Create instance (headless by default)
pinchtab instance launch

# CLI: Create headed (visible) instance
pinchtab instance launch --mode headed

# CLI: Create with specific port
pinchtab instance launch --mode headed --port 9999

# Curl: Create instance via API
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode": "headed", "port": "9999"}'

# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "Instance-...",
  "port": "9868",
  "headless": false,
  "status": "starting"
}
```

### Multiple Instances

You can run multiple instances simultaneously for isolation and scalability. The orchestrator manages them automatically:

```bash
# Terminal 1: Start orchestrator
pinchtab

# Terminal 2: Create multiple instances
for i in 1 2 3; do
  pinchtab instance launch --mode headless
done

# List all instances
curl http://localhost:9867/instances | jq .

# Response: 3 independent instances on ports 9868, 9869, 9870
[
  {"id": "inst_0a89a5bb", "port": "9868", "status": "running"},
  {"id": "inst_1b9a5dcc", "port": "9869", "status": "running"},
  {"id": "inst_2c8a5eef", "port": "9870", "status": "running"}
]
```

Each instance is completely independent — no shared state, no cookie leakage, no resource contention.

---

## Profile

A **browser profile** (Chrome user data directory) containing browser state. Optional per instance.

- Holds browser state: cookies, local storage, cache, browsing history, extensions
- Only one profile per instance
- Multiple tabs can share the same profile (and its state)
- Identified by profile ID: `prof_XXXXXXXX` (hash-based, stable)
- Useful for: user accounts, login sessions, multi-tenant workflows
- Persistent across instance restarts

**Key constraint:** Instance without a profile = ephemeral, no persistent state across restarts.

### Managing Profiles

```bash
# CLI: List all profiles
pinchtab profiles

# CLI: Create profile
pinchtab profile create my-profile

# Curl: List profiles (excludes temporary auto-generated profiles)
curl http://localhost:9867/profiles | jq .

# Response
[
  {
    "id": "278be873",
    "name": "my-profile",
    "created": "2026-03-01T05:21:38.274Z",
    "diskUsage": 5242880,
    "source": "created"
  }
]
```

### Using Profiles with Instances

```bash
# Create instance with specific profile
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId": "278be873"}'

# Or via CLI
pinchtab instance launch  # Uses temp auto-generated profile
```

### Profile Use Cases

**Separate User Accounts:**
```text
Instance 1 (profile: alice)
  ├── Tab 1: alice@example.com logged in
  └── Tab 2: alice@example.com dashboard

Instance 2 (profile: bob)
  ├── Tab 1: bob@example.com logged in
  └── Tab 2: bob@example.com dashboard
```

```bash
# Create profiles for each user
pinchtab profile create alice
pinchtab profile create bob

# Start instances with profiles
curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId": "alice-profile-id"}'

curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId": "bob-profile-id"}'

# Each instance has isolated cookies/auth
```

**Login Once, Use Anywhere:**
```bash
# Start instance with persistent profile
curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId": "work"}'

# Navigate and log in
curl -X POST http://localhost:9867/instances/inst_xyz/navigate \
  -d '{"url": "https://example.com/login"}'
# ... fill login form, click submit ...

# Later (even after instance restart): Profile is persistent
pinchtab instance launch  # Or restart orchestrator
# Cookies intact, still logged in via profile's saved state
```

---

## Tab

A **browser tab** (webpage) within an instance and its profile.

- Single webpage with its own DOM, URL, accessibility tree
- Identified by tab ID: `tab_XXXXXXXX` (hash-based, stable)
- Tabs are ephemeral (don't survive instance restart unless using a profile)
- Multiple tabs can be open simultaneously in one instance
- Each tab has stable element references (e0, e1...) for DOM interaction
- Can navigate, take snapshots, execute actions, evaluate JavaScript

```bash
# Create tab in instance (returns tabId)
curl -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"instanceId": "inst_0a89a5bb", "url": "https://example.com"}' | jq '.id'
# Returns: "tab_abc123"

# Or via CLI
pinchtab tab open inst_0a89a5bb https://example.com

# Get tab info
curl http://localhost:9867/tabs/tab_abc123 | jq .

# Navigate tab
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/navigate \
  -d '{"url": "https://google.com"}'

# Take snapshot (DOM structure)
curl http://localhost:9867/instances/inst_0a89a5bb/snapshot | jq .

# Interact with tab (click, type, etc.)
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/action \
  -d '{"kind": "click", "ref": "e5"}'

# Close tab
curl -X POST http://localhost:9867/tabs/tab_abc123/close

# Or via CLI
pinchtab tab close tab_abc123
```

**See:** [Tabs API Reference](references/tabs-api.md) for complete operations.

---

## Hierarchy

```text
PinchTab Orchestrator (HTTP server on port 9867)
  │
  ├── Instance 1 (inst_0a89a5bb, port 9868, temp profile)
  │     ├── Tab 1 (tab_xyz123, https://example.com)
  │     ├── Tab 2 (tab_xyz124, https://google.com)
  │     └── Tab 3 (tab_xyz125, https://github.com)
  │
  ├── Instance 2 (inst_1b9a5dcc, port 9869, profile: work)
  │     ├── Tab 1 (tab_abc001, internal dashboard, logged in as alice)
  │     └── Tab 2 (tab_abc002, internal docs)
  │
  └── Instance 3 (inst_2c8a5eef, port 9870, profile: personal)
        ├── Tab 1 (tab_def001, gmail, logged in as bob@example.com)
        └── Tab 2 (tab_def002, bank.com)
```

---

## Relationships & Constraints

| Relationship | Rule |
|---|---|
| **Tabs → Instance** | Every tab must exist in exactly one instance |
| **Tabs → Profile** | Every tab inherits the instance's profile (zero or one) |
| **Profile → Instance** | Every profile belongs to exactly one instance |
| **Instance → Profile** | An instance has zero or one profile |
| **Instance → Chrome** | One instance = one Chrome process |

---

## Common Workflows

### Workflow 1: Single Instance, Multiple Tabs

```bash
# Terminal 1: Start orchestrator
pinchtab

# Terminal 2: Create instance
INST=$(pinchtab instance launch --mode headless)
# Returns: inst_0a89a5bb

# Create multiple tabs in the same instance
curl -X POST http://localhost:9867/tabs/open \
  -d '{"instanceId":"'$INST'","url":"https://example.com"}'

curl -X POST http://localhost:9867/tabs/open \
  -d '{"instanceId":"'$INST'","url":"https://google.com"}'

# List all tabs across all instances
curl http://localhost:9867/tabs | jq .

# Or tabs in specific instance
curl http://localhost:9867/instances/$INST/tabs | jq .
```

### Workflow 2: Multiple Instances, Separate Profiles

```bash
# Create persistent profiles for Alice and Bob
pinchtab profile create alice
pinchtab profile create bob

# Get profile IDs
ALICE_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="alice") | .id')
BOB_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="bob") | .id')

# Start instance for Alice
INST_ALICE=$(curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$ALICE_ID'"}' | jq -r '.id')

# Start instance for Bob
INST_BOB=$(curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$BOB_ID'"}' | jq -r '.id')

# Create tabs in both instances with isolated cookies
curl -X POST http://localhost:9867/tabs/open \
  -d '{"instanceId":"'$INST_ALICE'","url":"https://app.example.com"}'

curl -X POST http://localhost:9867/tabs/open \
  -d '{"instanceId":"'$INST_BOB'","url":"https://app.example.com"}'

# Login in each instance separately — profiles keep sessions isolated
```

### Workflow 3: Ephemeral Instance (No Profile)

```bash
# Create instance without persistent profile (temporary auto-generated)
INST=$(pinchtab instance launch)

# Create tab, use it
curl -X POST http://localhost:9867/tabs/open \
  -d '{"instanceId":"'$INST'","url":"https://example.com"}'
# ... work ...

# Stop instance
pinchtab instance stop $INST

# Tab is gone, all cookies gone — clean slate next time
```

### Workflow 4: Polling for Instance Ready Status

```bash
# Create instance (returns with status "starting")
INST=$(pinchtab instance launch | jq -r '.id')

# Poll until running (monitor's health check initializes Chrome)
while true; do
  STATUS=$(curl http://localhost:9867/instances/$INST | jq -r '.status')
  if [ "$STATUS" == "running" ]; then
    echo "Instance ready!"
    break
  fi
  echo "Instance status: $STATUS, waiting..."
  sleep 0.5
done

# Now safe to make requests to the instance
curl -X POST http://localhost:9867/instances/$INST/navigate \
  -d '{"url":"https://example.com"}'
```

---

## Mental Model

```
What you control         │ What it is               │ Identified by
─────────────────────────┼──────────────────────────┼─────────────────────
PinchTab Orchestrator    │ HTTP server controller   │ port (9867 default)
Instance                 │ Chrome process           │ inst_XXXXXXXX (hash ID)
Profile (optional)       │ Browser state directory  │ prof_XXXXXXXX (hash ID)
Tab                      │ Single webpage           │ tab_XXXXXXXX (hash ID)
```

## Summary

- **PinchTab Orchestrator** is the HTTP server that manages everything
- **Instance** is a running Chrome process with optional profile and multiple tabs
- **Profile** is optional persistent browser state (cookies, auth, history)
- **Tab** is the actual webpage you navigate and interact with

**Key insights:**
- Instances are launched via API and auto-allocated unique ports (9868-9968)
- Instances are lazy: Chrome initializes on first request, not at creation time
- Profiles are optional but provide persistent state across instance restarts
- Tabs are ephemeral unless using a persistent profile
- Instance + Profile + Tabs = the complete mental model for using PinchTab effectively

**Next:** See [Instance API Reference](references/instance-api.md), [Tabs API Reference](references/tabs-api.md), and [Profile API Reference](references/profile-api.md) for complete endpoint documentation.
