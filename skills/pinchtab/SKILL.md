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

Same syntax in HTTP API via `selector` field. Legacy `ref` field still accepted.

## Command Chaining

Use `&&` when you don't need intermediate output: `pinchtab nav <url> && pinchtab snap -i -c`. Run separately when you must read refs before acting.

## Challenge Solving

If a page shows a challenge instead of content (e.g., "Just a moment..."), call `POST /solve` with `{"maxAttempts": 3}` to auto-detect and resolve it. Use `POST /tabs/TAB_ID/solve` for tab-scoped. Works best with `stealthLevel: "full"` in config. Safe to call speculatively — returns immediately if no challenge is present. See [api.md](./references/api.md) for full solver options.

## Handling Authentication and State

Patterns: (1) One-off: `pinchtab instance start` → `--server http://localhost:<port>`. (2) Reuse profile: `pinchtab instance start --profile work --mode headed` → switch to headless after login. (3) HTTP: `POST /profiles`, then `POST /profiles/<name>/start`. (4) Human-assisted: headed login, then agent reuses headless.

Agent sessions: `pinchtab session create --agent-id <id>` or `POST /sessions` → set `PINCHTAB_SESSION=ses_...`.

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
pinchtab tab                                        # List tabs (no subcommand - just `tab`)
pinchtab tab <tab-id>                               # Focus an existing tab
pinchtab tab new <url>                              # Open a new tab
pinchtab tab close <tab-id>                         # Close a tab — use this to clean up stale tabs between runs
pinchtab instance navigate <instance-id> <url>
```

**Tab workflow** — most commands target the active tab by default, so single-tab flows need no plumbing:
```bash
pinchtab nav http://example.com
pinchtab snap -i -c      # active tab
pinchtab click e5        # active tab
pinchtab text            # active tab
```

When you need to pin to a specific tab (parallel tabs, long-running flows, or shell-isolated
runners like agent tool calls where env vars don't persist across invocations), capture the
tab ID with `--print-tab-id` and pass `--tab` on every subsequent command:
```bash
TAB_ID=$(pinchtab nav http://example.com --print-tab-id)
pinchtab --tab "$TAB_ID" snap -i -c
pinchtab --tab "$TAB_ID" click e5
pinchtab --tab "$TAB_ID" text
```

Within a single shell session you can also `export PINCHTAB_TAB=$(pinchtab nav URL --print-tab-id)`
and drop the `--tab` flag. This does **not** survive across separate shell invocations (each
`Bash` tool call in an agent runs a fresh shell), so prefer explicit `--tab` for agent workflows.

Priority: `--tab <id>` flag > `PINCHTAB_TAB` env var > active tab.

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
# CLI returns JSON; use `| jq -r .text` for plain text
pinchtab find <query>                               # Semantic element search
pinchtab find --ref-only <query>                    # Return refs only
```

Guidance:

- `snap -i -c` is the default for finding actionable refs.
- `snap -d` is the default follow-up snapshot for multi-step flows.
- `text` is the default for reading articles, dashboards, reports, or confirmation messages.
- **`pinchtab find <query>`** is the direct route when you already know the semantic target
  (e.g. "login button", "email input", "accept cookies link") — skips the full snapshot and
  returns a ranked match with its ref. Pair with `--ref-only` on large/dense pages to get just
  the ref string for piping straight into `click` / `fill` / `type`. Prefer `find` over
  `snap -i -c` + visual scan whenever you can describe the target in a phrase.
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
pinchtab hover <selector>                           # Hover element
pinchtab select <selector> <value|text>             # Select dropdown option by value attr, or fall back to visible text
pinchtab scroll <pixels|direction|selector>         # e.g. `scroll 1500`, `scroll down`, `scroll '#footer'`
```

Rules:

- Prefer `fill` for deterministic form entry.
- Prefer `type` only when the site depends on keystroke events.
- Prefer `click --wait-nav` when a click is expected to navigate.
- Prefer low-level `mouse` commands only when normal `click` / `hover` abstractions are insufficient, such as drag handles, canvas widgets, or sites that depend on exact pointer sequences.
- Re-snapshot immediately after `click`, `press Enter`, `select`, or `scroll` if the UI can change.
- `select` matches by value attr first, then visible text (case-insensitive). Error lists available options if no match.
- For JS dialogs: use `--dialog-action accept` or `--dialog-action dismiss` on click. Add `--dialog-text` for prompt responses.
- For the `scroll` action via HTTP, use `"scrollX"` / `"scrollY"` for pixel deltas, or `"selector"` to scroll an element into view. Example: `{"kind":"scroll","scrollY":1500}` or `{"kind":"scroll","selector":"#footer"}`. The `x`/`y` fields are target viewport coordinates, not scroll deltas.
- The download HTTP endpoint (`GET /download?url=...` or `GET /tabs/TAB_ID/download?url=...`) returns JSON `{contentType, data (base64), size, url}`, not raw bytes. Decode `data` with base64 to get the file. Only `http`/`https` URLs are allowed. Private/internal hosts are blocked unless listed in `security.downloadAllowedDomains`.

### Waiting

Use `wait` when the DOM settles asynchronously — spinners, toasts, XHR-driven content.

```bash
pinchtab wait <selector>                            # Element to appear (default visible)
pinchtab wait <selector> --state hidden             # Element to disappear
pinchtab wait --text "Order confirmed"              # Text to appear
pinchtab wait --not-text "Loading..."               # Text to disappear (spinner/toast dismiss)
pinchtab wait --url "**/dashboard"                  # URL glob match
pinchtab wait --load networkidle                    # Network idle
pinchtab wait 500                                   # Fixed delay in ms (last resort)
```

Default timeout 10s, max 30s via `--timeout <ms>`. Prefer `--not-text` / `--state hidden` over polling.

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

Use curl when CLI unavailable. Key endpoints on instance port (e.g. 9867):
- `POST /navigate` with `{"url":"..."}`
- `GET /snapshot?filter=interactive&format=compact`
- `POST /action` with `{"kind":"fill","selector":"e3","text":"..."}`
- `POST /actions` with a batch of actions — runs them in one round-trip. Body accepts either
  an array `[{"kind":"fill",...},{"kind":"click",...}]` or an envelope
  `{"actions":[...],"stopOnError":true,"tabId":"..."}`. Use this for tight form flows (fill +
  fill + click submit) to cut round-trip latency. Set `stopOnError:true` to halt on the first
  failure; the response contains a per-step `{index, success, result?, error?}` array.
  Tab-scoped variant: `POST /tabs/TAB_ID/actions`.
- `GET /text`
- `POST /solve` with `{"maxAttempts": 3}`

### Tab-scoped HTTP API

Use `/tabs/TAB_ID/...` routes to target specific tabs. Get tab ID from navigate response or `GET /tabs`.

Pattern: `curl -H "Authorization: Bearer <token>" http://localhost:9867/tabs/TAB_ID/<endpoint>`

Key endpoints: `navigate`, `snapshot`, `text`, `action`, `screenshot`, `pdf`, `back`, `forward`, `close`, `wait`, `download`, `upload`, `handoff`, `resume`.

Action examples:
- Click: `{"kind":"click","selector":"#btn"}`
- Click with nav: `{"kind":"click","selector":"#link","waitNav":true}`
- Drag: `{"kind":"drag","selector":"#piece","dragX":12,"dragY":-158}`
- Scroll: `{"kind":"scroll","scrollY":1500}` or `{"kind":"scroll","selector":"#footer"}`

## Common Patterns

- **Form flow**: `nav` → `snap -i -c` → `fill` fields → `click --wait-nav` submit → `text` to verify
- **Multi-step**: After each action, `snap -d -i -c` for diff
- **Direct selectors**: Skip snapshot when structure is known: `pinchtab click "text:Accept Cookies"` or `fill "#search" "query"`

**Form submission:** Always click the submit button — never use `press Enter`.

## Token Economy

Prefer low-token commands: `text`, `snap -i -c`, `snap -d`. Use `--block-images` for read-heavy tasks. Reserve screenshots/PDFs for visual verification.

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
