# Endpoints Reference

This page summarizes the live HTTP surface exposed by PinchTab. Some routes are only available in bridge mode, some only in full server mode, and some are gated by security settings.

## Health And Server Metadata

```text
GET  /health
POST /ensure-browser
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
- `/metrics` reports the counters of the process answering it: in full server mode the front
  door's own request counters (auth rejections and unrouted paths included), in bridge mode
  the bridge's. Every response names its `layer`, and the layers are never summed — read one
  instance's counters at `/instances/{id}/metrics`. See
  [reference/metrics.md](reference/metrics.md) for the layer table.
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

- `POST /navigate` creates a new tab when `tabId` is omitted for anonymous callers
- session-authenticated callers keep a current tab per session; omitted `tabId` reuses that session's current tab when one exists, otherwise creates one
- bearer-token callers with `X-Agent-Id` keep a current tab per agent ID when no session is present
- `POST /tab` supports `new` and `focus`
- `POST /close` closes the `tabId` supplied in the JSON body, or the caller's current/default tab when `tabId` is omitted

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
- a paused-handoff tab blocks the action-execution routes, but the two envelopes differ.
  `POST /action` returns `409` with code `tab_paused_handoff` and a `details.hint` naming
  `/resume`. `POST /actions` and `POST /macro` return **200** — they answer with a result
  list — and carry the same refusal per item: the entry for each step against the paused tab
  has `success: false`, `code: "tab_paused_handoff"` and the same `details`. Match on the
  code, not on the message. With `stopOnError` false (the default) the remaining steps still
  run, so each step aimed at the paused tab is refused the same way while steps naming another
  tab execute normally. `/resume` clears the state for all of them.
- treat the handoff record as coordination state, not as a security boundary — non-action endpoints (snapshots, screenshots, network logs, evals subject to their own gates) remain reachable
- CLI wrappers exist: `pinchtab handoff`, `pinchtab resume`, `pinchtab handoff-status`, plus the `pinchtab tab handoff|resume|handoff-status` aliases

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
GET  /visible
GET  /tabs/{id}/visible
POST /find
POST /tabs/{id}/find
POST /evaluate
POST /tabs/{id}/evaluate
```

`/evaluate` is intentionally separate from selector frame scope. `GET/POST /frame` only affects selector-based `/snapshot` and `/action` calls, not arbitrary JavaScript evaluation.

`GET /action` decodes a subset of the action fields and refuses, with `400` naming the field, any parameter it cannot express rather than silently dropping it — so a modifier chord, a drag, `waitNav` or `humanize` must be sent as `POST /action` with a JSON body. A parameter the action request does not declare at all is refused the same way, with a `did you mean` hint for a near miss, so `?modifers=8` or `?Modifiers=8` no longer dispatches a plain click and answers `200`. The accepted set is the action request's own fields plus the parameters only the GET form carries, which today is `timeout` — a per-request action timeout in seconds, clamped to 0–60, that the POST form sends in its body instead. Cache-busters and stray parameters must be dropped from the URL.

`GET /snapshot` validates its cost controls the same way but resolves the unknown-parameter
question differently, and the difference is deliberate. A bad VALUE is refused with a `400`
naming the accepted set: `format` is `json`, `compact`, `text` or `yaml`; `filter` is `all`
or `interactive`; `maxTokens` is a positive whole number; `depth` is a whole number `>= -1`.
`format` and `filter` are compared case- and whitespace-insensitively, so `INTERACTIVE` and
`" interactive "` select the interactive subset — each previously fell through to the whole
tree, because the comparison was an exact string match, and every one of these controls used
to fail toward the *more expensive* answer without telling the caller.

`interactive` is the documented boolean alias for `filter`: `interactive=true` is
`filter=interactive`, `interactive=false` is `filter=all`. It takes the values Go's
`ParseBool` accepts (`true`/`false`, `1`/`0`, `t`/`f`), and anything else is a `400` rather
than a fall-through to the whole tree. Sending both is fine while they agree; `filter=all`
next to `interactive=true` is refused, because resolving that by a precedence rule the
caller cannot see would mean one of the two parameters it sent did nothing. The alias was
advertised on this endpoint for a long time without ever being read, so a raw HTTP caller
that followed the docs bought the full tree and was told nothing.

An unknown parameter NAME is reported rather than refused: `ignoredParams` on JSON and YAML
responses, an `# ignored params: ...` line on `compact` and `text` ones. `/action` can
refuse because its parameter set is the action request's own fields, and a caller sending
something else has genuinely asked for a behaviour that will not happen. `/snapshot` is a
read that newer clients call with parameters older servers have not learned yet, so
refusing would break version skew in the direction it normally occurs, while the disclosure
still ends the silence — which is what mattered, since the `quick` CLI command sent
`compact=true` for a long time, a parameter `/snapshot` has never read, and received the
JSON snapshot instead of the compact one it was written to request.

`GET /visible` (and `pinchtab visible <ref>`) answers CSS rendered-ness — `display`, `visibility`, `opacity`, and a laid-out box with non-zero size. Scroll position is not an input: an element far below the fold, or scrolled past, still reports `visible: true`. On-screen-ness is the response's `onScreen` field, which shares the capture snapshot's viewport-intersection predicate (see [reference/capture.md](reference/capture.md)); `onScreen` is omitted when the element could not be measured — absent means unknown, never "no".

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

`fill` and `type` write the string in `text`; `fill` also accepts it as `value`, which is the
field `select` reads. A `fill` carrying neither is rejected — send `"text": ""` to clear a
field, so clearing stays distinct from a request whose text never arrived.

`select` matches the `<option value="...">` attribute first and the option's visible text
second, so either spelling works. It draws the same absent-versus-supplied distinction as
`fill`: send `"value": ""` to select an `<option value="">` placeholder and reset the
dropdown, and a `select` carrying neither key is rejected. Every surface expresses it —
`POST /action` with `"value": ""`, `pinchtab select <ref> ""`, and the `pinchtab_select` MCP
tool with `value: ""`.

`button` accepts `left`, `right`, and `middle` — the same vocabulary the CLI's `--button`
help lists, tolerating case and surrounding whitespace, so `RIGHT` and ` middle ` are the
buttons they name. Any other value is refused with `400` `invalid_mouse_button` naming the
three, on any action body carrying the field: `primary`, `secondary` and `0` used to be
reinterpreted as `left` and reported as success. Omitting `button` means `left`, which is a
default rather than forgiveness for a name the server does not know.

`humanize` is a per-action override for input style. When omitted, actions use `instanceDefaults.humanize`, which defaults to `false`. Use `kind:"click"` or `kind:"type"` with `humanize:true` when a page needs the slower human-like pointer or typing path.

Pointer fallback behavior:

- `mouse-move` first attempts a real CDP `mouseMoved` event.
- If headless Chromium stalls that move waiting for renderer acknowledgement, PinchTab falls back to DOM `mouseover`/`mouseenter`/`mousemove` events at the same target so hover-style checks remain responsive.
- Non-timeout CDP errors and caller context cancellation are not hidden by the fallback.
- `mouse-wheel` dispatches a DOM `WheelEvent` at the target point and scrolls the window when the event is not cancelled.

Selector lookup is limited to the current frame scope. The default scope is `main`. Use `/frame` or `/tabs/{id}/frame` before selector-based iframe actions. Same-origin iframe scopes are supported; cross-origin iframe descendants are not currently exposed.

Snapshot query parameters:

- `filter`
- `interactive` (boolean alias for `filter`)
- `diff`
- `selector`
- `maxTokens`
- `depth`
- `format`
- `noAnimations`
- `output`

`selector` on `/snapshot` follows the same rule: it only searches the current frame scope. It does not automatically pierce into iframes, and cross-origin iframe descendants are not inlined.

Text query parameters:

- `mode=raw` (`mode=full` is an alias; any other value is a 400 naming the accepted ones)
- `format`
- `maxChars`
- `frameId`

`/text` default mode picks the first **visible** `<article>` / `[role="main"]` /
`<main>` (skips `display:none`) and strips nav/footer/ads. Use `mode=raw` for
full `innerText`, or `/snapshot` for structured UI text like prices and button
labels.

`mode=raw` and `mode=full` are the same extraction — the whole unfiltered page —
and are what the CLI's `--raw` and `--full` send. The default extraction keeps
block and table-cell boundaries: adjacent cells are separated by a tab and
adjacent blocks by a newline, so a status code and a timestamp in neighbouring
cells stay two fields rather than one number.

`/text` is also frame-aware. `frameId` targets a specific iframe for a one-shot
read; otherwise the endpoint inherits the tab's current `/frame` scope.

### The `frame` Disclosure On Scoped Reads

A `/frame` scope is per-tab server state, not a per-request argument: it survives every
later command until something clears it, so the caller who reads a scoped tab is often not
the one who scoped it. `/snapshot` and `/text` therefore publish the frame they were served
from:

```json
"frame": {
  "frameId": "886601397BFA0B332880152438BD0153",
  "frameUrl": "http://127.0.0.1:18798/inner.html",
  "frameName": "payment-frame",
  "frameTitle": "Inner",
  "ownerRef": "e3"
}
```

- The key is **absent** on a whole-document read, so nothing changes for an unscoped caller.
- It is published for a **one-shot `?frameId=` read too**, on a tab with no stored scope: the
  disclosure names the frame the read was actually served from, not whatever the tab happens
  to be scoped to. A one-shot read returns a fragment for the same reason a scoped one does,
  so it says so the same way.
- `frameUrl` and `frameTitle` are read from the frame at request time and are what the
  returned content belongs to; a frame that navigated since the scope was set reports where
  it is now.
- Top-level `url` and `title` keep their meaning in every response, scoped or not: they are
  the TAB's document. They are never re-pointed at the frame — a field that meant one thing
  usually and another under invisible state is the defect this disclosure exists to remove.
- `format=compact` and `format=text` carry the same fact in the header, as
  `# Outer | http://127.0.0.1:18798/ | frame e3 | 3 nodes`. The marker names the owner ref
  when one is known, because that is the handle `POST /frame` takes as a `target`; a raw
  frame id is not. Without a known ref it names a shortened frame id.
- The object is the one `GET /frame` returns under `frame`, plus `frameTitle`.

`/capture` publishes the same `frame` object on a scoped read, for the same reason: its
snapshot half is filtered to the scoped frame while top-level `url` and `title` name the tab
document.

`epoch.frameId` is **not** the scope and never was. It is the frame tree's ROOT id, taken
before the capture to pair the image with the DOM epoch it was shot against, and it holds
the same value whether or not a scope is set — so a scoped caller reading it is told the
content came from the main document. Read `frame.frameId` for the scope and `epoch.frameId`
for the epoch; they answer different questions and only agree when the tab is unscoped.

`/html` and `/styles` already disclose their frame as a top-level `frameId`, and their `url`
and `title` come from the frame's own document rather than the tab's, so a scoped read there
was never attributed to the parent.

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
GET  /annotate
GET  /tabs/{id}/annotate
GET  /capture
GET  /tabs/{id}/capture
GET  /pdf
POST /pdf
GET  /tabs/{id}/pdf
POST /tabs/{id}/pdf
GET  /screencast
GET  /screencast/tabs
GET  /instances/{id}/screencast
GET  /instances/{id}/proxy/screencast
POST /record/start
POST /record/stop
GET  /record/status
```

Screenshot query parameters:

- `tabId`
- `format=jpeg|png`
- `quality`
- `raw=true`
- `output=file`
- `noAnimations=true`
- `scale=<float>` — rescale the output bitmap (e.g. `0.5` = half size,
  `0.25` = quarter). Default `1`.

`/annotate` injects a persistent, clickable annotation overlay onto the live
page — one labelled box per interactive element — and leaves it there (the
`screenshot?annotate=true` overlay is transient and baked into the image
instead). Intended for headed browsers: clicking a label copies a reference
block (page, ref, role, accessible name, CSS selector, XPath) to the clipboard.
`?clear=true` removes it; `?selector=` scopes it. See
[Fix your website faster with an LLM](guides/annotate-for-llm-fixes.md).

Annotate query parameters:

- `tabId`
- `selector` — scope the overlay to elements within this selector
- `clear=true` — remove the overlay instead of injecting it

`/capture` returns a screenshot and an accessibility snapshot from the same
DOM epoch in a single call. It is the vision-grounded alternative to issuing
`/screenshot` and `/snapshot` back-to-back — the two unpaired calls share no
synchronization primitive, so the page can mutate between them and refs from
the snapshot can point at nodes that did not exist when the image was taken.

Capture query parameters:

- `tabId`
- `selector` — clips screenshot and filters snapshot subtree to the same element
- `filter=interactive|all`
- `depth` — snapshot max depth (default `-1` for full)
- `format=jpeg|png`
- `quality`
- `output=file|inline|raw` — default `file`
- `requirePair=true` — return `409 Conflict` when navigation is observed during the capture window
- `noAnimations=true`
- `scale=<float>` — rescale the output image via CDP's `clip.scale`.
  Default `1` (native pixels). `scale=0.5` halves each axis (quarter of
  the pixels). `image.devicePixelRatio` in the response tells you what
  your native DPR was, so you can compute CSS-pixel equivalence if you
  need to.
- `wait=stable|load|none` — default `stable`. `stable` waits for
  `Page.lifecycleEvent` quiescence (250ms of silence, 750ms ceiling) before
  opening the capture window so the screenshot and the AX-tree describe a
  settled page. `none` skips the wait. `load` is currently an alias for
  `none`; reserved for a future `document.readyState` gate.
- `withBounds=true|false` — default `true`. When on, every snapshot node
  with a non-zero backend node id gets a `boundingBox` field and a
  `visible` flag. `boundingBox` is the element's **border box** — the
  painted edge, the same rectangle `GET /box`, `screenshot?annotate=true`
  and `getBoundingClientRect` report, so a box can be cross-checked against
  any of them. It is not the content box: an element with a border or
  padding would otherwise report a rectangle inset from the edge a viewer
  identifies the control by. Each bounded node costs one `DOM.getBoxModel`
  round trip (~5ms); for the typical interactive-filter snapshot the budget
  is under 250ms. Pass `withBounds=false` to skip the per-node work.
- `beyondViewport=true|false` — default `false`. When on, the image spans
  the full document instead of just the visible viewport. The response
  sets `image.coordinateSpace` to `"document"` and bounding boxes are
  expressed in page (document) coordinates so they overlay the full image.
  When a `selector` is also supplied, the selector clip wins and
  `beyondViewport` is silently ignored — the same rule `/screenshot`
  enforces. Beyond-viewport captures force a layout pass that can resolve
  lazy images and fire `IntersectionObserver`; the AX-tree fetch and bounds
  harvest run after the screenshot so they reflect the post-reflow state.

The response carries `image.coordinateSpace`, `image.devicePixelRatio`,
and `image.viewport` ( `w`, `h`, `scrollX`, `scrollY` in CSS pixels at
capture time) so clients can translate between image pixels and
`boundingBox` values without guessing. Two axes have to be pinned for that
to hold: the coordinate ORIGIN, which `image.coordinateSpace` names
(`viewport` or `document`), and the box-model EDGE, which is always the
border box.

The response carries an `epoch.domEpoch` token cached on the tab's ref-cache.
Future client work can pass `expectedEpoch` to action endpoints to detect
stale refs at the use site; in P1 it is informational. `pairing.navigated`
is `true` when the main frame's `loaderId` changed mid-capture — that is the
only drift mode P1 detects. In-document churn (re-renders, observer
mutations) is the residual risk that later phases address.

Response shape:

```json
{
  "status": "ok",
  "tabId": "tab_abc",
  "url": "https://example.com",
  "title": "Example",
  "capturedAt": "2026-05-29T10:11:12.345Z",
  "epoch": { "frameId": "...", "loaderId": "...", "domEpoch": "ep_..." },
  "pairing": { "navigated": false, "captureDurationMs": 312 },
  "image": { "format": "jpeg", "path": "/.../captures/cap-...jpg", "bytes": 184223 },
  "snapshot": { "filter": "interactive", "nodeCount": 14, "nodes": [...] }
}
```

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

Record start body fields (JSON POST `/record/start`):

- `format`: `gif`, `webm`, or `mp4`.
- `fps`: Frames per second, 1-30 (default 5).
- `quality`: JPEG capture quality 1-100 (default 80).
- `scale`: Resolution multiplier (default 1.0).
- `tabId`: Target a specific tab.

Notes:

- Recording endpoints are gated by `security.allowScreencast`.
- `.webm` and `.mp4` formats require `ffmpeg` on the server PATH.
- `.gif` format uses pure Go encoding (always available).
- Only one recording per bridge instance.

## Downloads, Uploads, Cookies, And Clipboard

```text
GET  /download
GET  /tabs/{id}/download
POST /upload
POST /tabs/{id}/upload
GET  /cookies
POST /cookies
DELETE /cookies
GET  /tabs/{id}/cookies
POST /tabs/{id}/cookies
DELETE /tabs/{id}/cookies
GET  /clipboard/read
POST /clipboard/write
POST /clipboard/copy
GET  /clipboard/paste
POST /cache/clear
GET  /cache/status
```

Notes:

- download and upload endpoints are gated by `security.allowDownload` and `security.allowUpload`
- cookie endpoints (`GET/POST/DELETE /cookies`, plus tab-scoped variants) are gated by `security.allowCookies`
- download automatically decompresses `.gz` files and returns the decompressed content
- `security.downloadAllowedDomains` can whitelist specific domains (bypasses SSRF checks for those domains). Setting `["*"]` matches every host for the domain restriction, but grants no SSRF bypass — private-IP protection stays on. Name the internal host to reach it: `["docker-registry.internal"]`, `["10.0.0.5"]`, or `["*", "10.0.0.5"]`.
- clipboard endpoints are gated by `security.allowClipboard`
- upload uses a JSON body with `selector`, `files`, and optional `fileNames`
- `fileNames` is index-aligned with `files` and sets the name the page sees in `file.name` — send it, or every upload arrives as `upload-<i>.bin` and forms gating on `accept=".csv"` or `file.name.endsWith(...)` reject it. Without a name the extension is sniffed from content, which cannot identify text formats (`.csv`, `.json`, `.txt`, `.md`, `.html`) because they have no magic bytes. A supplied name wins over the sniffed type even when the two disagree, matching what a browser sends. Only the basename is used: any directory part is dropped.

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

## State

```text
GET    /state
GET    /state/list
GET    /state/show
POST   /state/save
POST   /state/load
DELETE /state
POST   /state/clean
```

`GET /state` returns the current full browser state for the current tab or an explicit `tabId`, including cookies, current-origin storage, metadata, and basic tab information.

`/state/save|load|list|show|delete|clean` manage persisted saved browser state on disk.

This is different from `GET /tabs/{id}/state`, which returns live tab/page runtime state for readiness and blocking checks.

Notes:

- All state and storage endpoints are gated by `security.allowStateExport`: `/storage`, `/tabs/{id}/storage`, `GET /state`, `GET /state/list`, `GET /state/show`, `POST /state/save`, `POST /state/load`, `DELETE /state`, and `POST /state/clean`
- state files are stored in `{stateDir}/sessions/` with `0600` permissions
- optional AES-256-GCM encryption via `security.stateEncryptionKey` config setting
- storage is captured only for the current origin (active tab)

`GET /state` query parameters:

- `tabId` — optional tab identifier; when omitted, uses the current tab

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

## Tab State

```text
GET /tabs/{id}/state
```

Returns lightweight live tab/page runtime state for a tab, including load state, dialog presence, and actionability.

Use it as a cheap readiness probe before actions. Keep the detailed semantics in the API/skill references rather than here.

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
- `bodyMode=auto|retained-preferred|retained-only|live-only` on detail requests to choose how response bodies are resolved
- `timeoutMs` on detail requests to bound the retained-body wait window (default 2000, max 30000)

Response body behavior for network detail/export:

- by default, response bodies are fetched on demand from live CDP state and may no longer be available for older requests
- when `server.retainNetworkBodies=true`, PinchTab opportunistically retains bounded response bodies in the in-memory network buffer and returns the retained body first
- `bodyMode=retained-preferred` waits briefly for pending retained-body capture before falling back to live CDP
- `bodyMode=retained-only` never falls back to live CDP and returns explicit pending/skipped/error state instead
- detail responses may expose `bodySource=retained|live` to distinguish which path produced the returned body
- retained-body detail responses may expose `bodyPending=true` while capture is still in flight, or `bodySkipped=true` with `bodySkipReason` when retention was not completed — either skipped up front (retention disabled, the tab's retention budget exhausted, concurrency limit reached) or because an over-budget base64 body was dropped rather than cut
- retained bodies are capped twice: per body by `server.retainNetworkBodyMaxBytes` (`bodySkipReason` says "retention limit") and by the tab's remaining retention buffer ("retention budget"). An oversized text body is truncated to a byte-exact prefix and marked `bodyTruncated=true`; an oversized base64 body is dropped entirely with `bodySkipped=true` and the reason, because a base64 fragment is undecodable
- `base64Encoded=true` marks the returned body (retained or live) as base64 — decode it before use. The field is omitted, never `false`, for a text body, so its presence is what to branch on. Both caps measure the encoded length, so a binary response has an effective raw budget of roughly three quarters of the configured bytes
- retained responses may include `bodyRetained=true`

Request body (`postData`) behavior:

- `postData` holds the request body as the page sent it, decoded. Chrome delivers it base64-encoded and split into chunks; PinchTab decodes and joins it, so no base64 decoding is needed by the caller
- it is capped at 64 KiB of decoded body, cut on a character boundary, and a cut body is marked `postDataTruncated=true` — without it a clipped request body reads as the body the client sent
- it is omitted when the body is not text — a binary part in a multipart upload, for example — because the field carries no encoding marker. An omitted body says why: `postDataSkipped=true` with `postDataSkipReason` ("request body entry is not base64", "request body is not valid UTF-8"), so an absent `postData` is never mistaken for a request sent without one
- `postDataTruncated` and `postDataSkipped` are different answers and never both set: truncated means cut but usable, skipped means there is no body to read. A request that simply had no body carries neither flag
- HAR export puts the same decoded value in `request.postData.text`, and omits the block entirely when there is no publishable body

Network export query parameters:

- `format` — `har` (default) or `ndjson`. Pluggable: new formats register at startup.
- `output=file` — save to disk instead of streaming to response
- `path` — filename when `output=file` (auto-generated if omitted, required for `/export/stream`)
- `body=true` — include response bodies (fetched on demand by default; retained-body mode can make this durable for bounded entries)
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

A named `solver` that cannot run is rejected with `400` before anything is
solved, and the two reasons carry different codes:

| Code | Meaning | Example message |
| --- | --- | --- |
| `unknown_solver` | No solver answers to that name — normally a misspelling. Lists what is available. | `unknown solver "cloudlfare" (available: [cloudflare semantic jschallenge])` |
| `solver_key_missing` | A known key-gated solver whose API key is unset. Names the config key to set. | `solver "capsolver" is configured but its API key is not set; set autoSolver.external.capsolverKey to use it` |

The API is deliberately stricter here than config validation, which accepts
`capsolver` or `twocaptcha` in `autoSolver.solvers` with no key set — configuring
a paid solver before its key is legitimate ordering, and the run falls back to
the solvers that can run. A request naming one solver has no such fallback: it
must not silently run a different solver, so it is rejected and told which key
would enable it.

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
GET  /instances/{id}/metrics
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
- `/instances/launch` is a sibling endpoint of `/instances/start` (separate handler `handleLaunchByName`), kept for the launch-by-profile workflow; `name` on the body is no longer supported, profiles must already exist
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

Agent session routes are only present in full server mode with `sessions.agent.enabled` true. The family always answers, so the state is readable from the error code rather than from a bare 404: a bridge returns `sessions_unavailable_bridge_mode`, whose remedy is to run `pinchtab server`, and a full server with the setting off returns `sessions_disabled`. No config value mounts the family in bridge mode.

`sessions_disabled` covers two states, and its `details.hint` says which one. A server that BOOTED with the setting off never mounted the family, so enabling it needs `pinchtab config set sessions.agent.enabled true` and a restart, and the refusal carries no `details.remedy` because that is not one command. A server that booted with it on and was switched off by a config save already mounted the family: `pinchtab config set sessions.agent.enabled true` applies live, needs no restart, and is carried as the refusal's `details.remedy`.

Session-authenticated callers cannot reach dashboard/admin endpoint families such as config, dashboard agent listings, dashboard event streams, session management, profile management, instance management, or cache controls. They are intended for trusted automation in controlled environments, not for untrusted multi-tenant isolation.

## Feature Gates

Some endpoints are intentionally disabled unless the matching config allows them:

These gates are not ordinary feature toggles. Enabling them is a documented, non-default, security-reducing choice that widens the control surface available to callers.

- `/evaluate` and `/tabs/{id}/evaluate` -> `security.allowEvaluate`
- `/download` and `/tabs/{id}/download` -> `security.allowDownload`
- `GET/POST/DELETE /cookies` and `GET/POST/DELETE /tabs/{id}/cookies` -> `security.allowCookies`
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

### Refusal guidance: `details.hint` and `details.remedy`

A refusal that a caller can act on carries a `details` object with two fields. They are
different kinds of answer and neither substitutes for the other:

- `hint` — prose for a human or a model to read. Explanations, alternatives, preconditions
  and anything that is not a single command live here.
- `remedy` — **one line a shell accepts**, so an agent can run it verbatim without parsing
  English.

`remedy` guarantees all of the following:

- one line, one or more `pinchtab` invocations, joined with `&&` when more than one is needed.
  `$(...)` command substitution is allowed and is used where a value has to be read back
  first — widening the domain allowlist appends to the current one rather than replacing it
- no prose connectives (`then:`, `or`, a parenthetical tail), no pipes, semicolons,
  redirections, backquotes, comments or brace expansion. `pinchtab dialog accept|dismiss`
  is not two suggestions to a shell — it is a pipeline into a command named `dismiss` — so a
  line like that is not a remedy and never appears in the field
- every command and flag in it exists in the CLI
- a free slot is a `<name>` placeholder, the same angle-bracket convention the CLI's own
  `--help` uses, and nothing else. Values known when the refusal is produced are already
  interpolated, so a placeholder means the value genuinely is the caller's to supply
- **the field is absent when no single command fixes the refusal.** Absence is the answer,
  not an omission: it says truthfully that there is nothing to run, and the guidance for that
  case is in `hint`. Do not treat a missing `remedy` as an error in the response

```json
{
  "error": "this endpoint requires the evaluate capability; enable security.allowEvaluate in config to use it",
  "code": "evaluate_disabled",
  "details": {
    "setting": "security.allowEvaluate",
    "hint": "Enable security.allowEvaluate to use this feature.",
    "remedy": "pinchtab config set security.allowEvaluate true && pinchtab server restart"
  }
}
```

`details` may carry further machine-readable fields beside these two — the capability refusal
above names the `setting`, an allowlist block names the blocked `url` and `domain` — so read
the object by key rather than assuming it holds only guidance.

`pinchtab` renders both fields when a request fails, printing `remedy` into a `Remedy:` line.
Every value in that slot meets the contract above, whether it came from the server or from
the CLI's own client-side refusals.
