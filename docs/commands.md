# Commands Reference

## Server & Daemon

```
pinchtab server                  # Start the full server (dashboard + API)
pinchtab server --extension /path/to/ext # Start with extension (repeatable)
pinchtab bridge                  # Start bridge-only server (no dashboard)
pinchtab mcp                     # Start the MCP stdio server
pinchtab daemon                  # Show daemon status
pinchtab daemon install          # Install as background service
pinchtab daemon start            # Start the background service
pinchtab daemon stop             # Stop the background service
pinchtab daemon restart          # Restart the background service
pinchtab daemon uninstall        # Remove the background service
```

## Navigation

```
pinchtab nav <url>               # Navigate to URL in current tab
pinchtab nav <url> --new-tab     # Navigate in a new tab
pinchtab nav <url> --tab <id>    # Navigate a specific tab
pinchtab nav <url> --block-images # Navigate with image blocking
pinchtab nav <url> --block-ads   # Navigate with ad blocking
pinchtab quick <url>             # Navigate + snapshot accessibility tree
```

```
pinchtab back                    # Go back in browser history
pinchtab back --tab <id>         # Go back in specific tab
pinchtab forward                 # Go forward in browser history
pinchtab forward --tab <id>      # Go forward in specific tab
pinchtab reload                  # Reload current page
pinchtab reload --tab <id>       # Reload specific tab
```

Hidden aliases: `goto`, `navigate`, `open`

## Tab Management

```
pinchtab tab                     # List all tabs
pinchtab tab <id>                # Focus tab by ID
pinchtab tab new                 # Open a new empty tab
pinchtab tab new <url>           # Open a new tab with URL
pinchtab tab close <id>          # Close tab by ID
```

Alias: `tabs`

Numeric arguments are resolved as 1-based tab indices via `/tabs`. Non-numeric arguments are passed through as tab IDs.

## Interaction

```
pinchtab click <ref>             # Click element by ref
pinchtab click --css <selector>  # Click element by CSS selector
pinchtab click --wait-nav <ref>  # Click and wait for navigation
pinchtab dblclick <ref>          # Double-click element by ref
pinchtab dblclick --css <selector> # Double-click element by CSS selector
pinchtab type <ref> <text>       # Type into element
pinchtab fill <ref|selector> <text> # Fill input directly (no keystroke events)
pinchtab press <key>             # Press key (Enter, Tab, Escape...)
pinchtab hover <ref>             # Hover over element
pinchtab hover --css <selector>  # Hover by CSS selector
pinchtab select <ref> <value>    # Select dropdown option
pinchtab scroll <ref|pixels>     # Scroll to element or by pixel amount
```

## Page Analysis

```
pinchtab snap                    # Snapshot accessibility tree
pinchtab snap -i                 # Interactive elements only
pinchtab snap -c                 # Compact output
pinchtab snap -d                 # Diff from previous snapshot
pinchtab snap --selector <css>   # Scope to CSS selector
pinchtab snap --max-tokens <n>   # Limit token budget
pinchtab snap --depth <n>        # Limit tree depth
pinchtab snap --text             # Text output format
pinchtab text                    # Extract page text (markdown)
pinchtab text --raw              # Raw text extraction
pinchtab find <query>            # Find elements by natural language
pinchtab find --threshold <0-1>  # Minimum similarity score
pinchtab find --explain          # Show score breakdown
pinchtab find --ref-only         # Output just the element ref
pinchtab eval <expression>       # Evaluate JavaScript
```

## Capture & Export

```
pinchtab screenshot              # Take a screenshot (JPEG)
pinchtab screenshot -o <path>    # Save to specific path (format based on extension)
pinchtab screenshot -q <0-100>   # Set JPEG quality
pinchtab pdf                     # Export page as PDF
pinchtab pdf -o <path>           # Save PDF to path
pinchtab pdf --landscape         # Landscape orientation
pinchtab pdf --scale <n>         # Page scale (e.g. 0.5)
pinchtab pdf --paper-width <in>  # Paper width in inches
pinchtab pdf --paper-height <in> # Paper height in inches
pinchtab pdf --page-ranges <r>   # Page ranges (e.g. 1-3)
pinchtab download <url>          # Download a file
pinchtab download <url> -o <path> # Download to specific path
pinchtab upload <file>           # Upload a file
pinchtab upload <file> -s <css>  # Upload to specific file input
```

## Instances & Profiles

```
pinchtab instances               # List running instances
pinchtab instance start          # Start a new browser instance
pinchtab instance start --profile <name> # Start with specific profile
pinchtab instance start --port <n> # Start on specific port
pinchtab instance start --extension /path/to/ext # Load extension (repeatable)
pinchtab instance stop <id>      # Stop an instance
pinchtab instance logs <id>      # View instance logs
pinchtab instance navigate <id> <url>  # Navigate instance to URL
pinchtab profiles                # List browser profiles
pinchtab health                  # Check server health
```

## Configuration

```
pinchtab config show             # Show current configuration
pinchtab config init             # Create default config file
pinchtab config path             # Show config file path
pinchtab config validate         # Validate config file
pinchtab config get <path>       # Get a config value
pinchtab config set <path> <val> # Set a config value
pinchtab config patch <json>     # Patch config with JSON
```

## Security

```
pinchtab security                # Review security posture
pinchtab security up             # Apply recommended security defaults
pinchtab security down           # Relax security settings
```

## Global Flags

Most browser commands support `--tab <id>` to target a specific tab.

Commands with `--tab`: nav, snap, click, dblclick, type, fill, press, hover, scroll, select, eval, screenshot, pdf, find, text

```
pinchtab <command> --tab <id>    # Run command against specific tab
pinchtab --help                  # Show help
pinchtab --version               # Show version
```
