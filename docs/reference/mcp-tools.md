# MCP Tool Reference

PinchTab currently exposes 36 MCP tools. All tool names are prefixed with `pinchtab_` and are served over stdio JSON-RPC.

For selector-based interaction tools, prefer `selector`. `ref` and `query` are still accepted as deprecated/alias fallbacks on the element-action tools (`query` is shorthand for `find:<text>`).

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

## Navigation

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_navigate` | `url` required, `tabId`, `newTab`, `snap` | Uses `/navigate`; omitting `tabId` opens a new tab. `newTab=true` opens the URL in a NEW tab and returns its `tabId`, leaving the current tab on its page — this is how an agent holds two pages at once. `snap=true` returns an interactive compact snapshot in the same response |
| `pinchtab_snapshot` | `tabId`, `interactive`, `compact`, `format`, `diff`, `selector`, `maxTokens`, `depth`, `noAnimations` | Returns compact by default — the tool asks `/snapshot` for `format=compact` unless `format` says otherwise or `compact=false` asks for the JSON tree. `selector` scopes the snapshot; `format` is limited to `compact` or `text` |
| `pinchtab_frame` | `tabId`, `target` | Get or set the frame scope for selector-based actions on the tab; `target` accepts `main`, a snapshot ref, an iframe selector, or a frame name/URL |
| `pinchtab_screenshot` | `tabId`, `selector`, `scale`, `format`, `quality`, `annotate`, `beyondViewport`, `browser` | `selector` captures a specific element in current frame scope; `scale` rescales the output bitmap (e.g. `0.5` = half size); `format` is `jpeg` or `png`; `annotate=true` overlays numbered ref boxes and populates the annotations envelope; `beyondViewport=true` captures the full scrollable document (ignored when `selector` is set) — box coords are document-relative in that mode; `browser` selects the browser (e.g. `chrome`, `cloak`) for this request |
| `pinchtab_capture` | `tabId`, `selector`, `filter`, `format`, `quality`, `depth`, `scale`, `wait`, `withBounds`, `beyondViewport`, `requirePair`, `noAnimations`, `browser` | Paired screenshot + accessibility snapshot from the same DOM epoch. Returns an image content block plus a JSON envelope with `epoch`, `pairing.navigated`, per-node `boundingBox`, `image.coordinateSpace` (`viewport`, `document`, or selector `clip`), the `frame` scope disclosure when the capture was scoped to a subframe, and the IDPI keys (see the security note). `browser` selects the browser (e.g. `chrome`, `cloak`); the static ghost-chrome runtime cannot paint, so it falls back to chrome. Use when the model reads pixels AND acts on refs in the same turn. |
| `pinchtab_get_text` | `tabId`, `raw`, `format`, `maxChars` | `raw=true` maps to `/text?mode=raw`; `format=text/plain` returns plain text; inherits the current `pinchtab_frame` scope for that tab |

## Interaction

All element-action tools accept the unified `selector` and the legacy aliases `ref` (deprecated) and `query` (semantic shorthand). Every one of them also accepts `nodeId` (a backend node ID from a snapshot) as an alternative target. Coordinates (`x`/`y`) are accepted only by `pinchtab_click`, `pinchtab_hover` and `pinchtab_scroll` — the other kinds have no coordinate behaviour.

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_click` | `selector`, `ref`, `query`, `tabId`, `x`, `y`, `nodeId`, `dialogAction`, `dialogText`, `waitNav`, `mode`, `snap` | Click element by selector or coordinate; `mode` accepts `dom` or `dispatch` as a broad low-level escape hatch for click delivery; `mode` and `humanize` are mutually exclusive; `dialogAction` handles a dialog opened by the click; `waitNav=true` waits for navigation; `snap=true` returns a snapshot |
| `pinchtab_type` | `selector`, `ref`, `query`, `nodeId`, `text` required, `tabId` | Sends key events at the targeted input; target with `selector` or `nodeId` |
| `pinchtab_hover` | `selector`, `ref`, `query`, `tabId`, `x`, `y`, `nodeId` | Hover an element or coordinate |
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
| `pinchtab_eval` | `expression` required, `tabId` | Requires `security.allowEvaluate` (documented non-default JS-execution opt-in). Not frame-scoped — current `pinchtab_frame` state does not change evaluation context |
| `pinchtab_pdf` | `tabId`, `landscape`, `scale`, `pageRanges` | Returns base64-encoded PDF content |
| `pinchtab_find` | `query` required, `tabId` | Semantic element search; returns `best_ref` and selector hints to reuse in action tools |

## Site

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_scrape` | `url` required, `preview`, `only`, `maxPages`, `maxPerPattern`, `include`, `exclude`, `concurrency`, `enrichAll`, `noBrowser`, `timeoutSeconds`, `browser` | Crawl a whole site to a page tree of markdown via `/scrape`. HTTP-first extraction; only thin/blocked/failed pages are browser-rendered. `preview=true` returns a cheap outline (sizes + snippets, no bodies, no browser); `only` (comma-separated URLs) expands chosen pages at full fidelity. `include`/`exclude` are comma-separated regexes. Full reports can be large — prefer preview then expand. Runs with an extended timeout (multi-page crawls take minutes) |

## Tab Management

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_list_tabs` | none | Lists open tabs |
| `pinchtab_close_tab` | `tabId` optional | Closes the given tab, or the current/default tab when omitted |
| `pinchtab_health` | none | Checks server health |
| `pinchtab_cookies` | `tabId` | Reads cookies for a tab; requires `security.allowCookies` |
| `pinchtab_cookies_set` | `name`, `value` required; `url`, `domain`, `path`, `sameSite`, `secure`, `httpOnly`, `expires`, `tabId` optional | Sets one cookie for session reuse; `url` defaults to the tab's current page and an empty `value` blanks the cookie; requires `security.allowCookies` |
| `pinchtab_connect_profile` | `profile` required | Returns the connect URL and instance status for a profile |

## Wait Utilities

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_wait` | `for` required, `value` required, `timeout`, `state`, `tabId` | One wait tool: `for` names the condition and `value` carries it. `for=ms` is a fixed-duration wait capped at 30000 ms; `selector` waits for an element (`state` is `visible` (default) or `hidden`); `text` waits for body text; `url` is a URL glob match; `load` is `ready-state` (`readyState=complete`), `content-loaded` (`readyState` in `{interactive, complete}`), or `network-idle` (0 in-flight requests for 500 ms); `function` waits for a JS expression to become truthy |

## Network

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_network` | `tabId`, `filter`, `method`, `status`, `type`, `limit`, `bufferSize` | Lists recent network requests |
| `pinchtab_network_detail` | `requestId` required, `tabId`, `body` | `body=true` includes response body when available |
| `pinchtab_network_clear` | `tabId` | Clears one tab or all tabs when omitted |
| `pinchtab_network_route` | `tabId` required, `pattern` required, `action`, `body`, `contentType`, `status`, `resourceType`, `method` | Install a request-interception rule on a tab. `action` is `continue` (default), `abort`, or `fulfill`. `fulfill` is blocked on hosts in `security.allowedDomains` and falls through to a real fetch on those hosts |
| `pinchtab_network_unroute` | `tabId` required, `pattern` | Remove a tab's interception rule by pattern, or all rules when `pattern` is omitted |

## Recording

| Tool | Key Parameters | Notes |
| --- | --- | --- |
| `pinchtab_record` | `action` required, `file`, `fps`, `quality`, `scale`, `tabId` | One recording tool. `action=start` starts recording (format inferred from the `file` extension — `.gif`, `.webm`, `.mp4`; requires `security.allowScreencast`; GIF works without ffmpeg); `stop` encodes and saves to `file`, which may take a while for long recordings; `status` returns the active recording status (format, fps, duration, frame count) |

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

When the server marks page-derived content as untrusted, the tool result carries the
signal in two forms. The response payload keeps `idpiWarning` (what tripped the guard, and
only when something did), `untrustedContent` and `idpiNotice`, exactly as the HTTP API
publishes them. And the notice arrives as **its own text block, ahead of the block carrying
the page content**, so the trust boundary is read before the material it describes. This is
an annotation on a successful call: it never turns a result into an error, and a response
that carries no boundary keys gains no extra block.

Which tools deliver which half follows the server and the payload shape the tool
asks for, not the MCP layer. `/capture`, `/snapshot` and `/find` publish
`untrustedContent` and `idpiNotice` on every response while content wrapping is
configured — a standing boundary, not a detection — so `pinchtab_capture` and
`pinchtab_find` lead with the prose notice block. `pinchtab_snapshot` leads with it only
when it asks for that envelope (`compact: false`); its default `compact` format is plain
text with no keys to read, and the boundary arrives in-band instead, as the `WARNING`
line and `<untrusted_web_content>` wrapper around the tree itself. `pinchtab_get_text` is
in-band for the same reason: `/text` wraps the boundary around its prose rather than
adding keys, so the tool's single block already contains the trust markers and no second
notice block is added; its payload carries `idpiWarning` only when the scanner matched
something.

For setup and client configuration, see [MCP Server](../mcp.md).

Saved browser state is intentionally not exposed as MCP tools right now. Use the CLI or HTTP API for `GET /state`, `pinchtab state`, and saved-state persistence operations.
