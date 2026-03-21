# Endpoints Reference

This page lists the live HTTP surface exposed by PinchTab. Some routes are only available in bridge mode, some only in full server mode, and some are gated by security settings.

## Health And Server Metadata

```text
GET  /health
POST /ensure-chrome
GET  /help
GET  /openapi.json
GET  /metrics
GET  /welcome
POST /shutdown
GET  /api/events
```

Notes:

- in bridge mode, `/health` reports bridge health and tab count
- in full server mode, `/health` reports dashboard health, auth state, and instance count
- `/metrics` in full server mode is a server metrics snapshot, not the bridge memory view

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
POST /tab/lock
POST /tab/unlock
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
```

Notes:

- download and upload endpoints are gated by `security.allowDownload` and `security.allowUpload`
- clipboard endpoints are gated by `security.allowClipboard`
- upload uses a JSON body with `selector` and `files`

## Wait, Network, Dialog, Console, And Errors

```text
POST /wait
POST /tabs/{id}/wait
GET  /network
GET  /network/stream
GET  /network/{requestId}
POST /network/clear
GET  /tabs/{id}/network
GET  /tabs/{id}/network/stream
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

Dialog body fields:

- `action`: `accept` or `dismiss`
- `text`: optional prompt text
- `tabId`: optional on `/dialog`

Console and error routes use query parameters:

- `tabId`
- `limit`

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

Scheduler routes are only present when `scheduler.enabled` is true.

## Feature Gates

Some endpoints are intentionally disabled unless the matching config allows them:

- `/evaluate` and `/tabs/{id}/evaluate` -> `security.allowEvaluate`
- `/download` and `/tabs/{id}/download` -> `security.allowDownload`
- `/upload` and `/tabs/{id}/upload` -> `security.allowUpload`
- clipboard routes -> `security.allowClipboard`
- attach routes -> `security.attach`
- screencast routes -> `security.allowScreencast`
