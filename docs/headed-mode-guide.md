# Headed Mode: When Your Agent Needs a Human in the Loop

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
    "id": "278be873adeb",
    "name": "Pinchtab org",
    "accountEmail": "admin@gi-ago.com",
    "accountName": "Luigi Agosti",
    "useWhen": "For gmail and x.com"
  }
]
```

Every profile gets a stable 12-character hex ID derived from its name. Use IDs in automation — they're URL-safe and won't break if you have spaces or special characters in profile names.

## Start and Stop with One Call

The dashboard exposes two endpoints that make profile management trivial for agents:

```bash
# Start a profile (port auto-allocated)
curl -X POST http://localhost:9867/start/278be873adeb
```

```json
{
  "id": "Pinchtab org-56490",
  "name": "Pinchtab org",
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
curl -X POST http://localhost:9867/stop/278be873adeb
```

You can also pass a specific port or run headless if you want:

```bash
curl -X POST http://localhost:9867/start/278be873adeb \
  -H 'Content-Type: application/json' \
  -d '{"port": "9868", "headless": true}'
```

## A Real Example: Reading Email

Here's what a full agent workflow looks like. The human already logged into Gmail through the profile once. Now the agent can check email any time:

```bash
# 1. Start the profile
INSTANCE=$(curl -s -X POST http://localhost:9867/start/278be873adeb)
PORT=$(echo $INSTANCE | jq -r .port)

# 2. Navigate to Gmail
curl -s -X POST http://localhost:$PORT/navigate \
  -d '{"url": "https://mail.google.com"}'

# 3. Read the inbox (accessibility tree, not screenshots)
curl -s "http://localhost:$PORT/snapshot?maxTokens=4000" | jq '.nodes[] | select(.role == "row") | .name' | head -5
```

Output:
```
"unread, X, New login to X from ChromeDesktop on Mac, 11:44 AM, ..."
"unread, Amazon Route 53, Registration of narrowexperts.com succeeded, 11:26 AM, ..."
"unread, Amazon Route 53, Registration of expertsytem.com succeeded, 11:26 AM, ..."
```

No screenshots. No vision models. No token-heavy page dumps. Just structured data from the accessibility tree — the same way a screen reader would see it.

```bash
# 4. Clean up
curl -s -X POST http://localhost:9867/stop/278be873adeb
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
pinchtab connect "Pinchtab org"
# → http://localhost:56490
```

## Multiple Profiles, Multiple Agents

Nothing stops you from running several profiles at once. Different accounts, different purposes:

```bash
# Work email
curl -X POST http://localhost:9867/start/278be873adeb

# Personal Twitter
curl -X POST http://localhost:9867/start/a1b2c3d4e5f6

# Research browser (no login, disposable)
curl -X POST http://localhost:9867/start/f6e5d4c3b2a1
```

Each gets its own port, its own Chrome process, its own isolated session. Agents can work across all of them simultaneously.

---

Headed mode isn't about choosing between humans and agents. It's about letting them work together — each doing what they're best at. Humans handle the messy, contextual, trust-requiring parts. Agents handle the repetitive, fast, scale-requiring parts.

That's the whole point. 🦀
