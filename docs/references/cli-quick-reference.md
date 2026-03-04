# CLI Quick Reference

Pinchtab CLI provides both **browser commands** and **management commands**. All browser commands talk to a running server via the HTTP API.

## Browser Commands

### Navigate

```bash
pinchtab nav https://example.com
```

### Snapshot (accessibility tree)

```bash
pinchtab snap                              # current page
pinchtab snap https://example.com          # navigate first, then snapshot
pinchtab snap -i                           # interactive elements only
pinchtab snap -c                           # compact output
pinchtab snap --depth 3                    # limit nesting depth
pinchtab snap --text                       # text format
```

### Find (semantic element search)

```bash
pinchtab find "Sign In"                    # search current page
pinchtab find "Sign In" --url https://example.com   # navigate first
pinchtab find "Iran guerra" --top 10 --threshold 0.15
```

Output:
```
  [e42] 0.85  button: Sign In
  [e15] 0.62  link: Sign In with Google

Best: e42
```

### Text extraction

```bash
pinchtab text                              # current page
pinchtab text https://example.com          # navigate first
```

### Screenshot

```bash
pinchtab screenshot                        # saves screenshot.jpg
pinchtab screenshot https://example.com    # navigate first
pinchtab screenshot --out page.jpg         # custom filename
```

### PDF export

```bash
pinchtab pdf                               # saves page.pdf
pinchtab pdf https://example.com           # navigate first
pinchtab pdf --out report.pdf              # custom filename
```

### Actions (require prior snapshot for refs)

```bash
pinchtab click e5                          # click element
pinchtab type e7 "hello world"             # type text
pinchtab fill e7 "hello world"             # replace input value
pinchtab press Enter                       # press key
pinchtab hover e5                          # hover element
pinchtab scroll e5                         # scroll element into view
pinchtab scroll                            # scroll page
pinchtab select e12 "option-value"         # select dropdown
```

### Evaluate JavaScript

```bash
pinchtab eval "document.title"
pinchtab eval "window.location.href"
```

### Typical workflow

```bash
pinchtab snap https://example.com -i       # navigate + snapshot interactive
pinchtab click e5                          # click something
pinchtab snap -i                           # see what changed
pinchtab find "Submit"                     # find an element
pinchtab click e42                         # click it
```

---

## Instance Commands

### Launch an instance

```bash
pinchtab launch                            # headless, auto-named
pinchtab launch myprofile                  # named profile
pinchtab launch --headed                   # with visible browser
pinchtab launch --port 9880               # specific port
```

### Stop an instance

```bash
pinchtab stop inst_abc123
```

### List running instances

```bash
pinchtab instances
```

---

## Tab Commands

### Open a new tab

```bash
pinchtab open                              # blank tab
pinchtab open https://example.com          # tab with URL
```

### Close a tab

```bash
pinchtab close tab_abc123
```

Uses `DELETE /tabs/{id}` under the hood.

### List open tabs

```bash
pinchtab tabs
```

---

## Management Commands

### Server status

```bash
pinchtab health
```

### List profiles

```bash
pinchtab profiles
```

### Get instance URL

```bash
pinchtab connect myprofile
# Output: http://localhost:9868
```

### Configuration

```bash
pinchtab config init                       # create default config
pinchtab config show                       # display config (JSON)
pinchtab config show --format yaml         # display config (YAML)
pinchtab config set server.port 9999       # set a value
pinchtab config patch '{"chrome":{"headless":false}}'
pinchtab config validate                   # check config
```

---

## Environment Variables

```bash
# Server
BRIDGE_PORT=9867                           # server port
BRIDGE_BIND=127.0.0.1                      # bind address
BRIDGE_HEADLESS=true                       # headless Chrome
BRIDGE_TOKEN=secret                        # API auth token

# Strategy (orchestrator mode)
PINCHTAB_STRATEGY=simple                   # simple | session | explicit
PINCHTAB_ALLOCATION_POLICY=fcfs            # fcfs | round_robin | random

# Client
PINCHTAB_URL=http://localhost:9867         # server URL for CLI commands
PINCHTAB_TOKEN=secret                      # auth token for CLI commands
```

---

## Smart URL Parameter

Browser commands that accept a URL (`snap`, `find`, `text`, `screenshot`, `pdf`) will automatically navigate to the URL first, wait for the page to be ready, then perform the operation — all in one call.

Wait strategy is chosen per operation:
- `snap` / `text` / `find` / `eval` → waits for DOM ready (fast)
- `screenshot` / `pdf` → waits for full page load (needs rendering)

See [CDP Bridge](../architecture/cdp-bridge.md) for details.
