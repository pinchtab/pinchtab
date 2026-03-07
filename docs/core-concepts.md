# Core Concepts

PinchTab is built around two process roles and three main runtime entities:
- **Server**
- **Bridge**
- **Instance**
- **Profile**
- **Tab**

**See also:**
- [Instance API Reference](references/instance-api.md) — Complete instance endpoints
- [Tabs API Reference](references/tabs-api.md) — Tab management endpoints
- [Profile API Reference](references/profile-api.md) — Profile management endpoints

---

## Server

The **full PinchTab server** is the main control-plane process. It manages instances, profiles, routing, and the web dashboard.

- Listens on port `9867` (configurable, dashboard + API)
- Routes requests to the appropriate instance
- Manages instance lifecycle (launch, monitor, stop)
- Provides unified HTTP API for all operations
- Does not talk to Chrome directly for normal managed instances; it delegates to bridge runtimes

```bash
# Start the full server
pinchtab
# Listening on http://localhost:9867

# Or explicitly
pinchtab server

# Or specify port
PINCHTAB_PORT=9870 pinchtab server
# Listening on http://localhost:9870
```

## Bridge

The **bridge** is the single-instance runtime.

- Wraps one browser instance
- Exposes the tab/browser HTTP API
- Used for managed child instances spawned by the server
- Can also be started explicitly with `pinchtab bridge`

```bash
# Explicit bridge mode
pinchtab bridge
```

Most users do not start `bridge` directly. They run the server, and the server launches bridge children as needed.

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

Instances are managed by the server via the API. Managed instances run as isolated bridge child processes.

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
# Terminal 1: Start server
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
  ├── Tab 1: alice@pinchtab.com logged in
  └── Tab 2: alice@pinchtab.com dashboard

Instance 2 (profile: bob)
  ├── Tab 1: bob@pinchtab.com logged in
  └── Tab 2: bob@pinchtab.com dashboard
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

# Open a login tab and log in
TAB_ID=$(curl -s -X POST http://localhost:9867/instances/inst_xyz/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url": "https://pinchtab.com/login"}' | jq -r '.tabId')

curl -X POST http://localhost:9867/tabs/$TAB_ID/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e3","text":"user@pinchtab.com"}'
# ... continue login flow ...

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
curl -X POST http://localhost:9867/instances/inst_0a89a5bb/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url": "https://pinchtab"}' | jq '.tabId'
# Returns: "tab_abc123"

# Or via CLI
pinchtab tab open inst_0a89a5bb https://pinchtab.com

# Get tab info
curl http://localhost:9867/tabs/tab_abc123 | jq .

# Navigate tab
curl -X POST http://localhost:9867/tabs/tab_abc123/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://google.com"}'

# Take snapshot (DOM structure)
curl http://localhost:9867/tabs/tab_abc123/snapshot | jq .

# Interact with tab (click, type, etc.)
curl -X POST http://localhost:9867/tabs/tab_abc123/action \
  -H "Content-Type: application/json" \
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
PinchTab Server (HTTP server on port 9867)
  │
  ├── Instance 1 (inst_0a89a5bb, port 9868, temp profile)
  │     ├── Tab 1 (tab_xyz123, https://pinchtab.com)
  │     ├── Tab 2 (tab_xyz124, https://google.com)
  │     └── Tab 3 (tab_xyz125, https://github.com)
  │
  ├── Instance 2 (inst_1b9a5dcc, port 9869, profile: work)
  │     ├── Tab 1 (tab_abc001, internal dashboard, logged in as alice)
  │     └── Tab 2 (tab_abc002, internal docs)
  │
  └── Instance 3 (inst_2c8a5eef, port 9870, profile: personal)
        ├── Tab 1 (tab_def001, gmail, logged in as bob@pinchtab.com)
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
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://pinchtab.com"}'

curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://google.com"}'

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
curl -X POST http://localhost:9867/instances/$INST_ALICE/tabs/open \
  -d '{"url":"https://app.pinchtab.com"}'

curl -X POST http://localhost:9867/instances/$INST_BOB/tabs/open \
  -d '{"url":"https://app.pinchtab.com"}'

# Login in each instance separately — profiles keep sessions isolated
```

### Workflow 3: Ephemeral Instance (No Profile)

```bash
# Create instance without persistent profile (temporary auto-generated)
INST=$(pinchtab instance launch)

# Create tab, use it
curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://pinchtab.com"}'
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

# Now safe to create a tab and work through tab endpoints
TAB_ID=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}' | jq -r '.tabId')

curl http://localhost:9867/tabs/$TAB_ID/snapshot
```

---

## Mental Model

```
What you control         │ What it is               │ Identified by
─────────────────────────┼──────────────────────────┼─────────────────────
PinchTab Server          │ HTTP control plane       │ port (9867 default)
Instance                 │ Chrome process           │ inst_XXXXXXXX (hash ID)
Profile (optional)       │ Browser state directory  │ prof_XXXXXXXX (hash ID)
Tab                      │ Single webpage           │ tab_XXXXXXXX (hash ID)
```

## Summary

- **PinchTab Server** is the HTTP control plane that manages everything
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
