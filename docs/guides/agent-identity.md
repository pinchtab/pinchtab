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

When a request has an agent ID but no agent session, PinchTab also keeps that agent's current tab separately from other agents. You can omit `tabId` in a multi-step flow without stealing another agent's current tab.

**When to use:** Multiple agents sharing one server where you want to see who did what, but don't need session management.

**Limitation:** No revocation, no idle tracking, no labels. The agent ID is self-declared — any caller can claim any identity.

## Agent Sessions

> **⚠️ Security notice:** Agent sessions are designed for **trusted, controlled environments** — local machines, private networks, CI, and setups where all agents are under your control. Do not expose the session management API (`/sessions`) to the public internet. Any authenticated caller (bearer token or dashboard cookie) can create, list, and inspect sessions for any agent. Session-authenticated callers are blocked from dashboard/admin endpoint families, but sessions are still not a multi-tenant isolation boundary.

Sessions are the full identity solution. Each session is a revocable, server-managed token tied to a specific agent ID. Sessions provide:

- **Labels** — human-readable names like "research task" or "daily scrape"
- **Activity grouping** — all requests within a session are grouped in the dashboard
- **Idle timeout** — sessions expire after 30 minutes of inactivity (configurable)
- **Max lifetime** — hard expiry after 24 hours (configurable)
- **Revocation** — kill a session without rotating the server token

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
curl -X POST http://localhost:9867/sessions \
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

Current-tab state is session-scoped. Two sessions for the same `agentId` keep separate current tabs, and session scope takes priority over any `X-Agent-Id` header.

### Manage Sessions

```bash
# List all sessions
curl http://localhost:9867/sessions \
  -H "Authorization: Bearer $PINCHTAB_TOKEN"

# Revoke
curl -X POST http://localhost:9867/sessions/ses_e6ac8132fe7e7016/revoke \
  -H "Authorization: Bearer $PINCHTAB_TOKEN"
```

### Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `sessions.agent.enabled` | `true` | Enable agent sessions |
| `sessions.agent.mode` | `preferred` | Auth mode. `preferred` serves agent sessions alongside the server token; `off` disables them exactly as `enabled: false` does. `required` is **not implemented** and is refused at config load — the bearer token and the dashboard cookie still authenticate, so the value cannot deliver the session-only auth it names |
| `sessions.agent.idleTimeoutSec` | `1800` (30m) | Session expires after this many seconds of inactivity |
| `sessions.agent.maxLifetimeSec` | `86400` (24h) | Hard session expiry |

If a session record carries explicit grants, those grants narrow which endpoint groups the session may call. Grants are set when the session is created — `pinchtab session create --agent-id <id> --grant browse`, or the `grants` field on `POST /sessions` — and are reported by `session list` and `session info`. If a session has no explicit grants, it can use the normal non-admin automation API by default, while dashboard/admin routes remain blocked. That default is meant for trusted automation only. A grant never widens: server-level capability gates still apply on top of it.

## Choosing the Right Level

| Scenario | Recommendation |
|----------|----------------|
| One agent, local only | Server token is enough |
| Multiple agents, want attribution | Add `--agent-id` or `PINCHTAB_AGENT_ID` |
| Production multi-agent, need revocation | Use agent sessions |
| Shared server, untrusted agents | Run separate PinchTab instances; sessions are not sufficient isolation |
