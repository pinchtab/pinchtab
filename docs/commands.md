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

`back`, `forward` and `reload` always report the URL the page actually landed on,
so a redirect, a login wall or an error page is visible without `--snap`.

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
pinchtab text --frame <frameId>         # Read text from one iframe
pinchtab find <query>                   # Semantic element search
pinchtab find --threshold <0-1>         # Minimum similarity score
pinchtab find --explain                 # Include score breakdown
pinchtab find --ref-only                # Print only the best ref
pinchtab eval <expression>              # Evaluate JavaScript
```

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
pinchtab wait <selector>                # Wait for selector to be visible
pinchtab wait <selector> --state hidden # Wait for selector to disappear
pinchtab wait <ms>                      # Fixed duration sleep (escape hatch; max 30000ms — prefer condition-based waits)
pinchtab wait --text <text>             # Wait for page text to appear
pinchtab wait --not-text <text>         # Wait for page text to disappear
pinchtab wait --url <glob>              # Wait for URL match (glob: **, *, ?)
pinchtab wait --load <state>            # state: ready-state | content-loaded | network-idle
                                        #   ready-state    → document.readyState === 'complete'
                                        #   content-loaded → readyState in {interactive, complete}
                                        #   network-idle   → 0 in-flight requests for 500ms (override with --idle-for)
pinchtab wait --fn <expression>         # Wait for JS to become truthy
pinchtab wait ... --timeout <ms>        # Override timeout (default 10000, max 30000)
pinchtab network                        # List captured network requests
pinchtab network <requestId>            # Show one request in detail
pinchtab network --stream               # Stream network entries
pinchtab network --clear                # Clear captured network data
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

Manual handoff and resume are available via CLI and API:

```bash
pinchtab tab handoff <tabId> --reason captcha --timeout-ms 120000
pinchtab tab handoff-status <tabId>
pinchtab tab resume <tabId> --status completed
```

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

## Instances, Profiles, And Activity

```bash
pinchtab instance list                  # List running instances
pinchtab instance start                 # Start an instance
pinchtab instance start --profile <id-or-name>
pinchtab instance start --mode headed
pinchtab instance start --port <n>
pinchtab instance start --extension /path/to/ext
pinchtab instance stop <id>             # Stop an instance
pinchtab instance logs <id>             # Show instance logs
pinchtab instance navigate <id> <url>   # Open a tab in an instance and navigate it
pinchtab profiles                       # List profiles
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
pinchtab config get <path>              # Read one file-config value
pinchtab config set <path> <val>        # Set one file-config value
pinchtab config patch <json>            # Merge JSON into the config file
pinchtab security                       # Interactive security overview
pinchtab security up                    # Apply stricter defaults
pinchtab security down                  # Apply documented guards-down preset
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

Commands with `--json` include: `tab`, `frame`, `network`, `click`, `type`, `scroll`, `nav`, `back`, `forward`, `reload`, `wait`, `find`, `eval`, and most action commands.
