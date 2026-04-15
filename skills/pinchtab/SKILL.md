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

If a page shows a challenge instead of content (e.g., "Just a moment..."), call `POST /solve` with `{"maxAttempts": 3}` to auto-detect and resolve it. Use `POST /tabs/TAB_ID/solve` for tab-scoped. Works best with `stealthLevel: "full"` in config. Safe to call speculatively — returns immediately if no challenge is present. See [api.md](./references/api.md) for full solver options.

## Handling Authentication and State

Pick a pattern before interacting with the site:

1. **One-off browsing**: `pinchtab instance start` → use `--server http://localhost:<port>` for commands.
2. **Reuse a profile**: `pinchtab instance start --profile work --mode headed` → switch to `--mode headless` after login is stored.
3. **Create profile via HTTP**: `POST /profiles` with `{"name":"..."}`, then `POST /profiles/<name>/start`.
4. **Human-assisted login**: Start headed, human signs in, agent reuses the profile headless.
5. **HTTP-only agent**: Use `POST /instances/start`, then target the instance port with curl. Send `X-Agent-Id` for attribution.

If the server is exposed beyond localhost, require a token. See [TRUST.md](./TRUST.md).

**Agent sessions**: Each agent can get its own revocable session token via `pinchtab session create --agent-id <id>` or `POST /sessions`. Set `PINCHTAB_SESSION=ses_...` or send `Authorization: Session ses_...`. Sessions have idle timeout (default 30m) and max lifetime (default 24h).

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
pinchtab nav <url> --print-tab-id                   # Print only the new tabId on stdout
pinchtab back                                       # Navigate back in history
pinchtab forward                                    # Navigate forward
pinchtab reload                                     # Reload current page
pinchtab tab                                        # List tabs (bare form; there is no `tab list` subcommand — `pinchtab tab list` returns 404)
pinchtab tab <tab-id>                               # Focus an existing tab
pinchtab tab new <url>                              # Open a new tab
pinchtab tab close <tab-id>                         # Close a tab — use this to clean up stale tabs between runs
pinchtab instance navigate <instance-id> <url>
```

When stdout is a pipe (e.g. inside `$(...)`), `nav` automatically switches to
`--print-tab-id` mode so agents can capture the tab ID with a single line,
then export it once via `PINCHTAB_TAB` so every subsequent tab-scoped command
picks it up without threading `--tab "$TAB"` through each call:

```bash
# Capture once, reuse across every following command.
export PINCHTAB_TAB=$(pinchtab nav http://example.com)
pinchtab snap -i -c
pinchtab eval "document.title"
pinchtab click '#submit'
pinchtab drag '#piece' '#zone-a'
```

An explicit `--tab <id>` on any command still overrides `PINCHTAB_TAB`.
See `references/env.md` for the full list of supported env vars.

### Observation

```bash
pinchtab snap
pinchtab snap -i                                    # Interactive elements only
pinchtab snap -i -c                                 # Interactive + compact
pinchtab snap -d                                    # Diff from previous snapshot
pinchtab snap --selector <css>                      # Scope to CSS selector
pinchtab snap --max-tokens <n>                      # Token budget limit
pinchtab snap --text                                # Text output format
pinchtab text                                       # Page text content (Readability-filtered; drops nav/repeated headlines)
pinchtab text --full                                # Full page text (document.body.innerText) — use when Readability is dropping content you need
pinchtab text --raw                                 # Alias of --full
# `text` has no `--format` / `--plain` flag — `--full` / `--raw` are the only mode switches. The CLI returns JSON `{url, title, text, truncated}`; pipe through `| jq -r .text` if you want just the body.
pinchtab find <query>                               # Semantic element search
pinchtab find --ref-only <query>                    # Return refs only
```

Guidance:

- `snap -i -c` is the default for finding actionable refs.
- `snap -d` is the default follow-up snapshot for multi-step flows.
- `text` is the default for reading articles, dashboards, reports, or confirmation messages.
- `find --ref-only` is useful when the page is large and you already know the semantic target.
- Refs from `snap -i` and full `snap` use different numbering. Do not mix them — if you snapshot with `-i`, use those refs. If you re-snapshot without `-i`, get fresh refs before acting.

### Interaction

All interaction commands accept unified selectors (refs, CSS, XPath, text, semantic). See the Selectors section above.

```bash
pinchtab click <selector>                           # Click element
pinchtab click --wait-nav <selector>                # Click and wait for navigation
pinchtab click --x 100 --y 200                      # Click by coordinates
pinchtab click <selector> --dialog-action accept    # Click + auto-accept any alert/confirm the click opens
pinchtab click <selector> --dialog-action dismiss   # Click + auto-dismiss
pinchtab click <selector> --dialog-action accept \
    --dialog-text "hello"                           # Click + accept a prompt() with a response
pinchtab dblclick <selector>                        # Double-click element
pinchtab mouse move <selector>                      # Move pointer to element center
pinchtab mouse move <x> <y>                         # Move pointer to coordinates
pinchtab mouse down <selector> --button left        # Press a mouse button at an explicit target
pinchtab mouse down --button left                   # Press a mouse button at current pointer
pinchtab mouse up <selector> --button left          # Release a mouse button at an explicit target
pinchtab mouse up --button left                     # Release a mouse button at current pointer
pinchtab mouse wheel 240 --dx 40                    # Dispatch wheel deltas at current pointer
pinchtab drag <from> <to>                           # Drag between selector/ref or x,y points (synthesized mouse sequence)
pinchtab drag <selector> --drag-x <n> --drag-y <n>  # Single-step drag by pixel offset (mirrors HTTP /action dragX/dragY)
pinchtab type <selector> <text>                     # Type with keystrokes
pinchtab fill <selector> <text>                     # Set value directly
pinchtab press <key>                                # Press key (Enter, Tab, Escape...)
pinchtab hover <selector>                           # Hover element — dispatches a `mousemove`, which normal event listeners and CSS `:hover` receive. Sites that wire hover via inline `onmouseenter="..."` attributes usually still respond; if they don't, fall back to `pinchtab eval` calling the handler directly (e.g. `pt eval "showInfo(2)"`).
pinchtab select <selector> <value|text>             # Select dropdown option by value attr, or fall back to visible text
pinchtab scroll <pixels|direction|selector>         # Positional only — no `--y`/`--pixels`/`--delta-y` flag. Valid directions: up, down, left, right (800 px step). `top`/`bottom` are NOT direction keywords — scroll to a specific element (e.g. `'#footer'`) or pass an integer like `scroll 99999` to scroll far enough. Examples: `pinchtab scroll 1500`, `pinchtab scroll down`, `pinchtab scroll '#footer'`.
```

Rules:

- Prefer `fill` for deterministic form entry.
- Prefer `type` only when the site depends on keystroke events.
- Prefer `click --wait-nav` when a click is expected to navigate.
- Prefer low-level `mouse` commands only when normal `click` / `hover` abstractions are insufficient, such as drag handles, canvas widgets, or sites that depend on exact pointer sequences.
- Re-snapshot immediately after `click`, `press Enter`, `select`, or `scroll` if the UI can change.
- `select` takes whatever you have: it tries the `<option value="...">` attribute first, then exact (trimmed) visible text, then case-insensitive trimmed text, then case-insensitive substring of visible text (last resort, first match wins). So `pinchtab select '#country' uk`, `pinchtab select '#country' 'United Kingdom'`, and `pinchtab select '#theme' 'Dark'` (matches option "Dark Mode") all work; the form receives the real `value=...` attr in every case. If nothing matches, the server returns a clear error listing available options. Prefer exact forms when multiple options share a prefix.
- If a click opens a JS dialog (`alert`, `confirm`, `prompt`), pass `"dialogAction": "accept"` or `"dialogAction": "dismiss"` on the click action body — or `pinchtab click <selector> --dialog-action accept` from the CLI (use `--dialog-text <str>` to supply a prompt response). The dialog is auto-handled in a single call. Without this, the click hangs until `/tabs/TAB_ID/dialog` is called from a parallel request, and a pending dialog wedges subsequent `/snapshot` and `/text` calls.
- For the `scroll` action via HTTP, use `"scrollX"` / `"scrollY"` for pixel deltas, or `"selector"` to scroll an element into view. Example: `{"kind":"scroll","scrollY":1500}` or `{"kind":"scroll","selector":"#footer"}`. The `x`/`y` fields are target viewport coordinates, not scroll deltas.
- The download HTTP endpoint (`GET /download?url=...` or `GET /tabs/TAB_ID/download?url=...`) returns JSON `{contentType, data (base64), size, url}`, not raw bytes. Decode `data` with base64 to get the file. Only `http`/`https` URLs are allowed. Private/internal hosts are blocked unless listed in `security.downloadAllowedDomains`.

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
pinchtab eval --await-promise "fetch('/api/me').then(r => r.json())"
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

### Tab-scoped HTTP API

**Important:** Each `POST /navigate` creates a new tab by default. The default (non-tab-scoped) endpoints like `/snapshot`, `/action`, `/text` operate on the *active* tab, which may not be the one you just navigated. In multi-tab workflows, always use tab-scoped routes to avoid acting on the wrong page.

Get the tab ID from the navigate response or from `GET /tabs`.

```bash
# List all tabs
curl http://localhost:9867/tabs \
  -H "Authorization: Bearer <token>"

# Navigate in a specific tab (does not create a new tab)
curl -X POST http://localhost:9867/tabs/TAB_ID/navigate \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

# Snapshot a specific tab
curl "http://localhost:9867/tabs/TAB_ID/snapshot?filter=interactive&format=compact" \
  -H "Authorization: Bearer <token>"

# Get text from a specific tab
curl http://localhost:9867/tabs/TAB_ID/text \
  -H "Authorization: Bearer <token>"

# Perform action on a specific tab
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","selector":"#submit-btn"}'

# Low-level pointer action on a specific tab
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-wheel","x":400,"y":320,"deltaY":240}'

# Drag action: selector + pixel OFFSETS (dragX, dragY), not absolute endpoints.
# For low-level absolute coords, sequence mouse-down / mouse-move / mouse-up.
curl -X POST http://localhost:9867/tabs/TAB_ID/action \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"drag","selector":"#piece","dragX":12,"dragY":-158}'

# Navigate back/forward in a specific tab
curl -X POST http://localhost:9867/tabs/TAB_ID/back \
  -H "Authorization: Bearer <token>"
curl -X POST http://localhost:9867/tabs/TAB_ID/forward \
  -H "Authorization: Bearer <token>"

# Screenshot (GET, not POST)
curl http://localhost:9867/tabs/TAB_ID/screenshot \
  -H "Authorization: Bearer <token>" \
  --output screenshot.png

# PDF export (GET or POST)
curl http://localhost:9867/tabs/TAB_ID/pdf \
  -H "Authorization: Bearer <token>" \
  --output page.pdf

# Close a tab
curl -X POST http://localhost:9867/tabs/TAB_ID/close \
  -H "Authorization: Bearer <token>"

# Pause automation for a human-only step
curl -X POST http://localhost:9867/tabs/TAB_ID/handoff \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"captcha","timeoutMs":120000}'

# Inspect handoff status
curl http://localhost:9867/tabs/TAB_ID/handoff \
  -H "Authorization: Bearer <token>"

# Resume automation after the human step is done
curl -X POST http://localhost:9867/tabs/TAB_ID/resume \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"status":"completed"}'

```

The default (non-tab-scoped) endpoints also support screenshots and PDF:

```bash
# Screenshot of active tab (GET)
curl http://localhost:9867/screenshot \
  -H "Authorization: Bearer <token>" \
  --output screenshot.png

# PDF of active tab (GET or POST)
curl http://localhost:9867/pdf \
  -H "Authorization: Bearer <token>" \
  --output page.pdf
```

**Navigation with `waitNav`:** When clicking a link or button that triggers page navigation, include `"waitNav": true` in the action body. Without it, PinchTab returns a `navigation_changed` error to protect against unexpected navigation during form interactions.

```json
{"kind": "click", "selector": "#search-btn", "waitNav": true}
```

All tab-scoped routes follow the pattern `/tabs/{TAB_ID}/...` and mirror the default endpoints. The full list includes: `navigate`, `back`, `forward`, `reload`, `snapshot`, `screenshot`, `text`, `pdf`, `action`, `actions`, `dialog`, `wait`, `find`, `lock`, `unlock`, `cookies`, `metrics`, `network`, `solve`, `close`, `storage`, `evaluate`, `download`, `upload`, `handoff`, and `resume`.

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

**Form submission:** Always click the submit button — never use `press Enter`. Most HTML forms only fire their submission handler on button click, not on Enter keypress.

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


## Security and Token Economy

- Use a dedicated automation profile, not a daily browsing profile.
- If PinchTab is reachable off-machine, require a token and bind conservatively.
- Prefer `text`, `snap -i -c`, and `snap -d` before screenshots, PDFs, eval, downloads, or uploads.
- Use `--block-images` for read-heavy tasks that do not need visual assets.
- Stop or isolate instances when switching between unrelated accounts or environments.

## Diffing and Verification

- Use `pinchtab snap -d` after each state-changing action in long workflows.
- Use `pinchtab text` to confirm success messages, table updates, or navigation outcomes. The default mode extracts Readability-filtered content (reader view), which may drop navigation, repeated headlines, short-text nodes, or collapse lists/grids down to a single representative item. Reach for `pinchtab text --full` whenever (a) you're verifying content on a list/grid/tab/accordion page, (b) the expected marker is short, or (c) a default read came back missing content you can see in the snapshot. It returns the raw `document.body.innerText` and is almost always the safer choice once you know Readability is going to trim.
- Use `pinchtab screenshot` only when visual regressions, CAPTCHA, or layout-specific confirmation matters.
- If a ref disappears after a change, treat that as expected and fetch fresh refs instead of retrying the stale one.
- Action responses like `{"clicked":true,"submitted":true}` mean the event fired on the target element — **not** that the form was accepted by the server or passed native HTML validation. Always verify the expected success marker or state change via `snap`/`text` before treating a submission as complete.
- **Same-origin iframes** are supported natively via `pinchtab frame <target>` — a stateful scope that subsequent selector-based `/snapshot` and `/action` calls inherit. Typical flow: `pinchtab frame '#payment-frame'` → `pinchtab snap -i -c` (refs reflect iframe interior) → `pinchtab fill '#card'` / `click '#pay'` → `pinchtab frame main`. Target accepts `main`, an iframe ref, a CSS selector for the iframe element, a frame name, or a frame URL. Nested iframes need multiple hops. Refs emitted by a full `snap` (no `-i`) for iframe descendants carry frame context — ref-based actions work across the boundary without an explicit scope set. **Cross-origin iframes** are not exposed as frame scopes; fall back to `eval` against `iframe.contentDocument` (same-origin-policy permitting). `pinchtab text` (and `text --full`) honors the active frame scope and also accepts an explicit `--frame <frameId>` flag for one-shot reads — so after `pinchtab frame '#content-frame'`, a following `pinchtab text --full` extracts from the iframe's document, not the outer page. **The `--frame` argument must be a frame ID (the 32-char hex `frameId` from `pinchtab frame <target>` output), not a CSS selector.** For a one-shot read, the idiom is: `FID=$(pinchtab frame '#content-frame' | jq -r .current.frameId); pinchtab frame main; pinchtab text --full --frame "$FID"`. Passing a selector like `text --frame '#content-frame'` returns "no frame for given id found".
- **`eval` → always IIFE.** `eval` expressions share the same realm across calls, so any top-level `const`/`let`/`class` from one call collides with the next: `SyntaxError: Identifier 'x' has already been declared`. Use an IIFE on every `eval` that introduces identifiers, not only on multi-statement ones: `pinchtab eval "(() => { const r = document.querySelector('#x').getBoundingClientRect(); return {x: r.x, y: r.y, w: r.width, h: r.height}; })()"`. For a single expression that doesn't introduce identifiers (e.g. `document.title`, `document.getElementById('x').value`), the IIFE is optional. The IIFE pattern also fixes DOMRect serialization — `getBoundingClientRect()` returns a value whose own-enumerable fields don't survive JSON, so the explicit projection is what actually ships the numbers back.
- **`pinchtab text` (both default and `--full`) returns content from `display:none` and `visibility:hidden` nodes** because it reads `document.body.innerText` (and Readability's input) from raw DOM — the visibility cascade is not applied. When you need to confirm that a success banner or error message is *actually visible* (not just present as a pre-seeded hidden element), verify via `pinchtab snap` (the accessibility tree respects visibility and hides non-rendered subtrees) or via `eval` against the element's `offsetHeight` / `getComputedStyle().display`. A common trap: a page ships with a hidden success `<div>` pre-rendered; `text` will report the success string before the form is ever submitted.
- The compact snapshot shows `<option>` elements by their visible text, not their `value` attribute. You don't normally need to look up the `value`: the `select` action accepts either — it matches on `value` first and falls back to visible text (case-insensitive). Only reach for `eval` + `Array.from(select.options)` when debugging an unexpected no-match error.
- `text:<value>` selectors are resolved by a JS-level search over visible text and can intermittently fail with `DOM Error` or `context deadline exceeded` on large/dynamic pages. If you have a fresh `snap -i -c` in hand, prefer the ref (`e12`) — refs resolve by stable backend node IDs and don't depend on page-side JS.
- `snap -i -c` (interactive, compact) skips non-interactive descendants. For iframe interiors, either set a `frame` scope first or use a full `pinchtab snap` (no `-i`) which flattens same-origin iframe descendants into the parent snapshot.
- ARIA expansion state (`aria-expanded="true" | "false"`) is usually placed on the **outermost container** of an accordion/menu/disclosure section, not on the header/trigger that dispatches the click. When verifying state after a click, query `document.querySelector('#section-a').getAttribute('aria-expanded')` (or the wrapper's equivalent) rather than the clicked element.
- `click --wait-nav` can return `{"success": true}` or, immediately after the navigation fires, `Error 409: unexpected page navigation` — the latter means the server saw a navigation while mid-response and aborted its reply, not that the click failed. Treat 409 after a navigation-expected click as success and verify the resulting page with a fresh `snap` / `text`.


## References

- Full API: [api.md](./references/api.md)
- Minimal env vars: [env.md](./references/env.md)
- Agent optimization: [agent-optimization.md](./references/agent-optimization.md)
- Profiles: [profiles.md](./references/profiles.md)
- MCP: [mcp.md](./references/mcp.md)
- Security model: [TRUST.md](./TRUST.md)
