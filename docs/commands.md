# Commands Reference

## Server And Runtime

```bash
pinchtab server                         # Start the full server (dashboard + API)
pinchtab bridge                         # Start the bridge-only runtime
pinchtab mcp                            # Start the MCP stdio server
pinchtab daemon                         # Show daemon status
pinchtab daemon install                 # Install as a background service
pinchtab daemon start                   # Start the background service
pinchtab daemon stop                    # Stop the background service
pinchtab daemon restart                 # Restart the background service
pinchtab daemon uninstall               # Remove the background service
pinchtab completion <shell>             # Generate shell completions
```

## Navigation

`pinchtab nav <url>` uses `/navigate`. When you do not pass `--tab`, PinchTab opens a new tab and navigates it.

```bash
pinchtab nav <url>                      # Open a new tab and navigate it
pinchtab nav <url> --tab <id>           # Reuse a specific tab
pinchtab nav <url> --new-tab            # Explicitly force a new tab
pinchtab nav <url> --block-images       # Block images for this navigation
pinchtab nav <url> --block-ads          # Block ads for this navigation
pinchtab nav <url> --snap               # Navigate and output interactive snapshot
pinchtab back                           # Go back in the active tab
pinchtab back --tab <id>                # Go back in a specific tab
pinchtab forward                        # Go forward in the active tab
pinchtab reload                         # Reload the active tab
```

Use `--tab <id>` when you intend to replace an existing tab's URL. Unscoped reads and actions use the current tracked tab, but unscoped `nav` intentionally opens a new tab to avoid replacing active work by accident.

Hidden aliases: `goto`, `navigate`, `open`

## Tabs

The `tab` command only lists, focuses, creates, and closes tabs. It does not proxy the rest of the browser command set.

```bash
pinchtab tab                            # List tabs
pinchtab tab <id>                       # Focus a tab by ID or 1-based index
pinchtab tab new                        # Open a blank tab
pinchtab tab new <url>                  # Open a tab and navigate it
pinchtab tab close <id>                 # Close a tab
```

Use top-level commands with `--tab` for tab-scoped work:

```bash
pinchtab snap --tab <id>
pinchtab click --tab <id> <selector>
pinchtab pdf --tab <id> -o page.pdf
```

## Interaction

Most element commands accept a unified selector:

- snapshot ref such as `e5`
- CSS selector such as `#login`
- XPath such as `xpath://button`
- text selector such as `text:Submit`
- semantic selector such as `find:login button`

Selector lookup is explicit by frame. Unscoped selectors search only the current frame scope, which defaults to `main`. Use `pinchtab frame ...` before selector-based iframe work. Same-origin iframe scopes are supported; cross-origin iframe descendants are not currently exposed.

```bash
pinchtab frame                         # Show current frame scope
pinchtab frame "#payment-frame"        # Scope selectors to an iframe
pinchtab frame main                    # Return selector scope to the top document
pinchtab click [selector]               # Click an element or coordinates with --x/--y
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
pinchtab snap                           # Accessibility snapshot
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
pinchtab network-export                 # Export as HAR 1.2 (saved to exports/)
pinchtab network-export -o session.har  # Export to specific file
pinchtab network-export --format ndjson # Export as NDJSON (one entry per line)
pinchtab network-export --body          # Include response bodies
pinchtab network-export --stream -o l.har # Live capture to file while browsing
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
pinchtab screenshot -o <path>           # Save screenshot to a chosen path
pinchtab screenshot -q <0-100>          # JPEG quality
pinchtab screenshot -s <selector>       # Capture a specific element by selector
pinchtab screenshot -s <selector> --css-1x # Export selector screenshot at CSS pixel size
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
```

## Instances, Profiles, And Activity

```bash
pinchtab instances                      # List running instances
pinchtab instance start                 # Start an instance
pinchtab instance start --profile <id-or-name>
pinchtab instance start --mode headed
pinchtab instance start --port <n>
pinchtab instance start --extension /path/to/ext
pinchtab instance stop <id>             # Stop an instance
pinchtab instance logs <id>             # Show instance logs
pinchtab instance navigate <id> <url>   # Open a tab in an instance and navigate it
pinchtab profiles                       # List profiles
pinchtab activity                       # List recorded activity events
pinchtab activity tab <tab-id>          # Filter activity by tab
pinchtab health                         # Check server health
```

## Configuration And Security

```bash
pinchtab config                         # Interactive config overview/editor
pinchtab config init                    # Create a default config file
pinchtab config show                    # Print effective runtime config
pinchtab config token                   # Copy server.token to the clipboard without printing it
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
pinchtab --server http://host:9867 <command>
pinchtab --help
pinchtab --version
```

Commands with `--tab` currently include:

- `nav`
- `back`
- `forward`
- `reload`
- `snap`
- `screenshot`
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
- `network-export`
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
