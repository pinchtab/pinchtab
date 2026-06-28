# Agent Sessions

Agent sessions provide durable, revocable authentication for automated agents. Instead of sharing the server bearer token, each agent gets its own session token that maps to a specific `agentId`.

## Overview

- **Session token**: `ses_<48 hex chars>` — high-entropy, never stored raw (only SHA-256 hash persisted)
- **Session ID**: `ses_<16 hex chars>` — public identifier for management
- **Auth header**: `Authorization: Session <token>`
- **Env var**: `PINCHTAB_SESSION` — CLI auto-detects and uses session auth

## Configuration

In `config.json`:

```json
{
  "sessions": {
    "agent": {
      "enabled": true,
      "mode": "preferred",
      "idleTimeoutSec": 1800,
      "maxLifetimeSec": 86400
    }
  }
}
```

### Modes

| Mode | Behavior |
|------|----------|
| `off` | Agent sessions disabled |
| `preferred` | Both bearer and session auth accepted (default) |
| `required` | Only session auth accepted for agents |

## Lifecycle

1. **Create** — `pinchtab session create --agent-id <id>` (or `POST /sessions` directly)
2. **Use** — agent sends `Authorization: Session ses_...` with each request, or sets `PINCHTAB_SESSION`
3. **Revoke** — `pinchtab session revoke <session-id>` (or `POST /sessions/{id}/revoke`)

## Security

- Tokens are never logged or persisted in plaintext
- SHA-256 hash comparison using `crypto/subtle.ConstantTimeCompare`
- Idle timeout (default 30m) and max lifetime (default 24h)
- Sessions persisted to `agent-sessions.json` (atomic writes)
- Each session bound to a specific agentId for activity tracking

> **⚠️ Trusted, controlled environments only.** Agent sessions are meant for operators and automation you already trust: local machines, private networks, CI, or other controlled systems. They are not a multi-tenant isolation boundary and should not be treated as safe for untrusted users, untrusted agents, or public internet exposure.
>
> The session management API (`/sessions`) still has admin-style authority for create, list, and inspect operations. Any caller authenticated with the server bearer token or a valid dashboard cookie can manage sessions for any agent. Session-authenticated callers are blocked from dashboard/admin endpoint families, but a session without explicit grants can still access the normal non-admin automation surface by default.
>
> In untrusted or shared environments where agent sessions are not needed, disable them entirely by setting `"enabled": false` or `"mode": "off"` in your config to reduce the auth surface.

### Session Grants

When a session record contains explicit `grants`, PinchTab enforces them in middleware and only allows routes covered by those grant groups. When a session has no explicit grants, PinchTab allows the normal non-admin automation routes by default but still blocks dashboard/admin endpoint families such as config, dashboard event streams, session management, profile management, instance management, and cache management.

The built-in grant groups are: `browse`, `network`, `media`, `cookies`, `clipboard`, `evaluate`, `storage`, `console`, `solve`, `tasks`, and `activity`.

The `cookies` grant only authorizes session access to cookie routes; cookie operations still require the server-level `security.allowCookies` gate.

That default is a convenience for trusted automation, not a sandbox. If you need hard isolation between agents or tenants, use separate PinchTab instances.

## CLI Usage

```bash
# Create a new session (prints the session token to stdout; use --json for a full JSON object)
pinchtab session create --agent-id agent-1
export PINCHTAB_SESSION=$(pinchtab session create --agent-id agent-1)

# CLI automatically uses session auth when PINCHTAB_SESSION is set
pinchtab snap

# Inspect the current session (uses PINCHTAB_SESSION)
pinchtab session info

# List all sessions on the server (server bearer token required)
pinchtab session list

# Revoke a session by id
pinchtab session revoke ses_abc123def456
```

## API Endpoints

See [endpoints.md](../endpoints.md) for full API reference.
