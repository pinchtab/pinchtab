---
name: pinchtab
description: "Use this skill when a task needs browser automation through PinchTab: open a website, inspect interactive elements, click through flows, fill out forms, scrape page text, log into sites with a persistent profile, export screenshots or PDFs, manage multiple browser instances, or fall back to the HTTP API when the CLI is unavailable. Prefer this skill for token-efficient browser work driven by stable accessibility refs such as `e5` and `e12`."
metadata:
  openclaw:
    requires:
      bins:
        - pinchtab
      anyBins:
        - google-chrome
        - google-chrome-stable
        - chromium
        - chromium-browser
    homepage: https://github.com/pinchtab/pinchtab
    install:
      - kind: brew
        formula: pinchtab/tap/pinchtab
        bins: [pinchtab]
      - kind: go
        package: github.com/pinchtab/pinchtab/cmd/pinchtab@latest
        bins: [pinchtab]
---

# Browser Automation with PinchTab

PinchTab gives agents a browser they can drive through stable accessibility refs, low-token text extraction, and persistent profiles or instances. Treat it as a CLI-first browser skill; use the HTTP API only when the CLI is unavailable or you need profile-management routes that do not exist in the CLI yet.

Preferred tool surface:

- Use `pinchtab` CLI commands first.
- Use `curl` for profile-management routes or non-shell/API fallback flows.
- Use `jq` only when you need structured parsing from JSON responses.

## Agent Identity And Attribution

When multiple agents share one PinchTab server, always give each agent a stable ID.

- CLI flows: prefer `pinchtab --agent-id <agent-id> ...`
- long-running shells: set `PINCHTAB_AGENT_ID=<agent-id>`
- raw HTTP flows: send `X-Agent-Id: <agent-id>` on requests that should be attributed to that agent

That identity is recorded as `agentId` in activity events and powers:

- the dashboard Agents view
- `GET /api/activity?agentId=<agent-id>`
- scheduler task attribution when work is dispatched on behalf of an agent

If you are switching between unrelated browser tasks, do not reuse the same agent ID unless you intentionally want one combined activity trail.

## Safety Defaults

- Default to `http://localhost` targets. Only use a remote PinchTab server when the user explicitly provides it and, if needed, a token.
- Prefer read-only operations first: `text`, `snap -i -c`, `snap -d`, `find`, `click`, `fill`, `type`, `press`, `select`, `hover`, `scroll`.
- Do not evaluate arbitrary JavaScript unless a simpler PinchTab command cannot answer the question.
- Do not upload local files unless the user explicitly names the file to upload and the destination flow requires it.
- Do not save screenshots, PDFs, or downloads to arbitrary paths. Use a user-specified path or a safe temporary/workspace path.
- Never use PinchTab to inspect unrelated local files, browser secrets, stored credentials, or system configuration outside the task.

## Core Workflow

Every PinchTab automation follows this pattern:

1. Ensure the correct server, profile, or instance is available for the task.
2. Navigate with `pinchtab nav <url>` or `pinchtab instance navigate <instance-id> <url>`.
3. Observe with `pinchtab snap -i -c`, `pinchtab snap --text`, or `pinchtab text`, then collect the current refs such as `e5`.
4. Interact with those fresh refs using `click`, `fill`, `type`, `press`, `select`, `hover`, or `scroll`.
5. Re-snapshot or re-read text after any navigation, submit, modal open, accordion expand, or other DOM-changing action.

Rules:

- Never act on stale refs after the page changes.
- Default to `pinchtab text` when you need content, not layout.
- Default to `pinchtab snap -i -c` when you need actionable elements.
- Use screenshots only for visual verification, UI diffs, or debugging.
- Start multi-site or parallel work by choosing the right instance or profile first.

## Selectors

PinchTab uses a unified selector system. Any command that targets an element accepts these formats:

| Selector | Example | Resolves via |
|---|---|---|
| Ref | `e5` | Snapshot cache (fastest) |
| CSS | `#login`, `.btn`, `[data-testid="x"]` | `document.querySelector` |
| XPath | `xpath://button[@id="submit"]` | CDP search |
| Text | `text:Sign In` | Visible text match |
| Semantic | `find:login button` | Natural language query via `/find` |

Auto-detection: bare `e5` → ref, `#id` / `.class` / `[attr]` → CSS, `//path` → XPath. Use explicit prefixes (`css:`, `xpath:`, `text:`, `find:`) when auto-detection is ambiguous.

```bash
pinchtab click e5                        # ref
pinchtab click "#submit"                 # CSS (auto-detected)
pinchtab click "text:Sign In"            # text match
pinchtab click "xpath://button[@type]"   # XPath
pinchtab fill "#email" "user@test.com"   # CSS
pinchtab fill e3 "user@test.com"         # ref
```

The same syntax works in the HTTP API via the `selector` field:

```json
{"kind": "click", "selector": "text:Sign In"}
{"kind": "fill", "selector": "#email", "text": "user@test.com"}
{"kind": "click", "selector": "e5"}
```

Legacy `ref` field is still accepted for backward compatibility.

## Command Chaining

Use `&&` only when you do not need to inspect intermediate output before deciding the next step.

Good:

```bash
pinchtab nav https://pinchtab.com && pinchtab snap -i -c
pinchtab click --wait-nav e5 && pinchtab snap -i -c
pinchtab nav https://pinchtab.com --block-images && pinchtab text
```

Run commands separately when you must read the snapshot output first:

```bash
pinchtab nav https://pinchtab.com
pinchtab snap -i -c
# Read refs, choose the correct e#
pinchtab click e7
pinchtab snap -i -c
```

## Challenge Solving

PinchTab includes a pluggable solver framework that auto-detects and resolves browser challenges (Cloudflare Turnstile, CAPTCHAs, interstitials). Use this **after navigation** when the page shows a challenge instead of the expected content.

**Important:** Solvers work best with `stealthLevel: "full"` in the PinchTab config (or `instanceDefaults.stealthLevel: "full"`). Full stealth mode patches CDP detection vectors, rotates fingerprints, and masks automation signals — all of which challenge providers like Cloudflare check before and after the checkbox click. Without full stealth, the solver may click correctly but the challenge can still fail fingerprint verification.

```bash
# Auto-detect and solve any challenge on the current page
curl -X POST http://localhost:9867/solve \
  -H 'Content-Type: application/json' \
  -d '{"maxAttempts": 3, "timeout": 30000}'

# Use a specific solver
curl -X POST http://localhost:9867/solve/cloudflare \
  -H 'Content-Type: application/json' \
  -d '{"maxAttempts": 3}'

# Tab-scoped solve
curl -X POST http://localhost:9867/tabs/TAB_ID/solve \
  -H 'Content-Type: application/json' \
  -d '{}'

# List available solvers
curl http://localhost:9867/solvers
```

**When to use solve:**

- Page title is "Just a moment..." or similar challenge indicator
- `pinchtab text` returns empty or challenge-page text after navigation
- A Cloudflare Turnstile widget blocks the target content

**Workflow pattern:**

```bash
pinchtab nav https://protected-site.com
pinchtab text                    # Check if page loaded or shows challenge
# If challenge detected:
curl -X POST http://localhost:9867/solve \
  -H 'Content-Type: application/json' -d '{}'
pinchtab text                    # Verify: should now show real page content
```

**Response fields:** `solver` (which solver handled it), `solved` (bool), `challengeType` (e.g. "managed"), `attempts`, `title` (final page title).

The auto-detect mode (`POST /solve` without specifying a solver) tries each registered solver in order and returns immediately with `solved: true, attempts: 0` if no challenge is present. This makes it safe to call speculatively after any navigation.

## Handling Authentication and State

Pick one of these five patterns before you start interacting with the site.

### 1. One-off public browsing

Use a temporary instance for public pages, scraping, or tasks that do not need login persistence.

```bash
pinchtab instance start
pinchtab instances
# Point CLI commands at the instance port you want to use.
pinchtab --server http://localhost:9868 nav https://pinchtab.com
pinchtab --server http://localhost:9868 text
```

### 2. Reuse an existing named profile

Use this for recurring tasks against the same authenticated site.

```bash
pinchtab profiles
pinchtab instance start --profile work --mode headed
pinchtab --server http://localhost:9868 nav https://mail.google.com
```

If the login is already stored in that profile, you can switch to headless later:

```bash
pinchtab instance stop inst_ea2e747f
pinchtab instance start --profile work --mode headless
```

### 3. Create a dedicated auth profile over HTTP

Use this when you need a durable profile and it does not exist yet.

```bash
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"billing","description":"Billing portal automation","useWhen":"Use for billing tasks"}'

curl -X POST http://localhost:9867/profiles/billing/start \
  -H "Content-Type: application/json" \
  -d '{"headless":false}'
```

Then target the returned port with `--server`.

### 4. Human-assisted headed login, then agent reuse

Use this for CAPTCHA, MFA, or first-time setup.

```bash
pinchtab instance start --profile work --mode headed
# Human completes login in the visible Chrome window.
pinchtab --server http://localhost:9868 nav https://app.example.com/dashboard
pinchtab --server http://localhost:9868 snap -i -c
```

Once the session is stored, reuse the same profile for later tasks.

### 5. Remote or non-shell agent with tokenized HTTP API

Use this when the agent cannot call the CLI directly.

```bash
curl http://localhost:9867/health
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"work"}'
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"work","mode":"headless"}'
curl -X POST http://localhost:9868/action \
  -H "X-Agent-Id: agent-main" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"e5"}'
```

If the server is exposed beyond localhost, require a token and use a dedicated automation profile. See [TRUST.md](./TRUST.md).

**Agent sessions**: Instead of sharing the server bearer token, each agent can get its own revocable session token. Set `PINCHTAB_SESSION=ses_...` or send `Authorization: Session ses_...`. Create via `POST /api/sessions` with `{"agentId":"...", "label":"..."}`. Sessions have idle timeout (default 12h) and max lifetime (default 24h). Manage with rotate (`POST /api/sessions/{id}/rotate`) and revoke (`POST /api/sessions/{id}/revoke`).

## Essential Commands

### Server and targeting

```bash
pinchtab server                                     # Start server foreground
pinchtab daemon install                             # Install as system service
pinchtab health                                     # Check server status
pinchtab instances                                  # List running instances
pinchtab profiles                                   # List available profiles
pinchtab --server http://localhost:9868 snap -i -c  # Target specific instance
```

### Navigation and tabs

```bash
pinchtab nav <url>
pinchtab nav <url> --new-tab
pinchtab nav <url> --tab <tab-id>
pinchtab nav <url> --block-images
pinchtab nav <url> --block-ads
pinchtab back                                       # Navigate back in history
pinchtab forward                                    # Navigate forward
pinchtab reload                                     # Reload current page
pinchtab tab                                        # List tabs or focus by ID
pinchtab tab new <url>
pinchtab tab close <tab-id>
pinchtab instance navigate <instance-id> <url>
```

### Observation

```bash
pinchtab snap
pinchtab snap -i                                    # Interactive elements only
pinchtab snap -i -c                                 # Interactive + compact
pinchtab snap -d                                    # Diff from previous snapshot
pinchtab snap --selector <css>                      # Scope to CSS selector
pinchtab snap --max-tokens <n>                      # Token budget limit
pinchtab snap --text                                # Text output format
pinchtab text                                       # Page text content
pinchtab text --raw                                 # Raw text extraction
pinchtab find <query>                               # Semantic element search
pinchtab find --ref-only <query>                    # Return refs only
```

Guidance:

- `snap -i -c` is the default for finding actionable refs.
- `snap -d` is the default follow-up snapshot for multi-step flows.
- `text` is the default for reading articles, dashboards, reports, or confirmation messages.
- `find --ref-only` is useful when the page is large and you already know the semantic target.

### Interaction

All interaction commands accept unified selectors (refs, CSS, XPath, text, semantic). See the Selectors section above.

```bash
pinchtab click <selector>                           # Click element
pinchtab click --wait-nav <selector>                # Click and wait for navigation
pinchtab click --x 100 --y 200                      # Click by coordinates
pinchtab dblclick <selector>                        # Double-click element
pinchtab type <selector> <text>                     # Type with keystrokes
pinchtab fill <selector> <text>                     # Set value directly
pinchtab press <key>                                # Press key (Enter, Tab, Escape...)
pinchtab hover <selector>                           # Hover element
pinchtab select <selector> <value>                  # Select dropdown option
pinchtab scroll <selector|pixels>                   # Scroll element or page
```

Rules:

- Prefer `fill` for deterministic form entry.
- Prefer `type` only when the site depends on keystroke events.
- Prefer `click --wait-nav` when a click is expected to navigate.
- Re-snapshot immediately after `click`, `press Enter`, `select`, or `scroll` if the UI can change.

### Export, debug, and verification

```bash
pinchtab screenshot
pinchtab screenshot -o /tmp/pinchtab-page.png       # Format driven by extension
pinchtab screenshot -q 60                            # JPEG quality
pinchtab pdf
pinchtab pdf -o /tmp/pinchtab-report.pdf
pinchtab pdf --landscape
```

### Advanced operations: explicit opt-in only

Use these only when the task explicitly requires them and safer commands are insufficient.

```bash
pinchtab eval "document.title"
pinchtab download <url> -o /tmp/pinchtab-download.bin
pinchtab upload /absolute/path/provided-by-user.ext -s <css>
```

Rules:

- `eval` is for narrow, read-only DOM inspection unless the user explicitly asks for a page mutation.
- `download` should prefer a safe temporary or workspace path over an arbitrary filesystem location.
- `upload` requires a file path the user explicitly provided or clearly approved for the task.

### HTTP API fallback

```bash
curl -X POST http://localhost:9868/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

curl "http://localhost:9868/snapshot?filter=interactive&format=compact"

curl -X POST http://localhost:9868/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","selector":"e3","text":"ada@example.com"}'

curl http://localhost:9868/text

## Instance-scoped solve (instance port, not server port)
curl -X POST http://localhost:9868/solve \
  -H "Content-Type: application/json" \
  -d '{"maxAttempts": 3}'

curl http://localhost:9868/solvers
```

Use the API when:

- the agent cannot shell out,
- profile creation or mutation is required,
- or you need explicit instance- and tab-scoped routes.

## Common Patterns

### Open a page and inspect actions

```bash
pinchtab nav https://pinchtab.com && pinchtab snap -i -c
```

### Fill and submit a form

```bash
pinchtab nav https://example.com/login
pinchtab snap -i -c
pinchtab fill e3 "user@example.com"
pinchtab fill e4 "correct horse battery staple"
pinchtab click --wait-nav e5
pinchtab text
```

### Search, then extract the result page cheaply

```bash
pinchtab nav https://example.com/search
pinchtab snap -i -c
pinchtab fill e2 "quarterly report"
pinchtab click e3  # Click the Search button
pinchtab text
```

**Form submission rules:**
- ❌ **NEVER use `press Enter` on regular form inputs.** It does NOT submit standard HTML forms.
- ✅ **ALWAYS click the submit button** to trigger form submission handlers.
- **Why?** HTML5 forms only auto-submit on Enter if they have explicit JavaScript `onkeypress` or `onkeyup` handlers. The `<form onsubmit>` handler only fires when you click the button, not on Enter.

**Real-world example from benchmark:**
```bash
# WRONG: This fails
pinchtab nav http://fixtures/wiki.html
pinchtab snap -i -c
pinchtab fill "#wiki-search-input" "go"
pinchtab press Enter  # ❌ Does NOT submit the form

# RIGHT: This works
pinchtab nav http://fixtures/wiki.html
pinchtab snap -i -c
pinchtab fill "#wiki-search-input" "go"
pinchtab click "#wiki-search-btn"  # ✅ Triggers onsubmit handler
pinchtab text  # Now on the results page
```

**Always follow this pattern:**
```bash
# Template: fill, then click
pinchtab fill "<selector>" "value"
pinchtab click "<button-selector>"  # Always click, never press Enter
pinchtab text  # Verify the form was submitted
```

### Use diff snapshots in a multi-step flow

```bash
pinchtab nav https://example.com/checkout
pinchtab snap -i -c
pinchtab click e8
pinchtab snap -d -i -c
```

### Target elements without a snapshot

When you know the page structure, skip the snapshot and use CSS or text selectors directly:

```bash
pinchtab click "text:Accept Cookies"
pinchtab fill "#search" "quarterly report"
pinchtab click "xpath://button[@type='submit']"
```

### Navigate through a Cloudflare-protected site

```bash
pinchtab nav https://protected-site.com
# Page may show CF challenge ("Just a moment...")
curl -X POST http://localhost:9867/solve \
  -H 'Content-Type: application/json' -d '{"maxAttempts": 3}'
# Now the real page is loaded — proceed normally
pinchtab snap -i -c
pinchtab text
```

### Bootstrap an authenticated profile

```bash
pinchtab profiles
pinchtab instance start --profile work --mode headed
# Human signs in once.
pinchtab --server http://localhost:9868 text
```

### Run separate instances for separate sites

```bash
pinchtab instance start --profile work --mode headless
pinchtab instance start --profile staging --mode headless
pinchtab instances
```

Then point each command stream at its own port using `--server`.

## Security and Token Economy

- Use a dedicated automation profile, not a daily browsing profile.
- If PinchTab is reachable off-machine, require a token and bind conservatively.
- Prefer `text`, `snap -i -c`, and `snap -d` before screenshots, PDFs, eval, downloads, or uploads.
- Use `--block-images` for read-heavy tasks that do not need visual assets.
- Stop or isolate instances when switching between unrelated accounts or environments.

## Diffing and Verification

- Use `pinchtab snap -d` after each state-changing action in long workflows.
- Use `pinchtab text` to confirm success messages, table updates, or navigation outcomes.
- Use `pinchtab screenshot` only when visual regressions, CAPTCHA, or layout-specific confirmation matters.
- If a ref disappears after a change, treat that as expected and fetch fresh refs instead of retrying the stale one.

## Privacy and Security

PinchTab is a fully open-source, local-only browser automation tool:

- **Runs on localhost only.** The server binds to `127.0.0.1` by default. No external network calls are made by PinchTab itself.
- **No telemetry or analytics.** The binary makes zero outbound connections.
- **Single Go binary (~16 MB).** Fully verifiable — anyone can build from source at [github.com/pinchtab/pinchtab](https://github.com/pinchtab/pinchtab).
- **Local Chrome profiles.** Persistent profiles store cookies and sessions on your machine only. This enables agents to reuse authenticated sessions without re-entering credentials, similar to how a human reuses their browser profile.
- **Token-efficient by design.** Uses the accessibility tree (structured text) instead of screenshots, keeping agent context windows small. Comparable to Playwright but purpose-built for AI agents.
- **Multi-instance isolation.** Each browser instance runs in its own profile directory with tab-level locking for safe multi-agent use.

## References

- Full API: [api.md](./references/api.md)
- Minimal env vars: [env.md](./references/env.md)
- Agent optimization: [agent-optimization.md](./references/agent-optimization.md)
- Profiles: [profiles.md](./references/profiles.md)
- MCP: [mcp.md](./references/mcp.md)
- Security model: [TRUST.md](./TRUST.md)

## Content Extraction: text vs snapshot

Choose the right extraction method:

| Use Case | Recommended | Why |
|----------|-------------|-----|
| Article body, paragraphs | `text` | Clean prose extraction |
| Prices, numbers in cards | `snapshot` | Text strips structured data |
| Form field values | `snapshot` | See current input values |
| Verify element exists | `snapshot` with selector | Text won't show headings |
| JS-rendered content | `snapshot` after wait | Text may miss dynamic content |

**Common pitfall**: `/text` extracts readable prose but strips headings, prices, and structured UI elements. If you need to verify a heading, price, or button label, use `/snapshot` instead.

```bash
# Wrong: looking for "$149.99" in text output
pinchtab text | grep "149.99"  # May fail - prices often stripped

# Right: snapshot includes all visible text
pinchtab snap -c | grep "149.99"  # Works
```

## Fixture Selector Quick Reference

| Page | Key selectors |
|------|---------------|
| ecommerce | `.add-to-cart`, `#checkout-btn`, `.price` (use `snapshot` not `-c` for price values) |
| search | `#search-input`, `#search-btn` (click button — never press Enter) |
| form | `#fullname`, `#email`, `#phone`, `#country`, `#subject`, `#message`, `#submit-btn` |
| wiki | `#wiki-search-input`, `#wiki-search-btn` |
| spa | `#new-task-input`, `#priority-select`, `#add-task-btn` |
| login | `#username`, `#password`, `#login-btn`, `#logout-btn` |
| dashboard | `#settings-btn`, `#theme-select`, `#modal-save` |
