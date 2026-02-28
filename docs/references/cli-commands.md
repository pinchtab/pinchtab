# Pinchtab CLI Commands

Complete reference for all `pinchtab` CLI commands. Requires a running Pinchtab server (except for `help`).

## Quick Start

**Terminal 1: Start the server**
```bash
pinchtab
```

**Terminal 2: Use CLI commands**
```bash
pinchtab quick https://example.com    # Navigate + analyze page
```

## Server Control

Start Pinchtab in different modes.

### Start Server
```bash
pinchtab
```
Launches Pinchtab server on `http://127.0.0.1:9867` (default).

**Options:**
```bash
BRIDGE_PORT=9868 pinchtab              # Custom port
BRIDGE_HEADLESS=false pinchtab         # Visible window
BRIDGE_PROFILE=/path/to/profile pinchtab  # Custom profile
BRIDGE_TOKEN=secret pinchtab           # Enable auth
```

### Dashboard (Profile Manager)
```bash
pinchtab dashboard
```
Run the orchestrator and profile manager (no browser).

### Connect to Running Profile
```bash
pinchtab connect <name>
```
Get the URL for a running Chrome profile.

---

## CLI Commands (Require Running Server)

### Quick Start Command

#### Navigate + Analyze Page
```bash
pinchtab quick <url>
```
**New beginner-friendly command that combines navigation and page analysis in one step.**

- Navigates to the URL
- Waits for page to load
- Shows page structure with clickable elements
- Suggests next actions (click, type, screenshot, PDF)

**Example:**
```bash
$ pinchtab quick https://example.com
🦀 Navigating to https://example.com ...

📋 Page structure:
[accessible tree output]

📌 Title: Example Domain
🔗 URL: https://example.com/

💡 Quick actions:
  pinchtab click <ref>        # Click an element (use refs from above)
  pinchtab type <ref> <text>  # Type into input field
  pinchtab screenshot         # Take a screenshot
  pinchtab pdf -o output.pdf  # Save as PDF
```

---

### Navigation

#### Navigate to URL
```bash
pinchtab nav <url>
pinchtab navigate <url>         # Alias
```

**Options:**
```bash
pinchtab nav https://example.com --new-tab
pinchtab nav https://example.com --block-images
pinchtab nav https://example.com --block-ads
```

**Flags:**
- `--new-tab` — Open in new tab instead of current
- `--block-images` — Don't load images
- `--block-ads` — Block 100+ tracking/ad domains

**Example:**
```bash
pinchtab nav https://example.com
# Returns: {"title": "Example Domain", "url": "https://example.com"}
```

---

### Snapshots & DOM

#### Get Accessibility Tree
```bash
pinchtab snap
pinchtab snapshot              # Alias
```

**Options:**
```bash
pinchtab snap -i               # Interactive only (buttons, links, inputs)
pinchtab snap -c               # Compact format (most token-efficient)
pinchtab snap -d               # Diff only (changes since last snapshot)
pinchtab snap -i -c            # Combine flags
pinchtab snap -s "main > form" # CSS selector scope
pinchtab snap --max-tokens 1000  # Truncate to ~1000 tokens
pinchtab snap --depth 3        # Max tree depth
pinchtab snap --tab <tabId>    # Target specific tab
```

**Flags:**
- `-i, --interactive` — Only buttons, links, inputs (faster, fewer tokens)
- `-c, --compact` — Compact JSON format (most token-efficient for AI)
- `-d, --diff` — Only elements that changed since last snapshot
- `-s, --selector CSS` — Scope to CSS selector
- `--max-tokens N` — Truncate output to ~N tokens
- `--depth N` — Max tree depth
- `--tab ID` — Target specific tab (default: first tab)

**Output Structure:**
```json
{
  "elements": [
    {
      "ref": "e0",
      "role": "heading",
      "name": "Example Domain",
      "visible": true
    },
    {
      "ref": "e1",
      "role": "link",
      "name": "More information...",
      "visible": true
    }
  ]
}
```

---

### Interaction

#### Click Element
```bash
pinchtab click <ref>
```

**Example:**
```bash
pinchtab click e5          # Click element with ref e5
```

#### Type Text
```bash
pinchtab type <ref> <text>
```

**Example:**
```bash
pinchtab type e12 "hello world"
pinchtab type e3 "user@example.com"
```

#### Fill Input (Direct)
```bash
pinchtab fill <ref|selector> <text>
```
Sets input value directly without triggering key events.

**Example:**
```bash
pinchtab fill e5 "value"
pinchtab fill "input[name='email']" "user@example.com"
```

#### Press Key
```bash
pinchtab press <key>
```

**Keys:** `Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, etc.

**Example:**
```bash
pinchtab press Enter
pinchtab press Tab
pinchtab press Escape
```

#### Hover Element
```bash
pinchtab hover <ref>
```

**Example:**
```bash
pinchtab hover e5
```

#### Focus Element
```bash
pinchtab focus <ref>
```

**Example:**
```bash
pinchtab focus e5
```

#### Scroll
```bash
pinchtab scroll <ref|pixels>
```

Scroll to element or by pixel amount.

**Example:**
```bash
pinchtab scroll e5         # Scroll to element
pinchtab scroll 500        # Scroll down 500px
```

#### Select Dropdown
```bash
pinchtab select <ref> <value>
```

**Example:**
```bash
pinchtab select e7 "Option 2"
```

---

### Content Extraction

#### Extract Text
```bash
pinchtab text
pinchtab text --raw        # Raw innerText (no readability)
```

**Modes:**
- Default — Readability extraction (strips nav, ads, sidebars)
- `--raw` — Raw `innerText` (full page text)

**Example:**
```bash
pinchtab text              # Returns readable article/content
pinchtab text --raw        # Returns all text on page
pinchtab text | jq .text   # Pipe output to jq
```

#### Evaluate JavaScript
```bash
pinchtab eval <expression>
pinchtab evaluate <expression>  # Alias
```

**Escape hatch to run any JavaScript.**

**Example:**
```bash
pinchtab eval "document.title"
pinchtab eval "document.querySelectorAll('a').length"
pinchtab eval "localStorage.getItem('token')"
```

---

### Capture & Export

#### Screenshot
```bash
pinchtab ss
pinchtab screenshot        # Alias
```

**Options:**
```bash
pinchtab ss -o screenshot.jpg
pinchtab ss -q 85          # Quality 0-100 (default: 90)
pinchtab ss -o page.jpg -q 80
```

**Flags:**
- `-o, --output FILE` — Save to file (default: stdout)
- `-q, --quality N` — JPEG quality 0-100 (default: 90)

**Example:**
```bash
pinchtab ss -o page.jpg
pinchtab ss -o page.jpg -q 75
```

#### Export PDF
```bash
pinchtab pdf
pinchtab pdf -o output.pdf
```

**Basic Options:**
```bash
pinchtab pdf -o output.pdf
pinchtab pdf -o report.pdf --landscape
pinchtab pdf -o report.pdf --scale 1.5
pinchtab pdf -o report.pdf --page-ranges "1-3,5"
```

**Paper & Margins:**
```bash
pinchtab pdf --paper-width 8.5 --paper-height 11   # US Letter (default)
pinchtab pdf --paper-width 210 --paper-height 297  # A4 (in mm)
pinchtab pdf --margin-top 0.5 --margin-bottom 0.5
pinchtab pdf --margin-left 1.0 --margin-right 1.0
pinchtab pdf --prefer-css-page-size               # Honor CSS @page size
```

**Headers & Footers:**
```bash
pinchtab pdf --display-header-footer
pinchtab pdf --header-template "<div>Page <span class='pageNumber'></span></div>"
pinchtab pdf --footer-template "<div>© 2024</div>"
```

**Accessibility:**
```bash
pinchtab pdf --generate-tagged-pdf           # Create accessible PDF
pinchtab pdf --generate-document-outline     # Add document outline/bookmarks
```

**File Output:**
```bash
pinchtab pdf --file-output              # Save to server disk
pinchtab pdf --file-output --path /tmp/output.pdf
```

**All Options:**
```bash
pinchtab pdf -o output.pdf \
  --landscape \
  --paper-width 8.5 \
  --paper-height 11 \
  --margin-top 0.5 \
  --margin-bottom 0.5 \
  --margin-left 1.0 \
  --margin-right 1.0 \
  --scale 1.0 \
  --page-ranges "1-5" \
  --prefer-css-page-size \
  --display-header-footer \
  --header-template "<h1>Report</h1>" \
  --footer-template "<p>Page <span class='pageNumber'></span></p>" \
  --generate-tagged-pdf \
  --generate-document-outline \
  --tab tabId123
```

---

### Tab Management

#### List Tabs
```bash
pinchtab tabs
pinchtab tab               # Alias
```

**Returns:** JSON array of open tabs with `tabId`, `title`, `url`.

**Example:**
```bash
$ pinchtab tabs
[
  {
    "tabId": "abc123",
    "title": "Example Domain",
    "url": "https://example.com"
  }
]
```

#### Open New Tab
```bash
pinchtab tabs new <url>
pinchtab tab new <url>     # Alias
```

**Example:**
```bash
pinchtab tabs new https://google.com
```

#### Close Tab
```bash
pinchtab tabs close <tabId>
pinchtab tab close <tabId>  # Alias
```

**Example:**
```bash
pinchtab tabs close abc123
```

---

### Server & Help

#### Health Check
```bash
pinchtab health
```

**Example:**
```bash
$ pinchtab health
{
  "status": "ok",
  "version": "0.7.6",
  "uptime": 3600
}
```

#### Show Help
```bash
pinchtab help
```

Displays all commands and options.

---

## Environment Variables

Configure Pinchtab behavior via environment variables.

### Client Variables
Used by CLI commands:

```bash
PINCHTAB_URL=http://localhost:9867     # Server URL (default: http://127.0.0.1:9867)
PINCHTAB_TOKEN=your-secret-token       # Auth token (sent as Bearer)
```

### Server Variables
Used when running `pinchtab` server:

```bash
BRIDGE_PORT=9867                       # Server port (default: 9867)
BRIDGE_BIND=0.0.0.0                    # Bind address (default: 127.0.0.1)
BRIDGE_HEADLESS=true                   # Headless mode (default: true)
BRIDGE_PROFILE=/path/to/profile        # Chrome profile directory
BRIDGE_TOKEN=your-secret-token         # API auth token
BRIDGE_BLOCK_ADS=true                  # Block 100+ ad domains
BRIDGE_NO_RESTORE=false                # Don't restore tabs on startup
CDP_URL=http://localhost:9222          # Connect to external Chrome (instead of launching)
```

---

## Examples

### Basic Workflow
```bash
# Terminal 1: Start server
pinchtab

# Terminal 2: Interact
pinchtab nav https://example.com
pinchtab snap -i -c                    # See interactive elements
pinchtab click e5                       # Click element
pinchtab snap                          # Verify change
pinchtab screenshot -o result.jpg      # Capture result
```

### Form Filling
```bash
pinchtab nav https://example.com/form
pinchtab snap -i -c                    # Find input refs
pinchtab click e1                      # Focus first field
pinchtab type e1 "John Doe"            # Type name
pinchtab click e2                      # Focus email field
pinchtab type e2 "john@example.com"    # Type email
pinchtab click e5                      # Click submit
pinchtab snap                          # Verify submission
```

### Scraping with Text Extraction
```bash
pinchtab nav https://example.com/article
pinchtab text > article.txt            # Extract text
pinchtab text | jq .text               # Parse JSON
```

### PDF Generation
```bash
pinchtab nav https://example.com/report
pinchtab pdf -o report.pdf \
  --landscape \
  --page-ranges "1-5" \
  --display-header-footer \
  --header-template "<h1>Report</h1>"
```

### Multi-Tab Automation
```bash
pinchtab nav https://google.com
pinchtab tabs new https://github.com
pinchtab tabs                          # List both tabs
pinchtab snap --tab tab2               # Snapshot second tab
```

### With Authentication
```bash
PINCHTAB_TOKEN=secret-token pinchtab nav https://api.example.com
PINCHTAB_URL=http://192.168.1.100:9867 pinchtab snap
```

### Piping with jq
```bash
# Get all links on page
pinchtab snap -i -c | jq '.elements[] | select(.role=="link") | {ref, name}'

# Extract readable text as JSON
pinchtab text | jq '.text'

# List all tabs
pinchtab tabs | jq '.[] | {tabId, title, url}'
```

---

## Tips

### Token Efficiency
For AI agents, use flags to reduce token usage:

```bash
pinchtab snap -i -c --max-tokens 1000    # Interactive, compact, truncated
pinchtab text --raw                      # Raw text instead of readability
```

### Error Messages
If server isn't running:
```
❌ Pinchtab server is not running on http://localhost:9867

To start the server:
  pinchtab                    # Run in foreground
  pinchtab &                  # Run in background
```

### Headless vs Headed
```bash
pinchtab                              # Headless (default, no window)
BRIDGE_HEADLESS=false pinchtab        # Headed (visible window)
```

### Profile Management
```bash
BRIDGE_PROFILE=~/.pinchtab/profile1 pinchtab    # Custom profile
BRIDGE_NO_RESTORE=true pinchtab                 # Start fresh
```

---

## Command Reference (Alphabetical)

| Command | Description |
|---------|-------------|
| `click <ref>` | Click element |
| `connect <name>` | Get URL for running profile |
| `dashboard` | Start profile manager |
| `eval <expr>` | Evaluate JavaScript |
| `fill <ref\|selector> <text>` | Set input value |
| `focus <ref>` | Focus element |
| `health` | Check server status |
| `help` | Show help text |
| `hover <ref>` | Hover over element |
| `nav <url>` | Navigate to URL |
| `pdf [-o file]` | Export page as PDF |
| `press <key>` | Press key |
| `quick <url>` | Navigate + analyze (quick start) |
| `screenshot` / `ss` | Take screenshot |
| `scroll <ref\|px>` | Scroll page |
| `select <ref> <val>` | Select dropdown |
| `snap` / `snapshot` | Get accessibility tree |
| `tab[s]` | List/manage tabs |
| `text` | Extract readable text |
| `type <ref> <text>` | Type into element |

---

## Aliases

| Command | Alias |
|---------|-------|
| `nav` | `navigate` |
| `snap` | `snapshot` |
| `ss` | `screenshot` |
| `eval` | `evaluate` |
| `tab` | `tabs` |

---

**For more help:** `pinchtab help`
