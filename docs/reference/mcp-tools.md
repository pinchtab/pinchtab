# MCP Tool Reference

PinchTab currently exposes 47 MCP tools. All tool names are prefixed with `pinchtab_` and are served over stdio JSON-RPC.

For selector-based interaction tools, prefer `selector`. `ref`, `element` and `target` are still accepted as deprecated aliases, and `query` as a fallback, on the element-action tools (`query` is shorthand for `find:<text>`).

Every tool refuses an argument its schema does not declare, naming the unknown key, the nearest declared name when one is close, and the declared arguments; nothing runs. Deprecated spellings stay declared with a description naming the canonical one.

If you allow MCP browsing on non-local or non-trusted domains, treat `pinchtab_snapshot` and `pinchtab_get_text` output as untrusted page data. Those tools can surface hostile prompt text from visited pages; operators should keep IDPI/domain restrictions narrow unless wider access is intentional.

Selector forms include:

- `e5`
- `#login`
- `xpath://button`
- `text:Submit`
- `find:login button`
- `role:button Save`
- `label:Email`, `placeholder:Search`, `alt:Logo`, `title:Close`, `testid:submit`
- `first:button`, `last:button`, `nth:2:button`

Positional wrappers index the candidates in **document order** for every selector kind, so `nth:0:` is the first match in the page and `nth:1:` always comes after it. A bare `text:` selector is the one form that does not index: it picks the most control-like match among the smallest ones, so `text:Save` prefers a `<button>` over a `<div>` carrying the same label — which means `text:X` and `first:text:X` can resolve to different elements. A wrapper only ever chooses among the matches a bare selector would find; it never changes which matches exist.

Structured semantic locators are matched by the semantic engine; CSS, XPath, refs, the existing `text:` action selector, and bare CSS/text wrappers stay browser-side.

Most browser-facing tools also declare an optional `browser` argument (e.g. `chrome`, `cloak`, `ghost-chrome`) that selects the browser for that one request; the tables below list it only where it changes behaviour.

Ref vocabulary: the MCP server remembers the `X-PinchTab-Vocab` token returned by `pinchtab_snapshot`, `pinchtab_find`, `pinchtab_extract`, `pinchtab_a11y_audit` and any `snap=true` follow-up snapshot, keyed by the `tabId` argument, and sends it as `vocab` on the element-action tools (`pinchtab_click`, `pinchtab_type`, `pinchtab_hover`, `pinchtab_focus`, `pinchtab_select`, `pinchtab_scroll`, `pinchtab_scroll_into_view`, `pinchtab_fill`). A ref from a snapshot that a newer one has renumbered is refused with `409 vocab_superseded`; re-snapshot and use the new refs.

## Navigation

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_navigate` | `url` required, `tabId`, `newTab`, `snap`, `browser` | Uses `/navigate`; reuses the current tab, and `newTab=true` opens one (as does a first navigate with no tab yet). The returned `tabId` targets the new tab in later tools. `snap=true` returns an interactive compact snapshot in the same response |
| `pinchtab_back` | `tabId`, `snap`, `browser` | Uses `/back` (or `/tabs/{id}/back`); returns `{tabId, url, navigated}` with the URL the tab landed on; `navigated: false` means the tab was at the edge of history and did not move; `snap=true` appends an interactive compact snapshot |
| `pinchtab_forward` | `tabId`, `snap`, `browser` | Uses `/forward`; same response shape as `pinchtab_back` |
| `pinchtab_reload` | `tabId`, `snap`, `browser` | Uses `/reload`; returns `{tabId, url}` and omits `navigated`; `snap=true` appends an interactive compact snapshot |
| `pinchtab_snapshot` | `tabId`, `interactive`, `compact`, `format`, `diff`, `selector`, `maxTokens`, `depth`, `noAnimations` | Returns compact by default — the tool asks `/snapshot` for `format=compact` unless `format` says otherwise or `compact=false` asks for the JSON tree. `selector` scopes the snapshot; `format` is limited to `compact` or `text` |
| `pinchtab_frame` | `tabId`, `target` | Get or set the frame scope for selector-based actions on the tab; `target` accepts `main`, a snapshot ref, an iframe selector, or a frame name/URL |
| `pinchtab_screenshot` | `tabId`, `selector`, `scale`, `format`, `quality`, `annotate`, `beyondViewport`, `browser` | `selector` captures a specific element in current frame scope; `scale` rescales the output bitmap (e.g. `0.5` = half size); `format` is `jpeg` or `png`; `annotate=true` overlays numbered ref boxes and populates the annotations envelope; `beyondViewport=true` captures the full scrollable document (ignored when `selector` is set) — box coords are document-relative in that mode; `browser` selects the browser (e.g. `chrome`, `cloak`) for this request |
| `pinchtab_capture` | `tabId`, `selector`, `filter`, `format`, `quality`, `depth`, `scale`, `wait`, `withBounds`, `beyondViewport`, `requirePair`, `noAnimations`, `browser` | Paired screenshot + accessibility snapshot from the same DOM epoch. Returns an image content block plus a JSON envelope with `epoch`, `pairing.navigated`, per-node `boundingBox`, and `image.coordinateSpace` (`viewport`, `document`, or selector `clip`). `browser` selects the browser (e.g. `chrome`, `cloak`); the static ghost-chrome runtime cannot paint, so it falls back to chrome. Use when the model reads pixels AND acts on refs in the same turn. |
| `pinchtab_get_text` | `tabId`, `mode`, `raw`, `format`, `maxChars` | `mode` is `readability` (default), `raw` or `markdown` and supersedes `raw`; `raw=true` alone maps to `/text?mode=raw`; `format=text/plain` returns plain text; inherits the current `pinchtab_frame` scope for that tab |

## Interaction

All element-action tools accept the unified `selector`, its deprecated aliases `ref`, `element` and `target`, and `query` (semantic shorthand). Deprecated value aliases: `onDialog`/`promptText` for `dialogAction`/`dialogText` on click, `value` for `text` on type, `option` for `value` on select, `text` for `value` on fill. Every one of them also accepts `nodeId` (a backend node ID from a snapshot) as an alternative target. Coordinates (`x`/`y`) are accepted only by `pinchtab_click`, `pinchtab_hover` and `pinchtab_scroll` — the other kinds have no coordinate behaviour.

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_click` | `selector`, `ref`, `query`, `tabId`, `x`, `y`, `nodeId`, `dialogAction`, `dialogText`, `waitNav`, `mode`, `humanize`, `snap` | Click element by selector or coordinate; `mode` accepts `dom` or `dispatch` as a broad low-level escape hatch for click delivery; `humanize` overrides the instance default for this click (omit to inherit it); `mode` and `humanize=true` are mutually exclusive; `dialogAction` handles a dialog opened by the click; `waitNav=true` waits for navigation; `snap=true` returns a snapshot |
| `pinchtab_type` | `selector`, `ref`, `query`, `nodeId`, `text` required, `tabId` | Sends key events at the targeted input; target with `selector` or `nodeId` |
| `pinchtab_hover` | `selector`, `ref`, `query`, `tabId`, `x`, `y`, `nodeId`, `humanize` | Hover an element or coordinate; `humanize` overrides the instance default for this hover |
| `pinchtab_focus` | `selector`, `ref`, `query`, `tabId`, `nodeId` | Focus element |
| `pinchtab_select` | `selector`, `ref`, `query`, `nodeId`, `value` required, `tabId`, `snap` | Select `<option>` by value or visible text; target with `selector` or `nodeId` |
| `pinchtab_scroll` | `selector`, `ref`, `query`, `nodeId`, `pixels`, `deltaX`, `deltaY`, `direction`, `steps`, `x`, `y`, `tabId` | Omit every target to scroll the page; element + `pixels` uses wheel semantics; `direction` accepts `down`/`left`/`right`/`up` and moves 800px per step — the same distance as the CLI's `pinchtab scroll <direction>`, multiplied by `steps` or overridden by `pixels` |
| `pinchtab_scroll_into_view` | `selector`, `ref`, `query`, `nodeId`, `tabId` | Scrolls the target into view and returns geometry for stable follow-up actions; target with `selector` or `nodeId` |
| `pinchtab_fill` | `selector`, `ref`, `query`, `nodeId`, `value` required, `tabId`, `snap` | Direct fill via JS dispatch instead of keystrokes; target with `selector` or `nodeId`. An empty `value` clears the field; omitting it entirely is refused |

## Keyboard

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_key` | `action` required, `key`, `text`, `nodeId`, `tabId` | One keyboard tool. `action=press` presses a key such as `Enter` (`nodeId` focuses that node first, otherwise the key goes to the focused element); `down` holds a key; `up` releases it; `type` types at the focused element with key events; `insert` is a paste-like insert without key events. `press`/`down`/`up` need `key`, `type`/`insert` need `text`. `nodeId` is honoured by `press` only |

## Content

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_eval` | `expression` required, `awaitPromise`, `tabId` | `awaitPromise=true` resolves a returned Promise (without it a Promise returns `{}` with a hint). Requires `security.allowEvaluate` (documented non-default JS-execution opt-in). Not frame-scoped — current `pinchtab_frame` state does not change evaluation context |
| `pinchtab_pdf` | `tabId`, `landscape`, `scale`, `pageRanges` | Returns base64-encoded PDF content |
| `pinchtab_find` | `query` required, `tabId` | Natural-language element search via `/find`; the result adds `bestRef`, `selector` (both the `best_ref`) and a `nextActionHint` to reuse in action tools. When `tied_refs` is present, `bestRef` and `selector` are withheld and the hint names the tied refs and the `retry_query` when the response has one |
| `pinchtab_extract` | `schema` (object) required, `tabId`, `scope`, `maxItems` | Schema-typed data via `/extract`: `data` plus a per-field `ref`, `score` and `confidence`; the refs are usable in action tools straight away. Prefer it over snapshot-then-parse for structured values; pin a `low` field with `x-pinchtab-hint`. See [Extract](./extract.md) |

## Diagnostics

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_console` | `tabId`, `clear` | Reads the tab's captured console logs via `/console`; `clear=true` posts `/console/clear` instead of reading |
| `pinchtab_errors` | `tabId`, `clear` | Reads the tab's uncaught JavaScript errors via `/errors`; `clear=true` posts `/errors/clear` instead of reading |
| `pinchtab_a11y_audit` | `tabId`, `engine`, `tags`, `rules`, `includeIncomplete`, `browser` | Accessibility audit via `/a11y/audit`. `engine` is `native` (default) or `axe` (any other value is a 400); `tags`, `rules` and `includeIncomplete` apply to `axe` only, and `rules` wins over `tags`. See [A11y](./a11y.md) |
| `pinchtab_memory` | `tabId`, `gc`, `browser` | JS heap usage and DOM counters via `/memory`; `gc=true` collects garbage first |
| `pinchtab_memory_snapshot` | `tabId`, `top`, `browser` | Takes a V8 heap snapshot (`POST /memory/snapshot`) and returns `{id, path, bytes, summary}`; `top` rows per summary table (default 20). Needs `security.allowMemory`. See [Memory](./memory.md) |
| `pinchtab_memory_compare` | `base` required, `head` required, `top`, `retained`, `browser` | Diffs two `pinchtab_memory_snapshot` ids via `/memory/compare`: per-constructor count and self-size deltas plus new duplicate strings; `top` rows (default 20); `retained=true` adds retained sizes (slower). Needs `security.allowMemory` |

## Site

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_scrape` | `url` required, `preview`, `only`, `maxPages`, `maxPerPattern`, `include`, `exclude`, `concurrency`, `enrichAll`, `noBrowser`, `timeoutSeconds`, `browser` | Crawl a whole site to a page tree of markdown via `/scrape`. HTTP-first extraction; only thin/blocked/failed pages are browser-rendered. `preview=true` returns a cheap outline (sizes + snippets, no bodies, no browser); `only` (comma-separated URLs) expands chosen pages at full fidelity. `include`/`exclude` are comma-separated regexes. Full reports can be large — prefer preview then expand. Runs with an extended timeout (multi-page crawls take minutes). `timeoutSeconds` is a crawl budget and the one seconds-unit argument; waits elsewhere use `timeoutMs` |

## Tab Management

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_list_tabs` | none | Lists open tabs |
| `pinchtab_close_tab` | `tabId` optional | Closes the given tab, or the current/default tab when omitted |
| `pinchtab_health` | none | Checks server health |
| `pinchtab_cookies` | `tabId` | Reads cookies for a tab; requires `security.allowCookies` |
| `pinchtab_cookies_set` | `name`, `value` required; `url`, `domain`, `path`, `sameSite`, `secure`, `httpOnly`, `expires`, `tabId` optional | Sets one cookie for session reuse; `url` defaults to the tab's current page and an empty `value` blanks the cookie; requires `security.allowCookies` |
| `pinchtab_connect_profile` | `profile` required | Returns the connect URL and instance status for a profile |

## Human Handoff

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_handoff` | `tabId` required; `reason`, `timeoutMs` optional | Pauses the tab for a human (CAPTCHA, login, consent); action tools on it answer `409 tab_paused_handoff` until resumed; `timeoutMs` auto-resumes |
| `pinchtab_resume` | `tabId` required; `status` optional | Resumes the tab; call only after the user confirms they finished the manual step |
| `pinchtab_handoff_status` | `tabId` required | Reports `paused_handoff` with `reason`, `pausedAt` and `expiresAt`, or `active` |

## Wait Utilities

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_wait` | `for` required, `value` required, `timeoutMs`, `state`, `tabId`, `browser` | One wait tool: `for` names the condition and `value` carries it. `timeoutMs` bounds a browser-backed condition (default 10000, max 30000); `timeout` is its deprecated alias. `for=ms` is a fixed-duration wait capped at 30000 ms; `selector` waits for an element (`state` is `visible` (default; a match is rendered), `hidden` (nothing matches or no match is rendered), `attached` (a match is in the DOM) or `detached` (nothing matches)); `text` waits for body text; `url` is a URL glob match; `load` is `ready-state` (`readyState=complete`), `content-loaded` (`readyState` in `{interactive, complete}`), or `network-idle` (0 in-flight requests for 500 ms, counted from the later of the last request change and the wait call, so an already-quiet page returns after 500 ms plus one 100 ms poll); `function` waits for a JS expression to become truthy |

## Network

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_network` | `tabId`, `filter`, `method`, `status`, `type`, `limit`, `bufferSize` | Lists recent network requests |
| `pinchtab_network_detail` | `requestId` required, `tabId`, `body` | `body=true` includes response body when available |
| `pinchtab_network_clear` | `tabId` | Clears one tab or all tabs when omitted |
| `pinchtab_network_route` | `tabId` required, `pattern` required, `action`, `body`, `contentType`, `status`, `resourceType`, `method` | Install a request-interception rule on a tab. `action` is `continue` (default), `abort`, or `fulfill`. `fulfill` is blocked on hosts in `security.allowedDomains` and falls through to a real fetch on those hosts |
| `pinchtab_network_unroute` | `tabId` required, `pattern` | Remove a tab's interception rule by pattern, or all rules when `pattern` is omitted |
| `pinchtab_network_rules` | `tabId` required | List a tab's interception rules as `{tabId, rules}`, each rule naming its `pattern` and `action` (`continue`, `abort`, `fulfill`). Rules survive navigation; an empty `rules` list means the tab mocks and blocks nothing |

## Recording

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_record` | `action` required, `file`, `fps`, `quality`, `scale`, `tabId` | One recording tool. `fps` 1-30 (default 5), `quality` 1-100 (default 80), `scale` up to 1.0 (default 1.0). `action=start` starts recording (format inferred from the `file` extension — `.gif`, `.webm`, `.mp4`; requires `security.allowScreencast`; GIF works without ffmpeg); `stop` encodes and saves to `file` (the same path given to `start`), which may take a while for long recordings; `status` returns the active recording status (format, fps, duration, frame count) |

## Dialog

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_dialog` | `action` required, `text`, `tabId` | `action` is `accept` or `dismiss`; `text` is used as the prompt response with `accept` |

## Return Shapes

Typical results:

- navigation tools return JSON from the matching HTTP endpoint
- `pinchtab_snapshot` returns text for `compact`/`text` formats and JSON otherwise
- `pinchtab_get_text` returns plain text when `format=text|plain`, JSON otherwise
- `pinchtab_screenshot` returns an MCP image content block (image/jpeg by default, image/png when `format=png`) plus a text block that is always the JSON envelope `{"format", "annotations": [...]}` — `annotations` is `[]` by default and `[{"ref","role","name","tag","box":{"x","y","w","h"}}, ...]` when `annotate=true`
- `pinchtab_pdf` returns JSON containing a base64-encoded PDF payload
- wait tools return wait status JSON
- network tools return the same request logs you would see from `/network`

Security note:

- extracted text and snapshot content should be treated as untrusted content from the visited page, not as trusted instructions
- widening IDPI allowlists or disabling strict protections increases the chance that prompt-injection text reaches downstream agent logic

For setup and client configuration, see [MCP Server](../mcp.md).

Saved browser state is intentionally not exposed as MCP tools right now. Use the CLI or HTTP API for `GET /state`, `pinchtab state`, and saved-state persistence operations.
