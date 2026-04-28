# Endpoints Reference

This page summarizes the live HTTP surface exposed by PinchTab. Some routes are only available in bridge mode, some only in full server mode, and some are gated by security settings.

## Health And Server Metadata

```text
GET  /health
POST /ensure-chrome
POST /browser/restart
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

## Dashboard Events And Agents

```text
GET  /api/events
GET  /api/agents
GET  /api/agents/{id}
GET  /api/agents/{id}/events
POST /api/agents/{id}/events
```

Notes:

- `/api/events` is the dashboard SSE stream
- `/api/agents/{id}/events` streams one agent's recent events
- `POST /api/agents/{id}/events` ingests agent activity into the dashboard feed

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
POST /close
POST /tabs/{id}/close
GET  /tabs/{id}/metrics
POST /tabs/{id}/handoff
GET  /tabs/{id}/handoff
POST /tabs/{id}/resume
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
- `POST /tab` supports `new` and `focus`
- `POST /close` closes the `tabId` supplied in the JSON body, or the current/default tab when `tabId` is omitted

## Handoff And Manual Intervention

```text
POST /tabs/{id}/handoff
GET  /tabs/{id}/handoff
POST /tabs/{id}/resume
```

Notes:

- these routes are tab-scoped only
- `POST /tabs/{id}/handoff` marks the tab as `paused_handoff` and records a reason
- `GET /tabs/{id}/handoff` returns the current handoff state, or `active` when no handoff is set
- `POST /tabs/{id}/resume` clears the handoff state and can carry resume metadata for the caller
- current behavior is advisory only: handoff state is not yet a hard block on subsequent automation requests
- treat the current implementation as temporary coordination state, not as a security boundary
- there is currently no dedicated CLI wrapper for handoff or resume; use the HTTP API

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
POST /wait
POST /tabs/{id}/wait
GET  /frame
POST /frame
GET  /tabs/{id}/frame
POST /tabs/{id}/frame
GET  /snapshot
GET  /tabs/{id}/snapshot
GET  /text
GET  /tabs/{id}/text
POST /find
POST /tabs/{id}/find
POST /evaluate
POST /tabs/{id}/evaluate
```

`/evaluate` is intentionally separate from selector frame scope. `GET/POST /frame` only affects selector-based `/snapshot` and `/action` calls, not arbitrary JavaScript evaluation.

Action kinds currently include:

- `click`
- `dblclick`
- `type`
- `fill`
- `press`
- `hover`
- `mouse-move`
- `mouse-down`
- `mouse-up`
- `mouse-wheel`
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
- `button`
- `deltaX` and `deltaY`
- `waitNav`
- `dialogAction` and `dialogText`
- `humanize`

`humanize` is a per-action override for input style. When omitted, actions use `instanceDefaults.humanize`, which defaults to `false`. Use `kind:"click"` or `kind:"type"` with `humanize:true` when a page needs the slower human-like pointer or typing path.

Pointer fallback behavior:

- `mouse-move` first attempts a real CDP `mouseMoved` event.
- If headless Chromium stalls that move waiting for renderer acknowledgement, PinchTab falls back to DOM `mouseover`/`mouseenter`/`mousemove` events at the same target so hover-style checks remain responsive.
- Non-timeout CDP errors and caller context cancellation are not hidden by the fallback.
- `mouse-wheel` dispatches a DOM `WheelEvent` at the target point and scrolls the window when the event is not cancelled.

Selector lookup is limited to the current frame scope. The default scope is `main`. Use `/frame` or `/tabs/{id}/frame` before selector-based iframe actions. Same-origin iframe scopes are supported; cross-origin iframe descendants are not currently exposed.

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

`selector` on `/snapshot` follows the same rule: it only searches the current frame scope. It does not automatically pierce into iframes, and cross-origin iframe descendants are not inlined.

Text query parameters:

- `mode=raw`
- `format`
- `maxChars`
- `frameId`

`/text` default mode picks the first **visible** `<article>` / `[role="main"]` /
`<main>` (skips `display:none`) and strips nav/footer/ads. Use `mode=raw` for
full `innerText`, or `/snapshot` for structured UI text like prices and button
labels.

`/text` is also frame-aware. `frameId` targets a specific iframe for a one-shot
read; otherwise the endpoint inherits the tab's current `/frame` scope.

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

## Storage

```text
GET    /storage
POST   /storage
DELETE /storage
GET    /tabs/{id}/storage
POST   /tabs/{id}/storage
DELETE /tabs/{id}/storage
```

Storage is captured only for the current origin (active tab). Multi-origin storage is not supported.

All storage routes are gated by `security.allowStateExport`.

GET query parameters:

- `type` — `local`, `session`, or empty (both)
- `key` — optional, specific key to retrieve
- `tabId` — optional tab identifier

POST body fields:

- `key` — required
- `value` — required
- `type` — `local` or `session` (required)
- `tabId` — optional

DELETE body fields:

- `type` — `local` or `session` (required)
- `key` — optional (if omitted, clears entire storage)
- `tabId` — optional

## State Management

```text
GET    /state/list
GET    /state/show
POST   /state/save
POST   /state/load
DELETE /state
POST   /state/clean
```

State management saves and restores browser state (cookies, localStorage, sessionStorage, metadata) to disk.

Notes:

- All state and storage endpoints are gated by `security.allowStateExport`: `/storage`, `/tabs/{id}/storage`, `GET /state/list`, `GET /state/show`, `POST /state/save`, `POST /state/load`, `DELETE /state`, and `POST /state/clean`
- state files are stored in `{stateDir}/sessions/` with `0600` permissions
- optional AES-256-GCM encryption via `security.stateEncryptionKey` config setting
- storage is captured only for the current origin (active tab)

`POST /state/save` body fields:

- `name` — state file name
- `encrypt` — optional, encrypt the state file
- `tabId` — optional tab identifier
- `metadata` — optional additional metadata

`POST /state/load` body fields:

- `name` — state file name (required)
- `tabId` — optional tab identifier

`DELETE /state` query parameters:

- `name` — state file name (required)

`POST /state/clean` body fields:

- `olderThanHours` — optional (default: 24)

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

- exactly one of:
  - `selector` — CSS / XPath (`xpath:` prefix or leading `//`) / text (`text:` prefix)
  - `text` — substring of `document.body.innerText`
  - `notText` — wait until substring is no longer present
  - `url` — glob pattern matched against `window.location.href` (`**`, `*`, `?`)
  - `load` — one of:
    - `ready-state` → `document.readyState === 'complete'`
    - `content-loaded` → `document.readyState` in {`interactive`, `complete`}
    - `network-idle` → zero in-flight CDP requests held for `idleFor` ms (default 500, max 10000). Legacy alias `networkidle` accepted.
  - `fn` — JS expression polled until truthy (requires `security.allowEvaluate`)
  - `ms` — fixed sleep in milliseconds, max 30000 (escape hatch; prefer condition-based waits)
- optional `tabId`
- optional `timeout` — ms, default 10000, clamped 100–30000
- optional `state` for selector waits — `visible` (default) or `hidden`
- optional `idleFor` for `load: network-idle` — ms quiet period, default 500, clamped 0–10000

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
GET  /config/autosolver
POST /solve
POST /solve/{name}
POST /tabs/{id}/solve
POST /tabs/{id}/solve/{name}
```

The autosolver framework auto-detects and resolves browser challenges (Cloudflare Turnstile, CAPTCHAs, interstitials, etc.). See [Solve reference](./reference/solve.md) for details.

Solve body fields:

- `solver` optional solver name (auto-detect when omitted)
- `tabId` optional
- `maxAttempts` optional (defaults to `autoSolver.maxAttempts`, default `8`)
- `timeout` optional in ms (auto-estimated when omitted, minimum `30000`)

`GET /config/autosolver` returns effective autosolver runtime settings and the
currently available solver list.

Example response:

```json
{
	"enabled": true,
	"autoTrigger": true,
	"triggerOnNavigate": true,
	"triggerOnAction": true,
	"maxAttempts": 8,
	"solverTimeoutSec": 30,
	"retryBaseDelayMs": 500,
	"retryMaxDelayMs": 10000,
	"solvers": ["cloudflare", "semantic", "jschallenge"],
	"llmProvider": "",
	"llmFallback": false
}
```

Notes:

- `capsolver` and `twocaptcha` appear in `solvers` only when their API keys are configured.

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
POST /instances/{id}/restart
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
- instance responses include both `mode` and `headless`
- instance start surfaces accept `securityPolicy.allowedDomains` for additive instance-scoped IDPI/domain allowlist overrides
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
- named non-client sources such as `dashboard` or `orchestrator` are stored in source-specific daily files only when enabled under `observability.activity.events`, and can then be queried with `?source=<name>`

Scheduler routes are only present when `scheduler.enabled` is true.

## Agent Sessions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/sessions` | Create a new agent session (body: `{agentId, label?}`) |
| `GET` | `/sessions` | List all agent sessions |
| `GET` | `/sessions/me` | Get current session (requires `Authorization: Session` auth) |
| `GET` | `/sessions/{id}` | Get session details by ID |
| `POST` | `/sessions/{id}/revoke` | Revoke session |

`POST /sessions`, `GET /sessions`, and `GET /sessions/{id}` require dashboard auth (bearer or cookie). The `/me` endpoint requires session auth. `POST /sessions/{id}/revoke` allows dashboard auth or the owning session.

Create returns `sessionToken` — the plaintext token shown only once.

Session-authenticated callers cannot reach dashboard/admin endpoint families such as config, dashboard agent listings, dashboard event streams, session management, profile management, instance management, or cache controls. They are intended for trusted automation in controlled environments, not for untrusted multi-tenant isolation.

## Feature Gates

Some endpoints are intentionally disabled unless the matching config allows them:

These gates are not ordinary feature toggles. Enabling them is a documented, non-default, security-reducing choice that widens the control surface available to callers.

- `/evaluate` and `/tabs/{id}/evaluate` -> `security.allowEvaluate`
- `/download` and `/tabs/{id}/download` -> `security.allowDownload`
- `/upload` and `/tabs/{id}/upload` -> `security.allowUpload`
- clipboard routes -> `security.allowClipboard`
- attach routes -> `security.attach`
- screencast routes -> `security.allowScreencast`
- storage routes (`/storage`, `/tabs/{id}/storage`) and the full state-management family (`/state/list`, `/state/show`, `/state/save`, `/state/load`, `DELETE /state`, `POST /state/clean`) -> `security.allowStateExport`

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
