# Tabs

Tabs are the main execution surface for browsing, extraction, interaction, and diagnostics.

Use tab-scoped HTTP routes once you already have a tab ID. In the CLI, use the normal top-level browser commands with `--tab <id>`.

`pinchtab tab` itself is only for:

- listing tabs
- focusing a tab
- opening a new tab
- closing a tab

There are no subcommands such as `pinchtab tab navigate` or `pinchtab tab click`.

## Top-Level Browser Commands

These pages cover the shorthand routes and matching CLI commands:

- [Health](./health.md)
- [Navigate](./navigate.md)
- [Snapshot](./snapshot.md)
- [Text](./text.md)
- [Click](./click.md)
- [Type](./type.md)
- [Fill](./fill.md)
- [Screenshot](./screenshot.md)
- [PDF](./pdf.md)
- [Eval](./eval.md)
- [Press](./press.md)
- [Hover](./hover.md)
- [Scroll](./scroll.md)
- [Select](./select.md)
- [Focus](./focus.md)
- [Find](./find.md)

## Open A Tab In A Specific Instance

```bash
curl -X POST http://localhost:9867/instances/inst_ea2e747f/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# Response
{
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "url": "https://pinchtab.com",
  "title": "PinchTab"
}
```

There is still no dedicated instance-scoped tab-open CLI command. The CLI shortcut is:

```bash
pinchtab instance navigate inst_ea2e747f https://pinchtab.com
```

That command opens a tab for the instance and then navigates it.

## List Tabs

### Active Bridge Or Shorthand Context

```bash
curl http://localhost:9867/tabs
# Response (API always returns JSON)
{
  "tabs": [
    {
      "id": "8f9c7d4e1234567890abcdef12345678",
      "url": "https://pinchtab.com",
      "title": "PinchTab",
      "type": "page"
    }
  ]
}

# CLI Alternative (human-readable by default)
pinchtab tab
# Output: *8f9c7d4e...  https://pinchtab.com  PinchTab

pinchtab tab --json                    # Full JSON response
```

Notes:

- `GET /tabs` is not a fleet-wide inventory
- in bridge mode or shorthand mode it lists tabs from the active browser context
- `pinchtab tab` follows that shorthand behavior

### Tabs For One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f/tabs
```

### Tabs Across All Running Instances

```bash
curl http://localhost:9867/instances/tabs
```

Use `GET /instances/tabs` when you need the orchestrator-wide view.

## Focus And Close From The CLI

```bash
pinchtab tab                           # list tabs
pinchtab tab 2                         # focus tab by 1-based index
pinchtab tab 8f9c7d4e1234...           # focus tab by tab ID
pinchtab nav https://pinchtab.com --new-tab  # open a new tab and navigate it
pinchtab tab close 8f9c7d4e1234...     # close tab
```

Numeric arguments are resolved as 1-based indices against `GET /tabs`. Non-numeric arguments are treated as tab IDs.

Focusing, navigating, or otherwise accessing a tracked tab marks it as the current tab. Unscoped commands use that current tab; if the recorded current tab has gone stale, PinchTab falls back to the most recently used tracked tab.

Top-level navigation uses the current tab when one is available. Use `pinchtab nav <url> --new-tab` when you explicitly want another tab.

## Operate On An Existing Tab

Use the tab-scoped HTTP route or the top-level CLI command with `--tab`.

### Navigate

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# CLI Alternative
pinchtab nav https://pinchtab.com --tab <tabId>
```

### Snapshot

```bash
curl "http://localhost:9867/tabs/<tabId>/snapshot?filter=interactive&format=compact"
# CLI Alternative
pinchtab snap --tab <tabId> -i -c
```

### Text

```bash
curl "http://localhost:9867/tabs/<tabId>/text?mode=raw"
# CLI Alternative
pinchtab text --tab <tabId> --raw
```

### Find

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/find \
  -H "Content-Type: application/json" \
  -d '{"query":"login button"}'
# CLI Alternative
pinchtab find --tab <tabId> "login button"
```

### Actions

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
# CLI Alternatives
pinchtab click --tab <tabId> e5
pinchtab fill --tab <tabId> '#email' 'ada@example.com'
pinchtab wait --tab <tabId> 'text:Done'
pinchtab network --tab <tabId> --limit 20
```

Low-level pointer control uses the same action surface:

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-move","ref":"e5"}'

curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-down","button":"left"}'

curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"mouse-wheel","x":400,"y":320,"deltaY":240}'

# CLI Alternatives
pinchtab mouse move --tab <tabId> e5
pinchtab mouse down --tab <tabId> --button left
pinchtab mouse wheel --tab <tabId> 240 --dx 40
```

### Handoff State

Human handoff is tab-scoped and available through CLI or API.

```bash
pinchtab tab handoff <tabId> --reason captcha --timeout-ms 120000
pinchtab tab handoff-status <tabId>
pinchtab tab resume <tabId> --status completed
```

API equivalents:

When a tab is marked `paused_handoff`, action execution routes reject with `409 tab_paused_handoff`
until the tab is resumed or the optional timeout expires.

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/handoff \
  -H "Content-Type: application/json" \
  -d '{"reason":"captcha","timeoutMs":120000}'

curl http://localhost:9867/tabs/<tabId>/handoff

curl -X POST http://localhost:9867/tabs/<tabId>/resume \
  -H "Content-Type: application/json" \
  -d '{"status":"completed","resolvedData":{"operator":"human"}}'
```

Use this when automation must pause for a CAPTCHA, 2FA prompt, login approval, or another human-only step.
When a timeout is provided, handoff status includes `expiresAt` and `timeoutMs`.

### Screenshot

```bash
curl "http://localhost:9867/tabs/<tabId>/screenshot?raw=true" > out.jpg
# CLI Alternative
pinchtab screenshot --tab <tabId> -o out.jpg
```

### PDF

```bash
curl "http://localhost:9867/tabs/<tabId>/pdf?raw=true" > page.pdf
# CLI Alternative
pinchtab pdf --tab <tabId> -o page.pdf
```

## Cookies

```bash
curl http://localhost:9867/tabs/<tabId>/cookies
curl -X POST http://localhost:9867/tabs/<tabId>/cookies \
  -H "Content-Type: application/json" \
  -d '{"cookies":[{"name":"session","value":"abc"}]}'
```

`POST` defaults `url` to the tab's current page, so injecting a session cookie
needs no URL lookup. A cookie with an empty `value` is set (blanking it without
deleting it); a cookie with no `name` is rejected with 400 rather than skipped.
`DELETE /tabs/<tabId>/cookies` is tab-scoped in addressing only — it clears every
cookie for every origin, exactly like `DELETE /cookies`.

The same operations from the CLI:

```bash
pinchtab cookies get --tab <tabId>                  # read cookies, with values
pinchtab cookies get --tab <tabId> --name session   # one cookie
pinchtab cookies set session abc123 --tab <tabId>   # set on the tab's current URL
pinchtab cookies clear                              # every cookie, every origin
```

`cookies set` takes `--url`, `--domain`, `--path`, `--same-site`, `--secure` and
`--http-only`; it exits non-zero if the server reports the cookie was not set.
There is no per-cookie removal verb — `clear` is browser-wide, and nothing in the
CLI restores what it wipes, so re-set what you need or use `state load`.

Reading, writing, and clearing cookies requires `security.allowCookies=true`.

## Metrics

```bash
curl http://localhost:9867/tabs/<tabId>/metrics
```

This reports the owning browser instance's aggregate memory — the whole process tree —
not an isolated per-tab reading: the tab id picks the instance to ask. Two tabs of one
instance return the same figures. See [Memory monitoring](../guides/memory-monitoring.md).

## Lock And Unlock

Tab locking is API-only.

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/lock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent","ttl":60}'

curl -X POST http://localhost:9867/tabs/<tabId>/unlock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent"}'
```

There are also active-tab forms at `POST /lock` and `POST /unlock`.

## Important Limits

- There is no `GET /tabs/{id}` endpoint for fetching single-tab metadata.
- `GET /tabs` and `GET /instances/tabs` serve different purposes and are not interchangeable.
- In the CLI, tab-scoped work happens through top-level commands with `--tab`, not through `pinchtab tab <subcommand>` variants — except for `handoff`, `resume`, and `handoff-status`, which are exposed both as top-level commands and as `pinchtab tab handoff|resume|handoff-status` subcommands.
