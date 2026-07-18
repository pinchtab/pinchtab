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
├─ Need full page structure? → snap --full
├─ Need to debug visually? → screenshot  (use sparingly, large output)
└─ Need to run a JS check? → eval  (precise, zero visual overhead)
```

**Token cost ranking (cheapest → most expensive):**
1. `eval` — single value, no DOM traversal output
2. `find` — targeted element list only
3. `text` — readable text only
4. `snap` / `snap -i -c` — interactive elements, compact format
5. `snap --full` — full JSON tree
6. `screenshot` — image payload, highest token cost

**Rule of thumb:** Reach for `snap -i -c` as your default snapshot. Only escalate to `screenshot` when visual layout matters (canvas or complex CSS).

---

## Diff Snapshots for Follow-Up Reads

Use `--snap-diff` on action commands to get all refs plus change markers — in one call, not two.

```bash
pinchtab click e5 --snap-diff      # action + full refs with diff markers
pinchtab fill e3 "text" --snap-diff
```

Output format shows all valid refs with change markers:
```
# Page | URL | 57 nodes | +2 ~1 -0
e0:link "Home"
e5:button "Submit" [+]           # added
e12:textbox val="updated" [~]    # changed
# removed: e99
```

**When to use `--snap-diff`:**
- After clicks that update part of the UI (e.g. accordion opens, toast appears)
- After form fills that show inline validation
- During multi-step wizards where only one section changes
- Any interaction where you need to see the result — you get all refs plus diff info

**When NOT to use `--snap-diff`:**
- After `nav` to a new URL (diff would mark everything as added — use `--snap` instead)
- First snapshot of a session (no baseline exists — use `--snap`)

**Fallback:** If you already performed an action without `--snap-diff`, use `snap -d` separately.

---

## Faster Page Loads

Use `--block-images` on navigation for read-heavy tasks where images are not needed.

```bash
pinchtab nav <url> --block-images --snap
```

**Best for:** Form automation, data extraction, API-heavy SPAs, and scraping workflows where image content is not required.

---

## Iframe Shortcuts

Default `snap` (without `-i`) **flattens same-origin iframes** — nested iframe content appears as regular refs in the tree. Ref-based actions (`click`, `fill`, etc.) work **across iframe boundaries** without `frame` scope changes.

```
# snap already shows everything, including nested iframes:
# e0:heading "Outer page"
# e1:Iframe
# e2:heading "Level 2"
# e3:Iframe
# e4:heading "Level 3"
# e5:button "Deep button"    ← 3 levels deep, but clickable as e5

pinchtab click e5              # works cross-boundary — no frame hops needed
```

**When you DO need `frame`:** only for scoped `text` reads. `text` respects the current frame scope, so to read text inside a nested iframe you must hop. Chain the hops without intermediate snaps — use CSS selectors or iframe IDs from the initial `snap`:

```bash
# BAD: snap at each level (expensive)
pinchtab frame e1; pinchtab snap; pinchtab frame e1; pinchtab snap; pinchtab text

# GOOD: chain hops directly, read once
pinchtab frame '#level-2'
pinchtab frame '#level-3'
pinchtab text                  # now scoped to deepest frame
pinchtab frame main            # back to top
```

**Summary:** Use refs for **actions** (zero frame hops). Use `frame` chains for **text reads** (skip intermediate snaps).

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
2. Navigate to the login page and ask the user to complete sign-in. Do not request or enter credentials, one-time codes, or session tokens.
3. If using a profile, start or target that profile explicitly: `pinchtab instance start --profile <name>`

---

### Connection Refused
**Cause:** PinchTab server is not running or crashed.

**Recovery:**
```bash
pinchtab health          # confirm down
pinchtab server          # restart in the foreground, or use `pinchtab nav <url>` to auto-start for a new navigation
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

**Prevention:** Use `--snap-diff` on actions to get updated refs with each interaction. Never cache refs across navigations.

---

### Timeout on Navigation
**Cause:** Page load exceeded default timeout (usually 30s).

**Recovery:**
If the page consistently times out, consider `--block-images` to speed up load:
```bash
pinchtab nav <url> --block-images --snap
```

---

## General Efficiency Rules

- **Use `--snap-diff` on actions.** `click e5 --snap-diff` returns OK + only changed elements in one call — most token-efficient for multi-step flows.
- **Set a stable agent ID up front.** Use `pinchtab --agent-id <agent-id> ...`, `PINCHTAB_AGENT_ID`, or `X-Agent-Id` for raw HTTP calls so work stays attributable to the same agent.
- **Batch reads before writes.** Snap once, extract all refs, then act. Use `--snap-diff` on each action to see changes without re-fetching the full tree.
- **Use `text` for extraction tasks.** If you only need to read content (not interact), `text` is cheaper than `snap` + parsing.
- **Scope snapshots.** Use `snap -s <selector>` to target a specific section of the page when you know where the element is.
- **Prefer `fill` over `type` for framework forms.** Saves retries caused by React/Vue not detecting raw keystroke events.
- **Check health before long workflows.** Run `pinchtab health` at the start of a multi-step task to fail fast if the server is down.
- **Inspect network activity only with approval.** Captures can contain tokens and personal data; do not inspect bodies or export data unless the user explicitly requests it, and preserve redaction.
