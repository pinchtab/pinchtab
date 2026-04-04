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

1. **Create** — via dashboard API: `POST /api/sessions`
2. **Use** — agent sends `Authorization: Session ses_...` with each request
3. **Rotate** — generate a new token, old one invalidated: `POST /api/sessions/{id}/rotate`
4. **Revoke** — permanently disable: `POST /api/sessions/{id}/revoke`

## Security

- Tokens are never logged or persisted in plaintext
- SHA-256 hash comparison using `crypto/subtle.ConstantTimeCompare`
- Idle timeout (default 30m) and max lifetime (default 24h)
- Sessions persisted to `agent-sessions.json` (atomic writes)
- Each session bound to a specific agentId for activity tracking

> **⚠️ Trusted environment only.** The session management API (`/api/sessions`) has no per-agent authorization scoping. Any caller authenticated with the server bearer token or a valid dashboard cookie can create, list, rotate, and revoke sessions for any agent. Do not expose these endpoints to untrusted networks or the public internet. If you need multi-tenant isolation, run separate PinchTab instances per tenant.
>
> In untrusted or shared environments where agent sessions are not needed, disable them entirely by setting `"enabled": false` or `"mode": "off"` in your config to reduce the auth surface.

## CLI Usage

```bash
# Set session token
export PINCHTAB_SESSION=ses_abc123...

# CLI automatically uses session auth
pinchtab snap

# Check session info
pinchtab session info
```

## API Endpoints

See [endpoints.md](../endpoints.md) for full API reference.
