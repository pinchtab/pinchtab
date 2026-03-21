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
pinchtab quick <url>                    # Navigate and take a snapshot
pinchtab back                           # Go back in the active tab
pinchtab back --tab <id>                # Go back in a specific tab
pinchtab forward                        # Go forward in the active tab
pinchtab reload                         # Reload the active tab
```

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

```bash
pinchtab click [selector]               # Click an element or coordinates with --x/--y
pinchtab click --css <selector>         # Force CSS selector mode
pinchtab click --wait-nav <selector>    # Click and wait for navigation
pinchtab dblclick [selector]            # Double-click
pinchtab type <selector> <text>         # Type via key events
pinchtab fill <selector> <text>         # Fill directly
pinchtab press <key>                    # Press a key
pinchtab hover [selector]               # Hover an element
pinchtab focus [selector]               # Focus an element
pinchtab scroll <selector|pixels>       # Scroll an element or the page
pinchtab select <selector> <value>      # Select a <select> option
pinchtab check <selector>               # Check a checkbox or radio
pinchtab uncheck <selector>             # Uncheck a checkbox or radio
pinchtab scrollintoview <selector>      # Scroll an element into view
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
pinchtab text --raw                     # Raw extraction
pinchtab find <query>                   # Semantic element search
pinchtab find --threshold <0-1>         # Minimum similarity score
pinchtab find --explain                 # Include score breakdown
pinchtab find --ref-only                # Print only the best ref
pinchtab eval <expression>              # Evaluate JavaScript
```

## Keyboard, Wait, And Diagnostics

```bash
pinchtab keyboard type <text>           # Type at the focused element
pinchtab keyboard inserttext <text>     # Insert text without key events
pinchtab keydown <key>                  # Hold a key down
pinchtab keyup <key>                    # Release a key
pinchtab wait <selector|ms>             # Wait for selector or fixed duration
pinchtab wait --text <text>             # Wait for page text
pinchtab wait --url <glob>              # Wait for URL match
pinchtab wait --load networkidle        # Wait for load state
pinchtab wait --fn <expression>         # Wait for JS to become truthy
pinchtab network                        # List captured network requests
pinchtab network <requestId>            # Show one request in detail
pinchtab network --stream               # Stream network entries
pinchtab network --clear                # Clear captured network data
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
```

## Capture And Export

```bash
pinchtab screenshot                     # Save a screenshot to a generated .jpg path
pinchtab screenshot -o <path>           # Save screenshot to a chosen path
pinchtab screenshot -q <0-100>          # JPEG quality
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
pinchtab config path                    # Print config file path
pinchtab config validate                # Validate the current config file
pinchtab config get <path>              # Read one file-config value
pinchtab config set <path> <val>        # Set one file-config value
pinchtab config patch <json>            # Merge JSON into the config file
pinchtab security                       # Interactive security overview
pinchtab security up                    # Apply stricter defaults
pinchtab security down                  # Relax defaults
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
