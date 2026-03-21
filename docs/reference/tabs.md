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
# CLI Alternative
pinchtab tab
# Response
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

## Focus, Create, And Close From The CLI

```bash
pinchtab tab                           # list tabs
pinchtab tab 2                         # focus tab by 1-based index
pinchtab tab 8f9c7d4e1234...           # focus tab by tab ID
pinchtab tab new                       # open blank tab
pinchtab tab new https://example.com   # open and navigate
pinchtab tab close 8f9c7d4e1234...     # close tab
```

Numeric arguments are resolved as 1-based indices against `GET /tabs`. Non-numeric arguments are treated as tab IDs.

## Operate On An Existing Tab

Use the tab-scoped HTTP route or the top-level CLI command with `--tab`.

### Navigate

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
# CLI Alternative
pinchtab nav https://example.com --tab <tabId>
```

### Snapshot

```bash
curl "http://localhost:9867/tabs/<tabId>/snapshot?interactive=true&compact=true"
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

There is no dedicated top-level cookies CLI command today.

## Metrics

```bash
curl http://localhost:9867/tabs/<tabId>/metrics
```

This reports memory metrics for the tab through the bridge, not a full per-tab performance profile.

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

There are also active-tab forms at `POST /tab/lock` and `POST /tab/unlock`.

## Important Limits

- There is no `GET /tabs/{id}` endpoint for fetching single-tab metadata.
- `GET /tabs` and `GET /instances/tabs` serve different purposes and are not interchangeable.
- In the CLI, tab-scoped work happens through top-level commands with `--tab`, not through `pinchtab tab <subcommand>` variants.
