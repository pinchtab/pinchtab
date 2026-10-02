# Navigate

Navigate the current tracked tab to a URL, or create a tab when no current tab is available.

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# CLI Alternative
pinchtab nav https://pinchtab.com
# Response (terminal: tab ID, then the landed URL; piped: tab ID only; --json for full JSON)
8f9c7d4e1234567890abcdef12345678
https://pinchtab.com/
```

API response: `{"tabId":"...","url":"<landed URL>","title":"...","status":200,"route":...}`.

`status` is the HTTP status of the main document the navigation landed on, after redirects. It is omitted when the document committed without a network response (`file:`, `data:`, `about:`, a same-document hash change) and when the page was served from a static tab.

## CLI Flags

`pinchtab nav <url>` auto-starts the default local server when it is not already running. When `--server` or `PINCHTAB_SERVER` targets another server, PinchTab connects to that server without auto-starting a new process.
The default config starts a headless browser, so a successful `pinchtab nav`
may not open a visible window. Use `--snap` to inspect the result, or run PinchTab
in headed mode when you want a visible browser.
Hidden aliases: `goto`, `navigate`, `open`.

| Flag | Description |
|------|-------------|
| `--tab` | Reuse existing tab |
| `--new-tab` | Force new tab |
| `--block-images` | Block image loading |
| `--block-ads` | Block ads |
| `--dismiss-banners` | After landing, click a visible cookie/consent dismissal button or remove obvious overlays |
| `--timeout` | Navigation timeout in seconds (max 120); overrides the 30s new-tab ceiling |
| `--snap` | Output snapshot after navigation |
| `--snap-diff` | Output snapshot diff after navigation |
| `--text` | Output page text after navigation |
| `--print-tab-id` | Print only tab ID (auto when piped); with `--snap`/`--text` the tab ID goes to stderr |
| `--json` | Full JSON response |

## Examples

```bash
pinchtab nav https://example.com              # Navigate current tab, or create one
pinchtab nav https://example.com --snap       # Navigate and snapshot
TAB=$(pinchtab nav https://example.com)       # Capture tab ID for reuse
pinchtab nav https://other.com --tab "$TAB"   # Reuse tab
pinchtab nav https://example.com --new-tab    # Force another tab
pinchtab nav https://example.com --block-images  # Skip images
```

## API Body Fields

| Field | Description |
|-------|-------------|
| `url` | Target URL (required) |
| `tabId` | Reuse existing tab |
| `newTab` | Force new tab |
| `blockImages` | Block image loading |
| `blockMedia` | Block media loading |
| `blockAds` | Block ads |
| `dismissBanners` | Dismiss cookie/consent banners after landing |
| `timeout` | Navigation timeout in seconds (capped at 120) |
| `waitTitle` | Wait up to N seconds for a title (capped at 30) |
| `waitFor` | Wait condition: `none` (default), `dom`, `selector`, `networkidle` |
| `waitSelector` | Selector to wait for; required when `waitFor` is `selector` |
| `dispatchOnly` | Return `{tabId,url,dispatched:true}` once the navigation is dispatched, without waiting for load |
| `browser` | Browser to route the request to |

`GET /navigate?url=...` accepts the same fields as query parameters, except `blockImages`, `blockMedia` and `blockAds`.

## Behavior

- Top-level `POST /navigate` opens a new tab when no `tabId` is provided, unless the caller is identified (session or agent id) and has a current tab, which is reused. Under the strict empty-pointer policy an identified caller with no current tab gets `409 no_current_tab`.
- `pinchtab nav <url>` uses the current tracked tab when one is available; otherwise it opens a new tab.
- `POST /tabs/{id}/navigate`, `POST /navigate` with `tabId`, and `pinchtab nav <url> --tab <id>` reuse the specified tab and make it the current tab for later unscoped operations.
- `--new-tab` and `newTab:true` force a new tab even if another tab is current.
- Commands that operate without `--tab` use the current tracked tab. Focusing or using a tab updates that current-tab pointer; if the pointer is stale, PinchTab falls back to the most recently used tracked tab.
- When the saved current-tab pointer names a tab the server no longer has, `pinchtab nav` retries the navigation without the tab id. For a session or agent-id caller — by `--agent-id` or `PINCHTAB_AGENT_ID`, either one — that retry reuses that scope's current tab, and the CLI stays silent because nothing was created. For a caller with neither, the server opens a **new** tab — the documented anonymous contract — so the CLI prints a `HINT` on stderr naming both the tab that was gone and the new one, rather than reporting plain success while leaving you with two tabs on one URL. Run with `PINCHTAB_SESSION` set to keep a single work surface. An explicit `--tab` never retries: it surfaces the 404.

- A page that answers `404`, `401`, `500` or any other HTTP error is still a completed navigation: the API answers `200` with that code in `status`, and `pinchtab nav` exits 0 and prints `HTTP <status>` on stderr when it is 400 or above, so `--snap` and `--text` still print the error page. With `--json`, read `status` from the body.

Errors: `400` for an invalid URL, `400 bad_wait_for` when the `waitFor` condition is unsupported or does not hold, `403` when the target is blocked (IDPI domain policy or a private/internal address), `409 dialog_blocked` when a JavaScript dialog is open on the tab, `409 tab_paused_handoff` while the tab is paused for [handoff](./handoff.md), `422` on a redirect loop.

Rationale: the CLI keeps one obvious work surface by default. Use `--new-tab` when you intentionally want another tab, or `--tab`/`tabId` when you need a specific tab.

## Related Pages

- [Snapshot](./snapshot.md)
- [Tabs](./tabs.md)
