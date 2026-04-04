# CLI Commands Reference — Pinchtab 0.8.x

> **Quick tip:** Use `pinchtab help` or `pinchtab <command> --help` for full flag lists.

---

## Control Plane

### `pinchtab start`
Start the PinchTab server (default port 9867).

```bash
pinchtab start
pinchtab start --port 9868
pinchtab start --profile work --headless
```

### `pinchtab stop`
Stop the running server.

### `pinchtab status` / `pinchtab health`
Check if the server is running and healthy.

---

## Browser Commands

### `pinchtab nav <url>`
Navigate the current tab to a URL.

```bash
pinchtab nav https://pinchtab.com
pinchtab nav https://pinchtab.com --new-tab
pinchtab nav https://pinchtab.com --block-images
pinchtab nav https://pinchtab.com --timeout 60
```

| Flag | Description |
|------|-------------|
| `--new-tab` | Open in a new tab instead of current |
| `--block-images` | Block image loading (faster, fewer tokens) |
| `--timeout <s>` | Navigation timeout in seconds |
| `--profile <name>` | Target a named profile |

> ⚠️ **Quirk:** Use `--profile`, not `--profileId`. The long-form flag is required.

### `pinchtab tab` (not `tabs`)
Manage browser tabs.

```bash
pinchtab tab list            # List all open tabs
pinchtab tab close           # Close current tab
pinchtab tab close <tabId>   # Close specific tab
```

> ⚠️ **Quirk:** The command is `tab` (singular), not `tabs`.

---

## Interaction Commands

### `pinchtab click <ref>`
Click an element by its accessibility ref (from `snap`).

```bash
pinchtab click e5
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

### `pinchtab scroll [ref]`
Scroll the page or a specific element.

```bash
pinchtab scroll            # scroll page down 300px
pinchtab scroll --pixels -300   # scroll up
pinchtab scroll e20 --pixels 500
```

### `pinchtab select <ref> <value>`
Select an option from a `<select>` dropdown.

```bash
pinchtab select e8 "option-value"
```

---

## Output Commands

### `pinchtab snap` (snapshot)
Get the accessibility tree of the current page. **Primary tool for understanding page state.**

```bash
pinchtab snap                   # full tree
pinchtab snap -i                # interactive elements only (smaller)
pinchtab snap -c                # compact format (fewer tokens)
pinchtab snap -i -c             # both: cheapest snapshot
pinchtab snap -d                # diff: only changes since last snap
pinchtab snap -s main           # scoped to CSS selector
pinchtab snap --max-tokens 2000 # token budget cap
```

> ⚠️ **Quirk:** Use `snap`, not `snapshot`. The alias `snap` is the intended short form.

### `pinchtab screenshot`
Capture a screenshot of the current page.

```bash
pinchtab screenshot
pinchtab screenshot --quality 80   # JPEG at 80%
```

> ⚠️ **Quirk:** Use `screenshot` (full word), not `ss` or `shot`.

### `pinchtab text`
Extract readable text from the page.

```bash
pinchtab text
pinchtab text --raw    # no formatting cleanup
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

### `pinchtab network-export`
Export captured network data in standard formats.

```bash
pinchtab network-export                           # HAR 1.2 to exports/
pinchtab network-export -o session.har            # HAR to specific file
pinchtab network-export --format ndjson -o s.ndjson # NDJSON (one JSON per line)
pinchtab network-export --body                    # Include response bodies
pinchtab network-export --stream -o live.har      # Live capture while browsing
```

Formats: `har` (Chrome DevTools compatible), `ndjson` (streamable). Sensitive headers redacted by default.

---

## Fleet / Multi-Profile Commands

### `pinchtab profile list`
List all available profiles.

### `pinchtab profile use <name>`
Switch the active profile.

```bash
pinchtab profile use work
```

### `pinchtab instances`
List running PinchTab instances across profiles.

---

## Known Quirks Summary

| Wrong | Right | Note |
|-------|-------|------|
| `pinchtab ss` | `pinchtab screenshot` | No `ss` alias |
| `pinchtab snapshot` | `pinchtab snap` | Use short form |
| `--profileId` | `--profile` | Long-form flag name |
| `pinchtab tabs` | `pinchtab tab` | Singular subcommand |
