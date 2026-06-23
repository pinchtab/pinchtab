# CLI Commands Reference — PinchTab

> **Quick tip:** Use `pinchtab help` or `pinchtab <command> --help` for full flag lists.

---

## Control Plane

### `pinchtab server`
Start the PinchTab server (default port 9867).

```bash
pinchtab server
pinchtab server -y              # guards down (enables evaluate, macro, download, cookies)
pinchtab server -H              # visible browser for debugging
pinchtab server -yH             # both combined
pinchtab server -e ./ext        # load browser extension
```

| Flag | Short | Description |
|------|-------|-------------|
| `--yolo` | `-y` | Apply guards down preset (enables evaluate, macro, download, cookies) |
| `--headed` | `-H` | Start browser in headed (visible) mode |
| `--extension <path>` | `-e` | Load browser extension (repeatable) |

> **Note:** Use `--headed` only when you need visual feedback (debugging, watching automation). Headless mode is more resource-efficient.

### `pinchtab daemon`
Manage the user-level background service.

```bash
pinchtab daemon
pinchtab daemon install
pinchtab daemon start
pinchtab daemon stop
pinchtab daemon restart
```

### `pinchtab health`
Check if the server is running and healthy.

---

## Browser Commands

### `pinchtab nav <url>`
Navigate the current tracked tab to a URL, or create one when no current tab is available. This is the browser command that auto-starts the default local server when it is not already running. Without a session, `nav` uses a shared current tab — set `PINCHTAB_SESSION` first to get an isolated tab.

```bash
pinchtab nav https://pinchtab.com
pinchtab nav https://pinchtab.com --new-tab
pinchtab nav https://pinchtab.com --snap
pinchtab nav https://pinchtab.com --block-images
pinchtab nav https://pinchtab.com --tab <tabId>
```

| Flag | Description |
|------|-------------|
| `--new-tab` | Explicitly force a new tab |
| `--tab <id>` | Reuse a specific tab |
| `--snap` | Navigate and print an interactive compact snapshot |
| `--block-images` | Block image loading (faster, fewer tokens) |
| `--block-ads` | Block ads for this navigation |
| `--print-tab-id` | Print only the tab ID |

Only `http`/`https` URLs are accepted by default. `file://` (for opening a local HTML file) is rejected unless the server is started with `security.allowFileScheme` enabled — and even then it is blocked when a strict-mode domain allowlist is active, since `file://` has no host. `javascript:`, `chrome://`, and `data:` are always rejected.

### `pinchtab tab` (not `tabs`)
Manage browser tabs.

```bash
pinchtab tab                 # List all open tabs
pinchtab tab <tabId>         # Focus a tab by ID or 1-based index
pinchtab nav <url> --new-tab # Open a new tab and navigate it
pinchtab tab close <tabId>   # Close specific tab
```

Unscoped commands resolve the current tab by caller identity. Session-authenticated callers use a current tab scoped to that session; `--agent-id` / `PINCHTAB_AGENT_ID` callers use a current tab scoped to that agent when no session is present; anonymous CLI calls use the shared local current-tab state file.

---

## Interaction Commands

### `pinchtab click <ref>`
Click an element by its accessibility ref (from `snap`).

```bash
pinchtab click e5                # normal click path (omit --mode)
pinchtab click e5 --mode dom     # bypass occlusion with element.click()
pinchtab click e5 --mode dispatch # bypass occlusion with synthetic events
pinchtab click e5 --snap-diff    # click + return only changed elements
pinchtab click e5 --snap         # click + return full snapshot
pinchtab click e5 --tab <tabId>
```

### `pinchtab type <ref> <text>`
Type text into an input element.

```bash
pinchtab type e12 "hello world"
```

### `pinchtab fill <ref> <value>`
Fill a form field using JS event dispatch. Prefer over `type` for React/Vue/Angular forms.

```bash
pinchtab fill e12 "hello world"
pinchtab fill e12 "hello" --snap-diff    # fill + return only changed elements
```

### `pinchtab press <key>`
Press a named keyboard key.

```bash
pinchtab press Enter
pinchtab press Tab
pinchtab press Escape
```

### `pinchtab hover <ref>`
Hover over an element to trigger tooltips or hover styles.

### `pinchtab mouse move|down|up|wheel [ref]`
Low-level pointer controls for cases where DOM-native click or hover behavior is not enough.

```bash
pinchtab mouse move e5
pinchtab mouse move 120 220
pinchtab mouse down e5 --button left
pinchtab mouse down --button left
pinchtab mouse up e5 --button left
pinchtab mouse up --button left
pinchtab mouse wheel 240 --dx 40
pinchtab mouse move --x 400 --y 320
pinchtab drag e5 400,320
```

Use these for drag handles, canvas controls, precise hover choreography, or sites that require exact pointer sequencing.

### `pinchtab scroll <pixels|direction|selector>`
Scroll the page or a specific element. Argument is positional — there is no `--pixels` flag.

```bash
pinchtab scroll 800            # scroll page down 800px
pinchtab scroll -300           # scroll page up 300px
pinchtab scroll down           # named direction (also up, top, bottom)
pinchtab scroll '#footer'      # scroll a CSS selector into view
pinchtab scroll e20            # scroll an element ref into view
pinchtab scroll 800 --snap-diff
```

### `pinchtab select <ref> <value>`
Select an option from a `<select>` dropdown.

```bash
pinchtab select e8 "option-value"
pinchtab select e8 "value" --snap-diff    # select + return only changed elements
```

---

## Output Commands

### `pinchtab snap` (snapshot)
Get the accessibility tree of the current page. **Primary tool for understanding page state.**

```bash
pinchtab snap                   # compact interactive snapshot (default)
pinchtab snap "#main"           # scoped positional selector
pinchtab snap -s main           # scoped with --selector
pinchtab snap --full            # full JSON tree
pinchtab snap -d                # diff: only changes since last snap (prefer --snap-diff on actions)
pinchtab snap --max-tokens 2000 # token budget cap
```

> ⚠️ **Quirk:** Use `snap`, not `snapshot`. The alias `snap` is the intended short form.

### `pinchtab screenshot`
Capture a screenshot of the current page.

```bash
pinchtab screenshot
pinchtab screenshot --quality 80           # JPEG at 80%
pinchtab screenshot --beyond-viewport      # full scrollable page, not just the viewport
```

> ⚠️ **Quirk:** Use `screenshot` (full word), not `ss` or `shot`.

`--beyond-viewport` is ignored when `-s/--selector` is set — selectors already clip to an element.

### `pinchtab record`
Record browser activity as a video file.

```bash
pinchtab record start output.gif          # start recording (format from extension)
pinchtab record start output.gif --fps 2  # lower frame rate
pinchtab record stop                      # stop and save to the path given at start
pinchtab record status                    # check if recording is active
```

| Flag | Description |
|------|-------------|
| `--fps <n>` | Frames per second (default 5) |
| `--quality <n>` | JPEG capture quality 1-100 (default 80) |
| `--scale <f>` | Resolution scale (default 1.0; 0.5 = half size) |
| `--tab <id>` | Target a specific tab |

Supported formats: `.gif` (always available), `.webm` and `.mp4` (require ffmpeg on the server). Requires `security.allowScreencast: true`.

### `pinchtab text`
Extract readable text from the page.

```bash
pinchtab text
pinchtab text --raw    # no formatting cleanup
pinchtab text "#main"  # text from one element
```

### `pinchtab find <query>`
Find elements by text content or CSS selector.

```bash
pinchtab find "Submit"
pinchtab find ".btn-primary"
```

### `pinchtab eval <expression>`
Run JavaScript in the browser context.

```bash
pinchtab eval "document.title"
pinchtab eval "document.querySelectorAll('a').length"
```

> Requires `security.allowEvaluate: true` in config. Returns 403 by default.

### `pinchtab network`
Inspect captured network requests for the current tab.

```bash
pinchtab network
pinchtab network --limit 20
pinchtab network --filter api
pinchtab network <requestId> --body
```

---

## Fleet / Multi-Profile Commands

### `pinchtab profiles`
List available profiles.

```bash
pinchtab profiles
pinchtab instance start --profile work
```

### `pinchtab instances`
List running PinchTab instances across profiles.

---

## Known Quirks Summary

| Wrong | Right | Note |
|-------|-------|------|
| `pinchtab ss` | `pinchtab screenshot` | No `ss` alias |
| `pinchtab snapshot` | `pinchtab snap` | Use short form |
