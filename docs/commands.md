# Commands Reference

## Server And Runtime

```bash
pinchtab server                         # Start the full server (dashboard + API)
pinchtab server -b                      # Start it detached; logs go to <stateDir>/server.log
pinchtab server -v                      # Full startup banner, and log at debug level
pinchtab server --log-level warn        # Record warnings and errors only
pinchtab server stop                    # Stop the running server (foreground or background)
pinchtab server restart                 # Stop + restart in background (applies config changes)
pinchtab bridge                         # Start the bridge-only runtime
pinchtab bridge --log-level debug       # Bridge threshold (same precedence as server)
pinchtab mcp                            # Start the MCP stdio server
pinchtab dashboard                      # Open the dashboard in your browser (--no-open prints the URL)
pinchtab session create --agent-id <id> # Create an agent session (--grant limits it to capability groups)
pinchtab session list                   # List agent sessions
pinchtab session info                   # Show the current agent session
pinchtab session revoke <session-id>    # Revoke an agent session
pinchtab daemon                         # Show daemon status
pinchtab daemon install                 # Install as a background service
pinchtab daemon start                   # Start the background service
pinchtab daemon stop                    # Stop the background service
pinchtab daemon restart                 # Restart the background service
pinchtab daemon uninstall               # Remove the background service
pinchtab completion <shell>             # Generate shell completions
```

Logging is a level, not an on/off switch. Every run — foreground or `--background`
— records the per-request access log (with its `requestId`), instance lifecycle
transitions, warnings and errors. The threshold comes from the first of these that
is set: `--log-level debug|info|warn|error`, then `server.logLevel` in the config
file, then `-v`, then the default `info`. `-v` always adds the full startup banner,
and it raises the level to debug only when neither of the other two is set.

Where those lines land depends on how the server was started, which is why `pinchtab`
with no arguments prints a `logs` row naming the live destination. There are four ways a
server starts, and three destinations: `<stateDir>/server.log` for `pinchtab server -b`
**and for the server a bare `pinchtab nav` or `pinchtab mcp` auto-starts** — both spawn
detached and both append to that same file — `~/.pinchtab/logs/daemon.err.log` for a
daemon-installed service, and the terminal for a foreground run. A `server.log` left
behind by an earlier detached run is called out as not being written by the current
server, so it cannot be mistaken for a live one.

A request that fails logs its cause there UNREDACTED — absolute paths intact — under the
same `requestId` the access log records, so a 5xx can be joined to the reason it happened:

```bash
grep '"request failed"' <stateDir>/server.log        # causes, with requestId and status
grep <requestId> ~/.pinchtab/activity/*.jsonl        # the access-log line it belongs to
```

The message that crosses the HTTP boundary is still path-sanitized (`fork/exec [path]`),
so the unredacted copy exists only in the server's own log. Server faults (5xx) log at
error level; a 4xx is the caller's input and logs at debug.

The access log is what an open dashboard costs: its errors and console panels each poll
on a 3s interval, so a dashboard left open writes roughly 40 lines a minute. That is the
deliberate trade for a run that explains itself afterwards, and `--log-level warn` is the
escape hatch when you want the record without the polling.

A daemon-installed server and the server a bare `pinchtab nav` auto-starts both run
`pinchtab server` with no flags, so `server.logLevel` is the only way to set their
threshold (`pinchtab config set server.logLevel warn`). That also means a 4xx cause is
not written at the default `info` level; raise the level when you need one.

Everything in this paragraph applies to `pinchtab bridge` as well: it reads the same
`server.logLevel`, accepts the same `--log-level`, and resolves them with the same
precedence — which matters because the bridge holds the CDP session, so it owns the
target-crash, instance-lifecycle and selector-resolution logging. The one difference
is that `bridge` has no `-v`: `-v` also switches on the server's startup banner, and
the bridge has no banner to switch on, so `--log-level debug` is how you raise a
bridge. Orchestrator-spawned bridges inherit the level through their child config.

## Navigation

```bash
pinchtab nav <url>                      # Navigate current tab, or create one if needed
pinchtab nav <url> --tab <id>           # Reuse a specific tab
pinchtab nav <url> --new-tab            # Explicitly force a new tab
pinchtab nav <url> --timeout 90          # Allow up to 90s (maximum 120s)
pinchtab nav <url> --block-images       # Block images for this navigation
pinchtab nav <url> --block-ads          # Block ads for this navigation
pinchtab nav <url> --snap               # Navigate and output interactive snapshot
pinchtab nav <url> --text               # Navigate and output page text
pinchtab nav <url> --print-tab-id       # Print only the tab ID, whatever stdout is (with --snap/--text the tab ID goes to stderr)
pinchtab back                           # Go back in the active tab
pinchtab back --tab <id>                # Go back in a specific tab
pinchtab forward                        # Go forward in the active tab
pinchtab reload                         # Reload the active tab
```

`back` and `forward` report the URL the page actually landed on, so a redirect,
a login wall or an error page is visible without `--snap`. When the tab is at the
edge of history and nothing moved, they print `OK no history` and still exit 0.
`reload` is unchanged: it always reports the landed URL.

`nav` is different, because its stdout is a value other commands consume: it
prints the tab ID first, and adds the landed URL only when stdout is a terminal.
Under `--print-tab-id`, a pipe or a redirect, stdout carries the tab ID alone, so
`TAB=$(pinchtab nav <url>)` captures a usable tab ID. A scripted navigation that
needs to know where it landed should ask for `--json`, which prints the response
body carrying `url`, or `--snap`, which puts the landed URL in the header line
above the nodes. Not `--text`: the URL reaches it only inside the IDPI
trust-boundary wrapper, so it disappears wherever `security.idpi.enabled` or
`security.idpi.wrapContent` is off.

## Tabs

The `tab` command only lists, focuses, and closes tabs. It does not proxy the rest of the browser command set.

```bash
pinchtab tab                            # List tabs
pinchtab tab <id>                       # Focus a tab by ID or 1-based index
pinchtab nav <url> --new-tab            # Open a new tab and navigate it
pinchtab tab close <id>                 # Close a tab
pinchtab close <id>                     # Same as tab close
```

Use top-level commands with `--tab` for tab-scoped work:

```bash
pinchtab snap --tab <id>
pinchtab click --tab <id> <selector>
pinchtab pdf --tab <id> -o page.pdf
```

Unscoped tab commands use server-side current-tab state when the caller is
identified: `PINCHTAB_SESSION` scopes current tabs by session, while
`--agent-id` or `PINCHTAB_AGENT_ID` scopes them by agent ID when no session is
present. Anonymous CLI calls keep using the shared local current-tab state file.

## Interaction

Most element commands accept a unified selector:

- snapshot ref such as `e5`
- CSS selector such as `#login`
- XPath such as `xpath://button`
- text selector such as `text:Submit`
- semantic selector such as `find:login button`
- role/name selector such as `role:button Save`
- label, placeholder, alt, title, or test id selectors such as `label:Email`, `placeholder:Search`, `alt:Logo`, `title:Close`, `testid:submit`
- positional wrappers such as `first:button`, `last:role:button`, or `nth:2:button` (`nth` is zero-based)

Positional wrappers index the candidates in **document order** for every selector kind, so `nth:0:` is the first match in the page and `nth:1:` always comes after it. A bare `text:` selector is the one form that does not index: it picks the most control-like match among the smallest ones, so `text:Save` prefers a `<button>` over a `<div>` carrying the same label — which means `text:X` and `first:text:X` can resolve to different elements. A wrapper only ever chooses among the matches a bare selector would find; it never changes which matches exist.

`text:` prefers an **exact label over containment**. On a page offering both `Save` and `Save and exit`, `text:Save` is the button labelled exactly `Save`, wherever either sits in the document. For a bare `text:` the exact label only breaks ties between equally control-like matches: a heading reading `Sign in` does not take the click from a `Sign in with Google` button, and a label inside a control counts as that control. Positional wrappers index the exact matches when there are any, and all containing matches otherwise, which is what lets `text:Sign` reach `Sign in with Google`. This is the same ladder [`select`](./reference/select.md) uses for option text. Prefer the full label when controls share a prefix — `Delete` and `Delete all` are one typo apart in effect.

Selector prefixes are case-insensitive, so `CSS:#login` and `css:#login` mean the same thing. Only the prefix is case-folded; the value after it is passed through unchanged.

Structured forms such as `role:`, `label:`, and `testid:` are matched by the semantic engine against enriched snapshot descriptors. CSS, XPath, refs, the existing `text:` action selector, and bare CSS/text wrappers remain browser-side selector resolution.

Selector lookup is explicit by frame. Unscoped selectors search only the current frame scope, which defaults to `main`. Use `pinchtab frame ...` before selector-based iframe work. Same-origin iframe scopes are supported; cross-origin iframe descendants are not currently exposed.

```bash
pinchtab frame                         # Show current frame scope
pinchtab frame "#payment-frame"        # Scope selectors to an iframe
pinchtab frame main                    # Return selector scope to the top document
pinchtab click [selector]               # Click an element or coordinates with --x/--y
pinchtab click <selector> --submit      # One terminal submit click; reports bounded post-submit state
pinchtab click --css <selector>         # Force CSS selector mode
pinchtab click --wait-nav <selector>    # Click and wait for navigation
pinchtab click --snap <selector>        # Click and output interactive snapshot
pinchtab dblclick [selector]            # Double-click
pinchtab type <selector> <text>         # Type via key events
pinchtab fill <selector> <text>         # Fill directly
pinchtab press <key>                    # Press a key
pinchtab hover [selector]               # Hover an element
pinchtab mouse move <x> <y>             # Move the mouse to coordinates
pinchtab mouse move [selector]          # Or move to an element center
pinchtab mouse down [selector]          # Press a mouse button
pinchtab mouse up [selector]            # Release a mouse button
pinchtab mouse wheel [dy|selector]      # Dispatch wheel deltas
pinchtab drag <from> <to>               # Drag between targets (selector/ref or x,y)
pinchtab drag <selector> --drag-x <n> --drag-y <n>  # Drag by a pixel offset
pinchtab focus [selector]               # Focus an element
pinchtab scroll <selector|pixels>       # Scroll an element or the page
pinchtab scroll down --snap             # Scroll and output snapshot
pinchtab scroll 800 --snap-diff         # Scroll and output snapshot diff
pinchtab select <selector> <value>      # Select a <select> option
pinchtab check <selector>               # Check a checkbox or radio
pinchtab uncheck <selector>             # Uncheck a checkbox or radio
pinchtab scrollintoview <selector>      # Scroll an element into view
```

Low-level mouse commands are useful for drag handles, canvas-like UIs, and flows where DOM-native click or hover abstractions are not enough:

```bash
pinchtab mouse move e5
pinchtab mouse down --button left
pinchtab mouse up --button left
pinchtab mouse wheel 240 --dx 40
pinchtab mouse move --x 400 --y 320
pinchtab drag e5 400,320
```

## Page Analysis

```bash
pinchtab snap [selector]                # Accessibility snapshot, optionally scoped
pinchtab snap -i -c                     # Interactive + compact
pinchtab snap -d                        # Diff from previous snapshot
pinchtab snap --selector <css>          # Scope snapshot
pinchtab snap --max-tokens <n>          # Limit token budget
pinchtab snap --depth <n>               # Limit tree depth
pinchtab snap --text                    # Text output
pinchtab text                           # Extract readable text
pinchtab text --full                    # Full page innerText
pinchtab text --raw                     # Raw extraction
pinchtab text --markdown                 # Markdown (preserves links, tables)
pinchtab text --markdown --output page.md # Write Markdown to a file (one-line confirmation)
pinchtab text --frame <frameId>         # Read text from one iframe
pinchtab html [selector]                # Document or element HTML (--max-chars, --frame)
pinchtab styles [selector]              # Computed styles (root element when omitted; --prop for one)
pinchtab title                          # Current tab title
pinchtab url                            # Current tab URL
pinchtab value <ref>                    # Current value of a form element
pinchtab attr <ref> <name>              # Value of one HTML attribute
pinchtab box <ref>                      # Bounding box of an element
pinchtab checked <ref>                  # Whether an element is checked
pinchtab enabled <ref>                  # Whether an element is enabled
pinchtab visible <ref>                  # Whether an element is rendered
pinchtab count <selector>               # Count elements matching a CSS selector
pinchtab find <query>                   # Semantic element search
pinchtab find --threshold <0-1>         # Minimum similarity score
pinchtab find --explain                 # Include score breakdown
pinchtab find --ref-only                # Print only the best ref
pinchtab extract --schema <file>        # Schema-typed data as JSON (see reference/extract.md)
pinchtab extract --schema -             # Read the schema from stdin
pinchtab extract --schema <file> --fields   # + field<TAB>ref<TAB>confidence table
pinchtab extract --schema <file> --scope role:table --max-items 5  # Confine and cap
pinchtab eval <expression>              # Evaluate JavaScript
pinchtab a11y audit                     # Accessibility score + findings (native engine)
pinchtab a11y audit --axe               # Run axe-core in the page (industry rule ids)
pinchtab a11y audit --axe --tags wcag2a,wcag2aa   # axe: filter by WCAG tags
pinchtab a11y audit --axe --rules image-alt,label # axe: run only these rule ids
pinchtab a11y audit --axe --json        # Full JSON envelope with per-node refs
pinchtab memory                         # JS heap usage and DOM counters for the tab
pinchtab memory --gc --json             # Collect garbage first, raw JSON
pinchtab memory snapshot                # V8 heap snapshot to a server-side file (security.allowMemory)
pinchtab memory snapshot --out app.heapsnapshot  # Also copy it to a local path
pinchtab memory summary <id> --top 5    # Top constructors and duplicate strings of a snapshot
pinchtab memory compare <a> <b> --top 5 # Constructor growth from snapshot a to snapshot b
pinchtab memory compare <a> <b> --retained  # Also retained sizes from b's dominator tree
```

`pinchtab memory snapshot`, `summary` and `compare` need
`security.allowMemory`, because a heap snapshot holds every string on the page.
See [reference/memory.md](reference/memory.md).

`pinchtab a11y audit --axe` runs the vendored axe-core engine (see
[reference/a11y.md](reference/a11y.md)) in the page's isolated world, so page
script cannot tamper with the result. Each violation node that maps to a
snapshot ref carries it, so a failing element can be actioned directly with
`pinchtab click` and the other element commands.

`pinchtab eval` is intentionally not frame-scoped. Current `pinchtab frame`
state affects selector-based commands such as `snap`, `click`, `fill`, and
`type`, and it also affects `text` when `--frame` is not provided explicitly.

Selector-based actions now fail fast when a selector does not match. If the UI
is still loading, use `pinchtab wait` first instead of relying on action
timeouts.

For a form button whose action must never be retried, use `pinchtab click
<selector> --submit`. This performs exactly one DOM click and reports a short
post-submit observation instead of retrying delivery. It cannot be combined
with `--wait-nav`, `--mode`, or `--humanize`; use a normal click for those
workflows. `pinchtab fill <selector> <text> --submit` remains the separate
Enter-after-fill shortcut.

## Standalone Bridges

Standalone `pinchtab bridge` processes register themselves in the local state
directory while running. Inspect them without sending signals:

```bash
pinchtab bridges list
pinchtab bridges list --json
pinchtab bridges list --prune   # Remove only records whose original PID is dead or reused
```

`--prune` never kills a process. It removes only conclusively stale registry
records; an unreachable listener with a live or unknown PID remains visible for
operator investigation.

## Keyboard, Wait, And Diagnostics

```bash
pinchtab keyboard type <text>           # Type at the focused element
pinchtab keyboard inserttext <text>     # Insert text without key events
pinchtab keydown <key>                  # Hold a key down
pinchtab keyup <key>                    # Release a key
pinchtab wait <selector>                # Wait for a match to be rendered (--state visible)
pinchtab wait <selector> --state hidden # Wait until nothing matches or no match is rendered
pinchtab wait <selector> --state attached # Wait for a match in the DOM, rendered or not
pinchtab wait <selector> --state detached # Wait until nothing matches
pinchtab wait <ms>                      # Fixed duration sleep (escape hatch; max 30000ms — prefer condition-based waits)
pinchtab wait --text <text>             # Wait for page text to appear
pinchtab wait --not-text <text>         # Wait for page text to disappear
pinchtab wait --url <glob>              # Wait for URL match (glob: **, *, ?)
pinchtab wait --load <state>            # state: ready-state | content-loaded | network-idle
                                        #   ready-state    → document.readyState === 'complete'
                                        #   content-loaded → readyState in {interactive, complete}
                                        #   network-idle   → 0 in-flight requests for 500ms (HTTP `idleFor` overrides; no CLI flag)
pinchtab wait --fn <expression>         # Wait for JS to become truthy
pinchtab wait ... --timeout-ms <ms>     # Override timeout in ms (default 10000, max 30000); --timeout is a deprecated alias
pinchtab network                        # List captured network requests
pinchtab network <requestId>            # Show one request in detail
pinchtab network --stream               # Stream network entries
pinchtab network --clear                # Clear captured network data
pinchtab network route <url> --abort    # Block matching requests (--body '<json>' fulfills instead)
pinchtab network unroute [url]          # Remove one interception rule, or all of them
pinchtab network rules                  # List interception rules
# HAR / NDJSON export is available over HTTP (no dedicated CLI subcommand):
#   curl http://127.0.0.1:9867/network/export                 → HAR 1.2 archive
#   curl http://127.0.0.1:9867/network/export?format=ndjson   → NDJSON (one entry per line)
#   curl http://127.0.0.1:9867/network/export?body=1          → include response bodies
#   curl http://127.0.0.1:9867/network/export/stream          → live HAR stream
# Per-tab variants live under /tabs/{id}/network/export[/stream].
pinchtab dialog accept [text]           # Accept alert/confirm/prompt
pinchtab dialog dismiss                 # Dismiss dialog
pinchtab console                        # Show console logs
pinchtab console --clear                # Clear console logs
pinchtab errors                         # Show browser error logs
pinchtab errors --clear                 # Clear browser error logs
pinchtab clipboard read                 # Read server-side clipboard text
pinchtab clipboard write <text>         # Write clipboard text
pinchtab clipboard copy <text>          # Alias for write
pinchtab clipboard paste                # Alias for read
pinchtab cache clear                    # Clear browser HTTP disk cache
pinchtab cache status                   # Check if cache can be cleared
```

`wait` takes exactly one condition: a selector, a millisecond count, or one of
`--text`, `--not-text`, `--url`, `--load`, `--fn`. Giving two is refused with an
error naming them rather than waiting on one and ignoring the other. `--state`
applies to a selector only; to wait for text to disappear use `--not-text`.
Rendered means a non-empty box that `display:none`, `visibility:hidden`,
`opacity:0` and the `hidden` attribute do not suppress, so `--state hidden`
completes when a modal or spinner is hidden by CSS as well as when it is removed.

Manual handoff and resume are available via CLI and API:

```bash
pinchtab tab handoff <tabId> --reason captcha --timeout-ms 120000
pinchtab tab handoff-status <tabId>
pinchtab tab resume <tabId> --status completed
```

`pinchtab handoff`, `pinchtab handoff-status` and `pinchtab resume` are top-level spellings of the same three commands.

API equivalents:

Paused handoff state blocks action execution routes (`/action`, `/actions`, `/macro`) with `409 tab_paused_handoff`
until resumed or expired via timeout.

```bash
curl -X POST "$PINCHTAB_SERVER/tabs/<tabId>/handoff"
curl "$PINCHTAB_SERVER/tabs/<tabId>/handoff"
curl -X POST "$PINCHTAB_SERVER/tabs/<tabId>/resume"
```

## Capture And Export

```bash
pinchtab screenshot                     # Save a screenshot to a generated .jpg path
pinchtab screenshot -o <path>           # Save to a chosen path (.png infers PNG; otherwise JPEG)
pinchtab screenshot --format <jpeg|png>  # Override the inferred output format
pinchtab screenshot -q <0-100>          # JPEG quality
pinchtab screenshot -s <selector>       # Capture a specific element by selector
pinchtab screenshot --scale 0.5         # Half-size output (quarter the pixels)
pinchtab screenshot --beyond-viewport   # Capture the full scrollable document (ignored with -s)
pinchtab screenshot --annotate          # Bake numbered ref boxes into the image (for vision models)
pinchtab annotate                       # Inject a persistent, clickable overlay on the LIVE page
pinchtab annotate -s <selector>         # Scope the overlay to elements within a selector
pinchtab annotate --clear               # Remove the persistent overlay
pinchtab capture                        # Paired screenshot + accessibility snapshot from the same DOM epoch
pinchtab capture -o <path>              # Save the paired image to a chosen path
pinchtab capture --beyond-viewport      # Capture the full document; bounds in page coords
pinchtab capture --require-pair         # Fail (409) if the page navigated mid-capture
pinchtab capture --with-bounds=false    # Skip per-node DOM.getBoxModel round trips
pinchtab capture --scale 0.5            # Half-size image (snapshot/bounds unchanged)
pinchtab pdf                            # Export the active page as PDF
pinchtab pdf -o <path>                  # Save PDF to a chosen path
pinchtab pdf --landscape                # Landscape orientation
pinchtab pdf --scale <n>                # Print scale
pinchtab pdf --paper-width <in>         # Paper width in inches
pinchtab pdf --paper-height <in>        # Paper height in inches
pinchtab pdf --page-ranges <r>          # Page ranges such as 1-3
pinchtab pdf --prefer-css-page-size     # Use CSS page size
pinchtab pdf --display-header-footer    # Show header/footer
pinchtab download <url>                 # Download through the browser session
pinchtab download <url> -o <path>       # Save downloaded file to a path
pinchtab upload <file>                  # Upload to the default file input
pinchtab upload <file> -s <css>         # Upload to a specific file input
pinchtab record start <file>            # Start recording (.webm, .mp4, .gif)
pinchtab record start <file> --fps 10   # Custom frame rate (default 5)
pinchtab record start <file> --quality 90 # JPEG capture quality (default 80)
pinchtab record start <file> --scale 0.5  # Half resolution
pinchtab record stop                    # Stop recording and save
pinchtab record status                  # Check recording status
```

## Storage And State

The key or state name is the first argument. `--key` / `--name` are accepted as the same
value for existing scripts; passing both the argument and the flag is refused.

```bash
pinchtab storage get                    # Both localStorage and sessionStorage for the tab's origin
pinchtab storage get --type local       # One store
pinchtab storage get <key>              # A single item (same as --key <key>)
pinchtab storage set <key> <value>      # Write an item (localStorage unless --type session)
pinchtab storage delete <key>           # Remove one key (same as --key <key>); a key is required
pinchtab storage clear                  # Wipe localStorage (--type session for sessionStorage)
pinchtab storage clear --all            # Wipe both stores
pinchtab state                          # Current browser state for the tab
pinchtab state list                     # List saved state files
pinchtab state save [name]              # Save cookies and storage (name auto-generated if omitted)
pinchtab state save <name> --encrypt    # Save encrypted (needs security.stateEncryptionKey)
pinchtab state load <name>              # Restore a saved state; <name> may be a prefix (newest match)
pinchtab state show <name>              # Print a saved state file
pinchtab state delete <name>            # Delete a saved state file
pinchtab state clean --older-than <h>   # Remove state files older than <h> hours (default 24)
pinchtab cookies get                    # Cookies for the tab's current page (--name, --url)
pinchtab cookies set <name> <value>     # Set a cookie (--domain, --path, --secure, --http-only, --same-site)
pinchtab cookies clear                  # Clear ALL browser cookies, every origin
```

`storage delete` never wipes a store: a bare `pinchtab storage delete` is refused and names
`storage clear`, the one wipe verb. Every storage verb takes `--tab <id>`, as do `state`,
`state save` and `state load`. The `state` family requires `security.allowStateExport`.

## Emulation

```bash
pinchtab set viewport <w> <h>           # Viewport size (--dpr, --mobile)
pinchtab set geo <lat> <lon>            # Geolocation (--accuracy)
pinchtab set media <feature> <value>    # CSS media feature, e.g. prefers-color-scheme dark
pinchtab set offline <true|false>       # Toggle offline mode
pinchtab set headers '<json>'           # Extra HTTP headers ({} clears them)
pinchtab set credentials <user> <pass>  # HTTP auth credentials
```

## Site Audit, Compare, And Scrape

```bash
pinchtab audit <url>                    # Browser-enriched page audit (see audit.md)
pinchtab compare <live-url> <staging-url>  # Visual + data diff of two site versions
pinchtab scrape <url>                   # HTTP crawl, browser-render only thin pages (see scrape.md)
```

See [audit.md](audit.md) and [scrape.md](scrape.md) for flags and report shapes.

## Instances, Profiles, And Activity

```bash
pinchtab instance list                  # List running instances
pinchtab instance start                 # Start an instance
pinchtab instance start --profile <id-or-name>
pinchtab instance start --mode headed
pinchtab instance start --port <n>
pinchtab instance start --extension /path/to/ext
pinchtab instance stop <id>             # Stop an instance
pinchtab instance restart <id>          # Soft-restart an instance's browser process
pinchtab instance logs <id>             # Show instance logs
pinchtab instance navigate <id> <url>   # Open a tab in an instance already on <url> (one step)
pinchtab profiles                       # List profiles
pinchtab profiles create <name>         # Create a profile for human setup and login
pinchtab profiles prune                 # List reclaimable quarantined profiles (removes nothing)
pinchtab profiles prune --confirm       # Remove them and report the disk freed
pinchtab profiles prune --profile <dir> # Reclaim just one quarantined directory
pinchtab activity                       # List recorded activity events
pinchtab activity tab <tab-id>          # Filter activity by tab
pinchtab health                         # Check server health
```

### Reclaiming quarantined profiles

When a profile's browser data becomes unreadable, PinchTab renames the directory to
`<profile>.quarantine-<timestamp>` and starts the profile again from an empty one. Those
directories are never read afterwards, so they are pure disk cost.

Two things remove them, and they answer different questions:

- The **automatic prune** bounds accumulation *per profile, at quarantine time*. Each time a
  profile is quarantined, older quarantined copies **of that same profile** are removed,
  keeping `profiles.quarantineKeep` of them (default 1). It only ever runs as a side effect
  of a new quarantine, so a profile that is quarantined once and never again keeps its copy
  indefinitely, and `profiles.quarantineKeep: 0` — the documented way to keep everything —
  switches it off entirely.
- **`pinchtab profiles prune`** reclaims *on demand, across all profiles*. It is the answer
  to "give me the disk back now", including for quarantines the automatic prune will never
  revisit. It ignores `quarantineKeep` completely, so keeping everything automatically still
  leaves an explicit way to reclaim.

Nothing is scheduled and nothing runs at startup; the on-demand path only runs when you ask.

The bare command is a dry run — it prints what it would remove and the total it would free,
and deletes nothing, so it is safe for an agent to run:

```bash
$ pinchtab profiles prune
default.quarantine-1748100001	412.6 MB
work.quarantine-1748100002	1.1 GB

2 quarantined profile(s), 1.5 GB reclaimable. Nothing was removed; re-run with --confirm.
```

Eligibility is the quarantine name pattern `<profile>.quarantine-<timestamp>`, not a record
of what PinchTab actually quarantined — so a profile you created under a name of that shape
is eligible too, and it is listed as quarantined everywhere else as well. The bare dry run
is where you see that before anything is removed. Nothing outside the pattern is removed,
whatever you pass. `--profile` names a quarantined directory, never a filesystem path — a
path is refused. To delete an ordinary profile, use the profile delete route instead; this
command cannot reach one.

Over HTTP the same operation is `POST /profiles/prune`, with `{"confirm": true}` to remove
and an optional `"profile"` to narrow it. Every removal is logged with its path and the
bytes it freed.

## Configuration And Security

```bash
pinchtab config                         # Interactive config overview/editor
pinchtab config init                    # Create a default config file
pinchtab config show                    # Print effective runtime config
pinchtab config token                   # Copy server.token to the clipboard without printing it
pinchtab config token --stdout          # Print server.token to stdout (headless hosts, $(...) capture)
pinchtab config path                    # Print config file path
pinchtab config validate                # Validate the current config file
pinchtab config schema                  # Print the config JSON Schema URL (--print for the schema)
pinchtab config get <path>              # Read one file-config value
pinchtab config set <path> <val>        # Set one file-config value
pinchtab config patch <json>            # Merge JSON into the config file
pinchtab security                       # Interactive security overview
pinchtab security up                    # Apply stricter defaults
pinchtab security down                  # Apply documented guards-down preset
pinchtab doctor                         # Read-only install and browser checks (--json, --check <name>)
pinchtab doctor browsers                # Configured and known browsers with availability
pinchtab doctor browser [name]          # Browser availability, or checks for one target
pinchtab version                        # Print the PinchTab version
```

## Global Flags

The root command supports:

```bash
PINCHTAB_TOKEN=<that-host-token> pinchtab --server http://host:9867 <command>
pinchtab --help
pinchtab --version
```

A non-loopback `--server` host requires its credential in `PINCHTAB_TOKEN` (or `PINCHTAB_SESSION`) on the same command — the CLI refuses to send the local config's `server.token` off the machine.

Commands with `--tab` currently include:

- `nav`
- `back`
- `forward`
- `reload`
- `snap`
- `screenshot`
- `capture`
- `pdf`
- `find`
- `extract`
- `text`
- `click`
- `dblclick`
- `hover`
- `mouse move`
- `mouse down`
- `mouse up`
- `mouse wheel`
- `focus`
- `type`
- `press`
- `fill`
- `scroll`
- `select`
- `eval`
- `check`
- `uncheck`
- `keyboard type`
- `keyboard inserttext`
- `keydown`
- `keyup`
- `scrollintoview`
- `network`
- `wait`
- `dialog accept`
- `dialog dismiss`
- `console`
- `errors`
- `frame`, `html`, `styles`, `title`, `url`
- `value`, `attr`, `box`, `checked`, `enabled`, `visible`, `count`
- `drag`, `download`, `upload`, `annotate`
- `a11y audit`, `memory`, `memory snapshot`, `record start`
- `network route`, `network unroute`, `network rules`
- `cookies get`, `cookies set`, `set` (every subcommand)
- `storage` (every subcommand), `state`, `state save`, `state load`

## Output Format

Most commands output human-readable text by default. Use `--json` for machine-parseable JSON output:

```bash
pinchtab tab                            # Human-readable: *abc123  https://...  Page Title
pinchtab tab --json                     # JSON: {"tabs":[...]}
pinchtab frame                          # Human-readable: main
pinchtab frame --json                   # JSON: {"tabId":"...","scoped":false,...}
pinchtab network                        # Human-readable: GET  200  https://...
pinchtab network --json                 # JSON: {"entries":[...],"count":5}
```

**For scripts and automation**: Always use `--json` when piping output or parsing programmatically. Human-readable formats may change between versions and are not guaranteed to be stable. The JSON schema is the stable contract.

Commands with `--json` include: `tab`, `frame`, `network`, `click`, `type`, `scroll`, `nav`, `back`, `forward`, `reload`, `wait`, `find`, `extract`, `eval`, and most action commands.
