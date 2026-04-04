# Endpoints Reference

This page lists the live HTTP surface exposed by PinchTab. Some routes are only available in bridge mode, some only in full server mode, and some are gated by security settings.

## Health And Server Metadata

```text
GET  /health
POST /ensure-chrome
GET  /openapi.json
GET  /help          (alias for /openapi.json)
GET  /metrics
GET  /api/metrics
POST /shutdown
GET  /api/events
```

Notes:

- in bridge mode, `/health` reports bridge health and tab count
- in full server mode, `/health` reports dashboard health, auth state, and instance count
- `/metrics` proxies to the bridge instance (per-instance runtime metrics)
- `/api/metrics` in full server mode is a server-level metrics snapshot (aggregated)

## Dashboard Auth And Config

```text
POST /api/auth/login
POST /api/auth/elevate
POST /api/auth/logout
GET  /api/config
PUT  /api/config
```

Notes:

- `server.token` is treated as write-only by `PUT /api/config`
- auth routes are for the dashboard session flow

## Navigation And Tabs

```text
POST /navigate
GET  /navigate
POST /tabs/{id}/navigate
POST /back
POST /back?tabId=<id>
POST /tabs/{id}/back
POST /forward
POST /forward?tabId=<id>
POST /tabs/{id}/forward
POST /reload
POST /reload?tabId=<id>
POST /tabs/{id}/reload
GET  /tabs
POST /tab
POST /tabs/{id}/close
GET  /tabs/{id}/metrics
```

Navigation request fields:

- `url` required
- `tabId` optional
- `newTab` optional
- `timeout` optional
- `blockImages`, `blockMedia`, `blockAds` optional
- `waitFor`, `waitSelector`, `waitTitle` optional

Important behavior:

- `POST /navigate` creates a new tab when `tabId` is omitted
- `POST /tab` supports `new`, `close`, and `focus`

## Tab Locking

```text
POST /lock
POST /unlock
POST /tabs/{id}/lock
POST /tabs/{id}/unlock
```

## Interaction And Analysis

```text
POST /action
GET  /action
POST /actions
POST /macro
POST /tabs/{id}/action
POST /tabs/{id}/actions
GET  /snapshot
GET  /tabs/{id}/snapshot
GET  /text
GET  /tabs/{id}/text
POST /find
POST /tabs/{id}/find
POST /evaluate
POST /tabs/{id}/evaluate
```

Action kinds currently include:

- `click`
- `dblclick`
- `type`
- `fill`
- `press`
- `hover`
- `focus`
- `select`
- `scroll`
- `drag`
- `check`
- `uncheck`
- `keyboard-type`
- `keyboard-inserttext`
- `keydown`
- `keyup`
- `scrollintoview`

Action targeting fields:

- `ref`
- `selector`
- `nodeId`
- `x` and `y`

Snapshot query parameters:

- `interactive`
- `compact`
- `diff`
- `selector`
- `maxTokens`
- `depth`
- `format`
- `noAnimations`
- `output`

Text query parameters:

- `mode=raw`
- `format`

Find body fields:

- `query`
- `tabId`
- `threshold`
- `topK`
- `lexicalWeight`
- `embeddingWeight`
- `explain`

## Screenshot, PDF, And Screencast

```text
GET  /screenshot
GET  /tabs/{id}/screenshot
GET  /pdf
POST /pdf
GET  /tabs/{id}/pdf
POST /tabs/{id}/pdf
GET  /screencast
GET  /screencast/tabs
GET  /instances/{id}/screencast
GET  /instances/{id}/proxy/screencast
```

Screenshot query parameters:

- `tabId`
- `format=jpeg|png`
- `quality`
- `raw=true`
- `output=file`
- `noAnimations=true`

PDF query parameters:

- `tabId`
- `raw=true`
- `output=file`
- `path`
- `landscape`
- `scale`
- `paperWidth`
- `paperHeight`
- `marginTop`
- `marginBottom`
- `marginLeft`
- `marginRight`
- `pageRanges`
- `preferCSSPageSize`
- `displayHeaderFooter`
- `headerTemplate`
- `footerTemplate`
- `generateTaggedPDF`
- `generateDocumentOutline`

## Downloads, Uploads, Cookies, And Clipboard

```text
GET  /download
GET  /tabs/{id}/download
POST /upload
POST /tabs/{id}/upload
GET  /cookies
POST /cookies
GET  /tabs/{id}/cookies
POST /tabs/{id}/cookies
GET  /clipboard/read
POST /clipboard/write
POST /clipboard/copy
GET  /clipboard/paste
POST /cache/clear
GET  /cache/status
```

Notes:

- download and upload endpoints are gated by `security.allowDownload` and `security.allowUpload`
- download automatically decompresses `.gz` files and returns the decompressed content
- `security.downloadAllowedDomains` can whitelist specific domains (bypasses SSRF checks for those domains). Setting `["*"]` matches every host and disables all private-IP protection on the download endpoint.
- clipboard endpoints are gated by `security.allowClipboard`
- upload uses a JSON body with `selector` and `files`

## Wait, Network, Dialog, Console, And Errors

```text
POST /wait
POST /tabs/{id}/wait
GET  /network
GET  /network/stream
GET  /network/export
GET  /network/export/stream
GET  /network/{requestId}
POST /network/clear
GET  /tabs/{id}/network
GET  /tabs/{id}/network/stream
GET  /tabs/{id}/network/export
GET  /tabs/{id}/network/export/stream
GET  /tabs/{id}/network/{requestId}
POST /dialog
POST /tabs/{id}/dialog
GET  /console
POST /console/clear
GET  /errors
POST /errors/clear
```

Wait body fields:

- one of `selector`, `text`, `url`, `load`, `fn`, or `ms`
- optional `tabId`
- optional `timeout`
- optional `state` for selector waits

Network query parameters:

- `tabId`
- `filter`
- `method`
- `status`
- `type`
- `limit`
- `bufferSize`
- `body=true` on detail requests

Network export query parameters:

- `format` — `har` (default) or `ndjson`. Pluggable: new formats register at startup.
- `output=file` — save to disk instead of streaming to response
- `path` — filename when `output=file` (auto-generated if omitted, required for `/export/stream`)
- `body=true` — include response bodies (fetched on demand, 10 MB cap per entry)
- `redact` — `true` (default) redacts Cookie/Authorization/Set-Cookie. `false` exports raw headers.
- all standard network filters (`filter`, `method`, `status`, `type`, `limit`)

The `/export` endpoint returns the full capture as a single response. The `/export/stream` endpoint writes entries to a file as they arrive (SSE progress events sent to the caller). The streamed file is atomically renamed on completion.

Dialog body fields:

- `action`: `accept` or `dismiss`
- `text`: optional prompt text
- `tabId`: optional on `/dialog`

Console and error routes use query parameters:

- `tabId`
- `limit`

## Challenge Solvers

```text
GET  /solvers
POST /solve
POST /solve/{name}
POST /tabs/{id}/solve
POST /tabs/{id}/solve/{name}
```

The solver framework auto-detects and resolves browser challenges (Cloudflare Turnstile, etc.). See [Solve reference](./reference/solve.md) for details.

Solve body fields:

- `solver` optional solver name (auto-detect when omitted)
- `tabId` optional
- `maxAttempts` optional (default: 3)
- `timeout` optional in ms (default: 30000)

## Profiles And Instances

```text
GET  /profiles
POST /profiles
POST /profiles/create
GET  /profiles/{id}
PATCH /profiles/{id}
DELETE /profiles/{id}
POST /profiles/{id}/start
POST /profiles/{id}/stop
GET  /profiles/{id}/instance
POST /profiles/{id}/reset
GET  /profiles/{id}/logs
GET  /profiles/{id}/analytics
POST /profiles/import
PATCH /profiles/meta
GET  /instances
GET  /instances/{id}
GET  /instances/tabs
GET  /instances/metrics
POST /instances/start
POST /instances/launch
POST /instances/attach
POST /instances/attach-bridge
POST /instances/{id}/start
POST /instances/{id}/stop
GET  /instances/{id}/logs
GET  /instances/{id}/logs/stream
GET  /instances/{id}/tabs
POST /instances/{id}/tabs/open
POST /instances/{id}/tab
```

Notes:

- `/instances/start` and `/instances/launch` use `mode`, not `headless`
- `/instances/launch` is a compatibility alias over `/instances/start`
- create profiles explicitly with `POST /profiles`; `name` is no longer supported on `/instances/launch`
- `/profiles/{id}/start` uses `headless`
- attach routes are gated by `security.attach`

## Activity And Scheduler

```text
GET  /api/activity
POST /tasks
GET  /tasks
GET  /tasks/{id}
POST /tasks/{id}/cancel
POST /tasks/batch
GET  /scheduler/stats
```

Activity query parameters include:

- `limit`
- `ageSec`
- `since`
- `until`
- `source`
- `requestId`
- `sessionId`
- `actorId`
- `agentId`
- `instanceId`
- `profileId`
- `profileName`
- `tabId`
- `action`
- `engine`
- `pathPrefix`

Activity attribution and source behavior:

- requests tagged with `X-Agent-Id` are recorded as `agentId` and can be filtered with `GET /api/activity?agentId=<id>`
- unfiltered `GET /api/activity` returns the primary activity feed
- named internal sources such as `dashboard` or `orchestrator` are stored in source-specific daily files and can be queried with `?source=<name>`

Scheduler routes are only present when `scheduler.enabled` is true.

## Agent Sessions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/agent-sessions` | Create a new agent session (body: `{agentId, label?}`) |
| `GET` | `/api/agent-sessions` | List all agent sessions |
| `GET` | `/api/agent-sessions/me` | Get current session (requires `Authorization: Session` auth) |
| `GET` | `/api/agent-sessions/{id}` | Get session details by ID |
| `POST` | `/api/agent-sessions/{id}/rotate` | Rotate session token |
| `POST` | `/api/agent-sessions/{id}/revoke` | Revoke session |

All endpoints except `/me` require dashboard auth (bearer or cookie). The `/me` endpoint requires session auth.

Create and rotate return `sessionToken` — the plaintext token shown only once.

## Feature Gates

Some endpoints are intentionally disabled unless the matching config allows them:

These gates are not ordinary feature toggles. Enabling them is a documented, non-default, security-reducing choice that widens the control surface available to callers.

- `/evaluate` and `/tabs/{id}/evaluate` -> `security.allowEvaluate`
- `/download` and `/tabs/{id}/download` -> `security.allowDownload`
- `/upload` and `/tabs/{id}/upload` -> `security.allowUpload`
- clipboard routes -> `security.allowClipboard`
- attach routes -> `security.attach`
- screencast routes -> `security.allowScreencast`

## Error Response Format

PinchTab currently uses two JSON error shapes during a transition period:

- Legacy JSON errors: `application/json` with fields like `error` and `code`
- Problem Details errors: `application/problem+json` (RFC 7807 style)

Problem Details is currently used for selected precondition and capability failures, including:

- websocket proxy pre-upgrade backend/hijack failures
- network stream unsupported streaming capability
- dashboard SSE unsupported streaming capability or deadline control
- instance logs SSE unsupported streaming capability or deadline control
- screencast tab-not-found precondition failure

Additional endpoints may be migrated over time. Clients should tolerate both error content types and branch on `Content-Type` when parsing failures.
