# Tabs

Tabs are the main execution surface for browsing, extracting, and interacting with pages.

Use tab-scoped routes once you already have a tab ID. Use instance-scoped routes when you need to create a new tab in a specific instance.

Tab IDs should be treated as opaque values returned by the API. Do not construct them yourself or assume one stable format across all routes.

## Shorthand Browser Commands

Top-level browser commands such as `nav`, `snap`, `text`, `click`, `type`, `fill`, `pdf`, `ss`, `eval`, and `health` now have their own quick reference pages.

Use those pages when you want the shorthand route plus the matching CLI command:

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

There is no dedicated instance-scoped `tab open` CLI command today.

If you want a CLI shortcut that opens a tab and navigates it, use:

```bash
pinchtab instance navigate inst_ea2e747f https://pinchtab.com
```

## List Tabs

### Shorthand Or Bridge List

```bash
curl http://localhost:9867/tabs
# CLI Alternative
pinchtab tabs
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

- `GET /tabs` is not a fleet-wide orchestrator inventory
- in bridge mode or shorthand mode it lists tabs from the active browser context
- `pinchtab tabs` follows that shorthand behavior

### One Instance

```bash
curl http://localhost:9867/instances/inst_ea2e747f/tabs
# Response
[
  {
    "id": "8f9c7d4e1234567890abcdef12345678",
    "instanceId": "inst_ea2e747f",
    "url": "https://pinchtab.com",
    "title": "PinchTab"
  }
]
```

### All Running Instances

```bash
curl http://localhost:9867/instances/tabs
```

Use `GET /instances/tabs` when you need the fleet-wide view.

## Navigate An Existing Tab

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
# CLI Alternative
pinchtab tab navigate <tabId> https://example.com
# Response
{
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

## Snapshot

```bash
curl "http://localhost:9867/tabs/<tabId>/snapshot?interactive=true&compact=true"
# CLI Alternative
pinchtab tab snapshot <tabId> -i -c
```

Use this to retrieve the accessibility snapshot and element refs for the page.

## Text

```bash
curl "http://localhost:9867/tabs/<tabId>/text?raw=true"
# CLI Alternative
pinchtab tab text <tabId> --raw
```

## Find

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/find \
  -H "Content-Type: application/json" \
  -d '{"query":"login button"}'
# Response
{
  "best_ref": "e5",
  "confidence": "high",
  "score": 0.85
}
```

There is no dedicated CLI `find` command today.

## Action

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
# CLI Alternative
pinchtab tab click <tabId> e5
```

Other CLI-backed tab operations include:

- `pinchtab tab type <tabId> <ref> <text>`
- `pinchtab tab fill <tabId> <ref> <text>`
- `pinchtab tab press <tabId> <key>`
- `pinchtab tab hover <tabId> <ref>`
- `pinchtab tab scroll <tabId> <direction|pixels>`
- `pinchtab tab select <tabId> <ref> <value>`
- `pinchtab tab focus <tabId> <ref>`

## Screenshot

```bash
curl "http://localhost:9867/tabs/<tabId>/screenshot?raw=true" > out.png
# CLI Alternative
pinchtab tab screenshot <tabId> -o out.png
```

## PDF

```bash
curl "http://localhost:9867/tabs/<tabId>/pdf?raw=true" > page.pdf
# CLI Alternative
pinchtab tab pdf <tabId> -o page.pdf
```

## Cookies

```bash
curl http://localhost:9867/tabs/<tabId>/cookies
# CLI Alternative
pinchtab tab cookies <tabId>
```

## Metrics

```bash
curl http://localhost:9867/tabs/<tabId>/metrics
```

This returns aggregate browser metrics for the tab's owning instance, not isolated per-tab memory.

## Lock And Unlock

Tab lock/unlock is **API-only** — no CLI commands exist yet.

### Lock A Specific Tab

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/lock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent","ttl":60}'
```

### Lock The Active Tab

```bash
curl -X POST http://localhost:9867/tab/lock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent","ttl":60}'
```

### Unlock A Specific Tab

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/unlock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent"}'
```

### Unlock The Active Tab

```bash
curl -X POST http://localhost:9867/tab/unlock \
  -H "Content-Type: application/json" \
  -d '{"owner":"my-agent"}'
```

## Close A Tab

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/close
# CLI Alternative
pinchtab tab close <tabId>
# Response
{
  "status": "closed"
}
```

## Important Limits

- **There is no `GET /tabs/{id}` endpoint** for fetching single tab info. Use `GET /tabs` to list all tabs or access tab-scoped sub-paths like `/tabs/{id}/snapshot`, `/tabs/{id}/action`, etc.
- `GET /tabs` and `GET /instances/tabs` serve different purposes and should not be treated as interchangeable
