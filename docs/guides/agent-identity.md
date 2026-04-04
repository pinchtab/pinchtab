# Agent Identity

PinchTab offers three levels of agent identification, from simple to fully managed. Pick the one that fits your setup.

## Server Token

Every PinchTab server has a bearer token configured in `server.token`. This is the baseline authentication method — it proves the caller is authorized to use the server, but says nothing about *which* agent is making the request.

```bash
pinchtab --token "your-server-token" nav https://example.com
```

Or via environment variable:

```bash
export PINCHTAB_TOKEN=your-server-token
pinchtab nav https://example.com
```

**When to use:** Single-agent setups, quick scripting, or when you don't need per-agent tracking.

**Limitation:** All requests look the same in the activity feed — no way to tell which agent did what.

## Agent ID

Adding an agent ID tags every request with a name. This shows up in the activity feed and the dashboard's Agents page. The server still authenticates via the bearer token, but now each request carries an identity.

```bash
pinchtab --agent-id bosch nav https://example.com
```

Or via environment variable:

```bash
export PINCHTAB_AGENT_ID=bosch
pinchtab nav https://example.com
```

The `X-Agent-Id` header is sent with every request. No server-side setup required — any string works.

**When to use:** Multiple agents sharing one server where you want to see who did what, but don't need session management.

**Limitation:** No revocation, no idle tracking, no labels. The agent ID is self-declared — any caller can claim any identity.

## Agent Sessions

> **⚠️ Security notice:** Agent sessions are designed for **trusted environments** — local machines, private networks, or setups where all agents are under your control. Do not expose the session management API (`/api/sessions`) to the public internet. Any authenticated caller (bearer token or dashboard cookie) can create, list, rotate, and revoke sessions for any agent. There is no per-agent authorization scoping — treat session management endpoints like admin APIs.

Sessions are the full identity solution. Each session is a revocable, server-managed token tied to a specific agent ID. Sessions provide:

- **Labels** — human-readable names like "research task" or "daily scrape"
- **Activity grouping** — all requests within a session are grouped in the dashboard
- **Idle timeout** — sessions expire after 12 hours of inactivity (configurable)
- **Max lifetime** — hard expiry after 24 hours (configurable)
- **Revocation** — kill a session without rotating the server token
- **Rotation** — generate a new token for an existing session

### Enable Sessions

Add to your `config.json`:

```json
{
  "sessions": {
    "agent": {
      "enabled": true,
      "mode": "preferred"
    }
  }
}
```

Modes:

| Mode | Behavior |
|------|----------|
| `off` | Agent sessions disabled |
| `preferred` | Both bearer and session auth accepted (default when enabled) |
| `required` | Only session auth accepted for agents |

### Create a Session

```bash
curl -X POST http://localhost:9867/api/sessions \
  -H "Authorization: Bearer $PINCHTAB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"agentId": "bosch", "label": "research task"}'
```

Response:

```json
{
  "id": "ses_e6ac8132fe7e7016",
  "agentId": "bosch",
  "label": "research task",
  "sessionToken": "ses_1138f72e77f23c49...",
  "status": "active"
}
```

The `sessionToken` is returned exactly once. Store it — PinchTab only persists the hash.

### Use a Session

```bash
export PINCHTAB_SESSION=ses_1138f72e77f23c49...
pinchtab nav https://example.com
pinchtab snap -i -c
pinchtab click e5
```

Or pass the header directly:

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Authorization: Session ses_1138f72e77f23c49..." \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

No need to set `--agent-id` — the session carries the agent identity.

### Manage Sessions

```bash
# List all sessions
curl http://localhost:9867/api/sessions \
  -H "Authorization: Bearer $PINCHTAB_TOKEN"

# Rotate token (old token invalidated, new one returned)
curl -X POST http://localhost:9867/api/sessions/ses_e6ac8132fe7e7016/rotate \
  -H "Authorization: Bearer $PINCHTAB_TOKEN"

# Revoke
curl -X POST http://localhost:9867/api/sessions/ses_e6ac8132fe7e7016/revoke \
  -H "Authorization: Bearer $PINCHTAB_TOKEN"
```

### Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `sessions.agent.enabled` | `false` | Enable agent sessions |
| `sessions.agent.mode` | `preferred` | Auth mode: `off`, `preferred`, `required` |
| `sessions.agent.idleTimeoutSec` | `43200` (12h) | Session expires after this many seconds of inactivity |
| `sessions.agent.maxLifetimeSec` | `86400` (24h) | Hard session expiry |

## Choosing the Right Level

| Scenario | Recommendation |
|----------|----------------|
| One agent, local only | Server token is enough |
| Multiple agents, want attribution | Add `--agent-id` or `PINCHTAB_AGENT_ID` |
| Production multi-agent, need revocation | Use agent sessions |
| Shared server, untrusted agents | Use sessions with `mode: "required"` |
