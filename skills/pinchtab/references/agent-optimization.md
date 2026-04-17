# Agent Optimization Playbook

Practical guidance for running token-efficient, resilient PinchTab agent workflows.

---

## Cheapest-Path Decision Tree

Choose the lowest-cost tool that satisfies your goal:

```
Need to check page state?
├─ Know the element ref already? → skip snap, use click/type directly
├─ Need to find interactive elements? → snap -i -c  (cheapest)
├─ Need to read text/data only? → pinchtab text  (no tree overhead)
├─ Need to find a specific element? → pinchtab find "<text>"
├─ Need full page structure? → snap -c  (compact tree)
├─ Need to debug visually? → screenshot  (use sparingly, large output)
└─ Need to run a JS check? → eval  (precise, zero visual overhead)
```

**Token cost ranking (cheapest → most expensive):**
1. `eval` — single value, no DOM traversal output
2. `find` — targeted element list only
3. `text` — readable text only
4. `snap -i -c` — interactive elements, compact format
5. `snap -c` — full tree, compact
6. `snap -i` — interactive elements, verbose
7. `snap` — full tree, verbose
8. `screenshot` — image payload, highest token cost

**Rule of thumb:** Reach for `snap -i -c` as your default snapshot. Only escalate to `screenshot` when visual layout matters (CAPTCHA, canvas, complex CSS).

---

## Diff Snapshots for Follow-Up Reads

After an interaction, use `snap -d` to see only what changed — not the full tree.

```bash
pinchtab click e5      # trigger action
pinchtab snap -d       # only the delta — much smaller
```

**When to use `-d`:**
- After clicks that update part of the UI (e.g. accordion opens, toast appears)
- After form submissions that show inline validation
- During multi-step wizards where only one section changes

**When NOT to use `-d`:**
- After full page navigations (diff will be the entire new page)
- After `nav` — always take a fresh `snap` instead
- First snapshot of a session (no baseline exists)

---

## Lite Engine

Start PinchTab with `--engine lite` for minimal rendering overhead.

```bash
pinchtab start --engine lite
```

**Lite engine capabilities:**
- Faster page loads (no CSS animations, reduced JS execution)
- Lower memory footprint — useful for multi-tab fleet workflows
- Accessibility tree (`snap`) works fully
- `text`, `find`, `eval` all work as normal

**Lite engine limitations:**
- `screenshot` output may not reflect full visual styling
- Pages that depend on CSS transitions for state changes may behave differently
- Some canvas/WebGL content will not render
- Not suitable for visual regression testing

**Best for:** Form automation, data extraction, API-heavy SPAs, scraping workflows where visual fidelity is not required.

---

## Recovery Patterns

### 403 Forbidden
**Cause:** `eval` called without `security.allowEvaluate: true`, or a page blocked the request.

**Recovery:**
```bash
# Option 1: enable eval in config, restart server
# Option 2: switch to snap + find instead of eval
pinchtab find "target text"   # avoids eval entirely
```

---

### 401 Unauthorized
**Cause:** Session expired, auth cookie gone, or protected resource.

**Recovery:**
1. `pinchtab screenshot` — confirm login page is showing
2. Re-authenticate: `pinchtab nav <login-url>`, then fill credentials
3. If using a profile: `pinchtab profile use <name>` may restore the session

---

### Connection Refused
**Cause:** PinchTab server is not running or crashed.

**Recovery:**
```bash
pinchtab health          # confirm down
pinchtab start           # restart
pinchtab health          # confirm up before continuing
```

For fleet workflows: check `pinchtab instances` to confirm the right instance is running.

---

### Stale Element Refs
**Cause:** A `snap` was taken, then the page re-rendered (navigation, dynamic update). Old refs (`e5`, `e12`) are no longer valid.

**Symptoms:** Interaction returns "ref not found" or acts on the wrong element.

**Recovery:**
```bash
pinchtab snap -i -c      # fresh snapshot → new refs
# Now use the new refs from this response
```

**Prevention:** Never cache refs across navigations. Always re-snap after a page load or major DOM update.

---

### Bot Detection / CAPTCHA / Cloudflare
**Cause:** Target site detected automated behavior or uses a challenge gateway.

**Recovery options:**
1. Try `POST /solve` first — it auto-detects Cloudflare Turnstile and solves it:
   ```bash
   curl -X POST http://localhost:9867/solve \
     -H 'Content-Type: application/json' -d '{"maxAttempts": 3}'
   ```
2. If solve returns `solved: false`, try with more attempts or check `challengeType`
3. Slow down: add `pinchtab wait --ms 1500` between interactions
4. Switch to a profile with existing session cookies (CF cookies persist)
5. If unsupported CAPTCHA (not Cloudflare): report to user for manual intervention
6. Check `GET /solvers` to see which solver types are available
7. Verify `stealthLevel: "full"` is active — solvers depend on it. Check with `GET /stealth/status`

---

### Timeout on Navigation
**Cause:** Page load exceeded default timeout (usually 30s).

**Recovery:**
```bash
pinchtab nav <url> --timeout 90   # extend timeout
```

If the page consistently times out, consider `--block-images` to speed up load:
```bash
pinchtab nav <url> --block-images --timeout 60
```

---

## General Efficiency Rules

- **Set a stable agent ID up front.** Use `pinchtab --agent-id <agent-id> ...`, `PINCHTAB_AGENT_ID`, or `X-Agent-Id` for raw HTTP calls so work stays attributable to the same agent.
- **Batch reads before writes.** Snap once, extract all refs, then act. Avoid snap → act → snap → act loops when you can snap → act × N → snap once to verify.
- **Use `text` for extraction tasks.** If you only need to read content (not interact), `text` is cheaper than `snap` + parsing.
- **Scope snapshots.** Use `snap -s <selector>` to target a specific section of the page when you know where the element is.
- **Prefer `fill` over `type` for framework forms.** Saves retries caused by React/Vue not detecting raw keystroke events.
- **Check health before long workflows.** Run `pinchtab health` at the start of a multi-step task to fail fast if the server is down.
- **Export network traces after sessions.** `pinchtab network-export -o session.har` captures every request. For live capture: `pinchtab network-export --stream -o live.har`.
