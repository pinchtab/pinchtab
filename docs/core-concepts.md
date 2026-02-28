# Core Concepts

PinchTab is an HTTP server that controls four key entities: **PinchTab itself**, **Instances**, **Profiles**, and **Tabs**.

---

## PinchTab

The **HTTP server controller** that manages all instances, profiles, and tabs.

- Listens on a port (default `9867`)
- Routes requests to the appropriate instance
- Manages instance lifecycle
- Provides a unified HTTP API

```bash
# Start PinchTab (the controller)
pinchtab
# Listening on http://localhost:9867
```

---

## Instance

A **running Chrome process** with an optional profile, listening on a port.

- One Chrome browser per instance
- Optional profile (see [Profile](#profile) below)
- Can host multiple tabs
- Isolated from other instances
- Identified by port or instance ID

**Key constraint:** One instance = one Chrome process = zero or one profile.

```bash
# Start an instance (default: no profile)
pinchtab
# or with a specific profile
BRIDGE_PROFILE=work pinchtab --port 9867
```

### Multiple Instances

You can run multiple instances in parallel for isolation and scalability:

```bash
# Terminal 1: Instance 1 (no profile)
pinchtab --port 9867

# Terminal 2: Instance 2 (with profile "work")
BRIDGE_PROFILE=work pinchtab --port 9868

# Terminal 3: Instance 3 (with profile "personal")
BRIDGE_PROFILE=personal pinchtab --port 9869
```

Each instance is completely independent — no shared state, no cookie leakage, no resource contention.

---

## Profile

A **browser profile** within an instance. Optional.

- Holds browser state: cookies, local storage, cache, browsing history
- Only one profile per instance
- Multiple tabs can share the same profile (and its state)
- Useful for: user accounts, login sessions, multi-tenant workflows

**Key constraint:** Instance without a profile = no persistent state across restarts.

```bash
# Instance with default (no) profile
pinchtab

# Instance with "work" profile
BRIDGE_PROFILE=work pinchtab

# Instance with "personal" profile
BRIDGE_PROFILE=personal pinchtab
```

### Profile Use Cases

**Separate User Accounts:**
```text
Instance 1 (profile: user-alice)
  ├── Tab 1: alice@example.com logged in
  └── Tab 2: alice@example.com dashboard

Instance 2 (profile: user-bob)
  ├── Tab 1: bob@example.com logged in
  └── Tab 2: bob@example.com dashboard
```

**Login Once, Use Anywhere:**
```bash
# Instance 1: Login
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com/login"}'
# ... fill login form, click submit ...

# Later (even after restart): Reuse session
BRIDGE_PROFILE=work pinchtab  # Profile restored
curl ... # Cookies intact, still logged in
```

---

## Tab

A **browser tab** within an instance (and its profile).

- Single webpage
- Has its own DOM, URL, accessibility tree
- Identified by `tabId`
- Tabs are ephemeral (don't survive instance restart unless using a profile)
- Multiple tabs can be open simultaneously in one instance
- Each tab has stable element references (e0, e1...) for interaction

```bash
# Create tab (returns tabId)
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com"}' | jq '.tabId'
# Returns: "abc123"

# Interact with tab
curl "http://localhost:9867/snapshot?tabId=abc123"
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5","tabId":"abc123"}'

# Close tab
curl -X POST http://localhost:9867/tab \
  -d '{"action":"close","tabId":"abc123"}'
```

---

## Hierarchy

```text
PinchTab (HTTP server controller on port 9867)
  │
  ├── Instance 1 (port 9867, default profile)
  │     ├── Tab 1 (https://example.com)
  │     ├── Tab 2 (https://google.com)
  │     └── Tab 3 (https://github.com)
  │
  ├── Instance 2 (port 9868, profile "work")
  │     ├── Tab 1 (internal dashboard, logged in as alice)
  │     └── Tab 2 (internal docs)
  │
  └── Instance 3 (port 9869, profile "personal")
        ├── Tab 1 (gmail, logged in as bob@example.com)
        └── Tab 2 (bank.com)
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
# Start instance
pinchtab

# Terminal 2: Create tabs and work
curl -X POST http://localhost:9867/tab -d '{"action":"new","url":"https://example.com"}'
curl -X POST http://localhost:9867/tab -d '{"action":"new","url":"https://google.com"}'
curl http://localhost:9867/tabs  # List all tabs
```

### Workflow 2: Multiple Instances, Separate Profiles

```bash
# Terminal 1: Instance for user Alice
BRIDGE_PROFILE=alice pinchtab --port 9867

# Terminal 2: Instance for user Bob
BRIDGE_PROFILE=bob pinchtab --port 9868

# Terminal 3: Create tabs in both
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://app.example.com"}'  # Alice's tab

curl -X POST http://localhost:9868/tab \
  -d '{"action":"new","url":"https://app.example.com"}'  # Bob's tab
```

Both users are logged in simultaneously, cookies isolated.

### Workflow 3: Stateless (No Profile)

```bash
# Start instance without profile
pinchtab

# Create tab, use it, close it
curl -X POST http://localhost:9867/tab -d '{"action":"new","url":"https://example.com"}'
# ... work ...
# Restart instance
pkill pinchtab
pinchtab
# Tab is gone, state is gone
```

---

## Summary

- **PinchTab** is the HTTP server controller
- **Instance** is a Chrome process (optional profile, multiple tabs)
- **Profile** is optional state (cookies, auth, history)
- **Tab** is the actual webpage you interact with

**Key insight:** Instance + Profile + Tabs = the complete mental model for using PinchTab effectively.
