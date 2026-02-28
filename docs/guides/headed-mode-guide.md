# Headed Mode

Most browser automation assumes you're either fully automated or fully manual. Real workflows aren't like that.

Your agent can fill out a 50-field form in seconds, but it can't solve a CAPTCHA. It can navigate to the right page, but it can't approve a bank transfer. It can scrape a dashboard, but first someone needs to log in with 2FA.

That's what headed mode is for.

## The Problem with Headless-Only

Headless browsers are great until they aren't. The moment you hit any of these, you're stuck:

- **Login walls** — OAuth flows, 2FA, SMS codes
- **CAPTCHAs** — reCAPTCHA, hCaptcha, Cloudflare challenges
- **Visual verification** — "does this look right?" before submitting
- **Sensitive actions** — payments, deletions, things you want a human to approve

The usual workaround? Copy cookies from a real browser, hope they don't expire, pray the session doesn't get flagged. Fragile, annoying, doesn't scale.

## How Pinchtab Solves It

Pinchtab's headed mode gives you a real Chrome window that both humans and agents share. Same browser, same session, same cookies. No hacks.

The flow is simple:

1. **Human** opens a profile, logs in, handles 2FA
2. **Agent** takes over via HTTP API — same browser, same session
3. **Human** can watch, intervene, or take back control at any time

The agent doesn't need screenshots to "see" the page. It reads the accessibility tree — the same structure screen readers use. Structured, fast, cheap on tokens.

## Profiles: Persistent Identity

Each Chrome profile is a complete browser identity — cookies, localStorage, saved passwords, extensions, everything. Pinchtab stores them in `~/.pinchtab/profiles/`.

Profiles persist across restarts. Log in once to Gmail on Monday, and your agent can read email on Friday without re-authenticating.

```bash
# List profiles
curl http://localhost:9867/profiles
```

```json
[
  {
    "id": "a1b2c3d4e5f6",
    "name": "Work",
    "accountEmail": "you@company.com",
    "accountName": "Your Name",
    "useWhen": "Work email and social accounts"
  }
]
```

Every profile gets a stable 12-character hex ID derived from its name. Use IDs in automation — they're URL-safe and won't break if you have spaces or special characters in profile names.

## Start and Stop with One Call

The dashboard exposes two endpoints that make profile management trivial for agents:

```bash
# Start a profile (port auto-allocated)
curl -X POST http://localhost:9867/start/a1b2c3d4e5f6
```

```json
{
  "id": "Work-56490",
  "name": "Work",
  "port": "56490",
  "status": "starting",
  "url": "http://localhost:56490"
}
```

The response tells you which port the instance landed on. All your API calls go to that port:

```bash
PORT=56490

# Navigate
curl -X POST http://localhost:$PORT/navigate \
  -d '{"url": "https://mail.google.com"}'

# Read the inbox
curl "http://localhost:$PORT/snapshot?maxTokens=4000"

# Done — shut it down
curl -X POST http://localhost:9867/stop/a1b2c3d4e5f6
```

You can also pass a specific port or run headless if you want:

```bash
curl -X POST http://localhost:9867/start/a1b2c3d4e5f6 \
  -H 'Content-Type: application/json' \
  -d '{"port": "9868", "headless": true}'
```

## A Real Example: Reading Email

Here's what a full agent workflow looks like. The human already logged into Gmail through the profile once. Now the agent can check email any time:

```bash
# 1. Start the profile
INSTANCE=$(curl -s -X POST http://localhost:9867/start/a1b2c3d4e5f6)
PORT=$(echo $INSTANCE | jq -r .port)

# 2. Navigate to Gmail
curl -s -X POST http://localhost:$PORT/navigate \
  -d '{"url": "https://mail.google.com"}'

# 3. Read the inbox (accessibility tree, not screenshots)
curl -s "http://localhost:$PORT/snapshot?maxTokens=4000" | jq '.nodes[] | select(.role == "row") | .name' | head -5
```

Output:
```
"unread, GitHub, [org/repo] New pull request #42, 11:44 AM, ..."
"unread, Stripe, Your January invoice is ready, 11:26 AM, ..."
"unread, AWS, Your EC2 instance is running, 11:15 AM, ..."
```

No screenshots. No vision models. No token-heavy page dumps. Just structured data from the accessibility tree — the same way a screen reader would see it.

```bash
# 4. Clean up
curl -s -X POST http://localhost:9867/stop/a1b2c3d4e5f6
```

## When to Use Headed vs Headless

| Scenario | Mode | Why |
|---|---|---|
| First login to a service | **Headed** | Human handles 2FA/CAPTCHA |
| Daily email check | Either | Session persists, no login needed |
| Scraping public data | **Headless** | No human interaction needed |
| Filling forms with approval | **Headed** | Human reviews before submit |
| CI/CD automation | **Headless** | No display available |
| Debugging agent behavior | **Headed** | Watch what the agent does |

The key insight: you don't have to choose one mode forever. Start headed when you need human involvement, then switch to headless for routine automation. The profile carries the session either way.

## Dashboard: The Control Plane

Running `pinchtab dashboard` gives you a web UI at `http://localhost:9867/dashboard` for managing everything:

- Create and import Chrome profiles
- Launch instances (headed or headless) on any port
- Monitor running instances — status, tabs, logs
- Live view of what the agent is doing
- Stop instances gracefully

The dashboard itself doesn't run Chrome. It's a lightweight control plane that spawns and manages profile instances as separate processes.

For agents that prefer CLI over UI:

```bash
# Resolve a running profile to its URL
pinchtab connect "Work"
# → http://localhost:56490
```

## Multiple Profiles, Multiple Agents

Nothing stops you from running several profiles at once. Different accounts, different purposes:

```bash
# Work email
curl -X POST http://localhost:9867/start/a1b2c3d4e5f6

# Personal Twitter
curl -X POST http://localhost:9867/start/a1b2c3d4e5f6

# Research browser (no login, disposable)
curl -X POST http://localhost:9867/start/f6e5d4c3b2a1
```

Each gets its own port, its own Chrome process, its own isolated session. Agents can work across all of them simultaneously.

---

Headed mode isn't about choosing between humans and agents. It's about letting them work together — each doing what they're best at. Humans handle the messy, contextual, trust-requiring parts. Agents handle the repetitive, fast, scale-requiring parts.

## Monitoring Agents Remotely

Here's where it gets interesting. Your agents are running on a server — a Mac Mini under your desk, a VPS, a home lab box. You're on your laptop, maybe on the couch, maybe in a coffee shop. You want to know what your agents are doing right now.

The dashboard gives you that. Open it in your browser and you get a real-time view of every agent, every profile, every action.

### Setting Up Remote Access

By default, Pinchtab only listens on `127.0.0.1` — locked to the machine. To access the dashboard from another device on your network, you need two things: open the bind address and set an auth token.

```bash
BRIDGE_BIND=0.0.0.0 BRIDGE_TOKEN=your-secret-token pinchtab dashboard
```

That's it. Now open `http://<server-ip>:9867/dashboard` from your laptop and you'll see the full dashboard.

Every API call needs the token too:

```bash
curl http://192.168.1.100:9867/profiles \
  -H "Authorization: Bearer your-secret-token"
```

Without the token, every request gets a `401`. No exceptions — health check, profiles, everything.

### What You See

The dashboard has three views:

**Profiles** — your Chrome profiles, their status (running/stopped), account info. Click Details on any profile to get three tabs:

- **Profile** — metadata, status, path, account info
- **Live** — real-time screencast of what the browser is showing right now. You're literally watching your agent browse. <!-- screenshot: live-tab.png -->
- **Logs** — open tabs, connected agents, activity stats, instance logs

<div align="center" style="padding: 12px 0;">
  <img src="../assets/live-view.png" width="400" alt="Pinchtab live view" style="padding: 8px;" />
</div>

**Agents** — every agent that's made an API call, across all running profiles. You see their ID, which profile they're using, their last action, and when they were last active. The Activity Feed shows a real-time stream of every navigate, snapshot, and action — filterable by type. 

<div align="center" style="padding: 12px 0;">
  <img src="../assets/agents-feed.png" width="400" alt="Pinchtab agents feed" style="padding: 8px;" />
</div>

**Settings** — screencast quality, stealth level, browser options.

### The Activity Feed

The Activity Feed is the heartbeat of your agent fleet. Every action from every agent across every profile streams in real-time:

```
mario → POST /navigate (Work) — 23ms
mario → GET /snapshot (Work) — 145ms  
scraper → POST /navigate (Research) — 31ms
scraper → GET /text (Research) — 89ms
```

Filter by type — just navigations, just snapshots, just actions — to focus on what matters.

This works because the dashboard subscribes to each running child instance's event stream and relays everything through a single SSE connection to your browser. You get one unified view even when agents are spread across multiple profiles on different ports.

### Agent Identification

For agents to show up with a name instead of "anonymous", they just need to pass a header:

```bash
curl -X POST http://localhost:9868/navigate \
  -H "X-Agent-Id: mario" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

That's all it takes. The `X-Agent-Id` header tags every request, and the dashboard tracks it automatically. No registration, no config — just a header.

### Watching Live

The Live tab in the profile details is a real-time screencast. The dashboard streams JPEG frames from Chrome's DevTools protocol directly to your browser. You see exactly what the agent sees — every page load, every click, every form fill.

This is useful for:

- **Debugging** — why did the agent click the wrong button? Watch it happen.
- **Trust** — your agent is buying something? Watch it fill in the details before it submits.
- **Fun** — there's something oddly satisfying about watching an AI browse the web.

The screencast is lightweight — configurable FPS (1-15), quality (10-80%), and max width. Default settings use minimal bandwidth.

### A Typical Remote Monitoring Setup

On your server:

```bash
# Start the dashboard, open to network, locked with token
BRIDGE_BIND=0.0.0.0 \
BRIDGE_TOKEN=my-secret-token \
pinchtab dashboard &

# Your agents start profiles and work as usual
# They just pass X-Agent-Id headers so you can identify them
```

On your laptop, phone, or tablet:

1. Open `http://<server-ip>:9867/dashboard`
2. See all profiles, their status, running agents
3. Click into any profile → Live tab to watch the screencast
4. Switch to Agents view to see the real-time activity feed

No SSH tunnels. No VPN. Just a browser and a token.

## Security

Pinchtab binds to `127.0.0.1` by default — only accessible from the machine it's running on. This is intentional. Your agent runs on the same machine, so it just works. Nobody on your network can reach the dashboard or the API.

If you need remote access, set `BRIDGE_BIND=0.0.0.0` and **always** pair it with `BRIDGE_TOKEN`:

```bash
BRIDGE_BIND=0.0.0.0 BRIDGE_TOKEN=my-secret-token pinchtab dashboard
```

Without `BRIDGE_TOKEN`, every request is rejected with `401` — including the dashboard UI itself. There's no "public mode". If you open the bind address, you must set a token. This is by design.

For production setups, consider:

- Running behind a reverse proxy (nginx, Caddy) with HTTPS
- Using a strong, random token (`openssl rand -hex 32`)
- Restricting network access with firewall rules

That's the whole point. 🦀
