# Click

Click an element using a snapshot ref, CSS selector, XPath selector, text selector, or semantic selector.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
# CLI Alternative
pinchtab click e5
# Response (use --json for full JSON)
OK
```

## When the click navigates

A click that moves the page **succeeds**. There is nothing to declare in advance and
nothing to opt into: the result reports where the tab landed and that your refs are
gone.

```bash
pinchtab nav https://example.com --snap
# e1:link "Learn more"
pinchtab click e1
# OK navigated https://www.iana.org/help/example-domains
# HINT: every ref from your last snapshot is dead — run `pinchtab snap -i` before the next action
echo $?   # 0
```

With `--json`, the same click carries the outcome in `result`:

```json
{
  "success": true,
  "result": {
    "clicked": true,
    "navigated": true,
    "url": "https://www.iana.org/help/example-domains",
    "previousUrl": "https://example.com/",
    "refsStale": true
  }
}
```

`navigated` is keyed on what actually happened — the URL before the action compared
against the URL after — not on the element's role or tag, so it is the same answer for
a link, a router `<button>`, and a form control that redirects. A fragment-only change
(`#section`) is not a navigation: the document is the same and your refs still resolve.

**`refsStale: true` means every ref from your previous `/snapshot` is dead.** Refs are
minted per snapshot, so take a new one before the next ref-targeted action:

```bash
pinchtab click e1 --snap    # click, then print the new snapshot in one call
```

These fields are present whichever form you use: a plain click, a click with
`--wait-nav`, and a `--submit` click that redirects all report where they landed and
that your refs are dead. A `--submit` click keeps its own `postState` as the headline
and carries the landing alongside it.

`--wait-nav` is not permission to navigate — it makes the click *wait* for the
navigation to settle before returning, which is what you want when the next action
depends on the new page having loaded.

When a `--wait-nav` click ends without a navigation, it says so: the CLI prints
`OK no navigation` and the JSON result carries `navigated: false`. When the clicked
control's form was refused by browser validation (an empty `required` field, say), the
result carries the same `postState` a `--submit` click reports — `status: "blocked"`,
`signal: "validation_blocked"`, and the `invalidFields` — and the CLI prints
`BLOCKED validation_blocked` followed by one `name (Label): message` line per field.

A plain ref click on a form's submit button gets the same check: if the browser
refuses the form for validation, the result carries that `validation_blocked`
`postState` and the CLI prints the same `BLOCKED` lines instead of `OK`. When the form
goes through, the click waits up to 500ms for the page to confirm it, returning as soon
as the URL changes, an open modal closes, or a live region shows new text; that
confirmation arrives as a `succeeded` `postState` and the CLI prints `SUCCEEDED`. A
submit that changes nothing within the 500ms prints plain `OK`, and so does a click on
anything that is not a submit button, which pays no wait at all. Use `--submit` when
the confirmation may take longer.

A `blocked` result exits 1 after printing the `BLOCKED` lines, in plain, `--wait-nav`,
`--submit` and `--json` modes alike, so `pinchtab click e11 && pinchtab text` stops at
the refused form. `SUCCEEDED`, `PENDING` and `OK no navigation` exit 0.

## CLI Flags

| Flag | Description |
|------|-------------|
| `--css` | CSS selector instead of ref |
| `--wait-nav` | Wait for navigation after click |
| `--snap` | Output interactive snapshot after click |
| `--snap-diff` | Output snapshot diff after click |
| `--text` | Output page text after click |
| `--dialog-action` | Auto-handle JS dialog: `accept` or `dismiss` |
| `--dialog-text` | Prompt response text (with `--dialog-action accept`) |
| `--dismiss-banners` | Dismiss cookie/consent banners after a `--wait-nav` click (no-op without `--wait-nav`) |
| `--dismiss-known-interstitials` | Dismiss a recognized portal interstitial before resolving the click target (refused with `known_interstitial_not_dismissed` when it cannot) |
| `--x`, `--y` | Click at specific coordinates |
| `--humanize` | Use humanized bezier+jitter input path (overrides instance config) |
| `--submit` | Use the once-only submit-click path and include bounded post-submit state in the response |
| `--mode dom\|dispatch` | Broad low-level escape hatch for click delivery. Omit `--mode` for the normal click path, use `dom` for `element.click()`, or `dispatch` for synthetic click events on the target |
| `--json` | Full JSON response |
| `--tab` | Target specific tab |

## Examples

```bash
pinchtab click e5                       # Click by ref
pinchtab click "#login"                 # Click by CSS
pinchtab click "text:Submit"            # Click by text
pinchtab click e5 --snap                # Click and show new snapshot
pinchtab click e5 --wait-nav            # Click and wait for navigation
pinchtab click e5 --dialog-action accept  # Auto-accept alert/confirm
pinchtab click "#sign-in" --submit        # Submit once and report the observed outcome
pinchtab click e5 --mode dom             # Activate target directly despite occlusion
pinchtab click e5 --mode dispatch        # Dispatch click events on target despite occlusion
pinchtab click --x 100 --y 200           # Click at coordinates
```

## Notes

- Element refs come from `/snapshot`, and a navigation invalidates all of them — see [When the click navigates](#when-the-click-navigates)
- Refs for iframe descendants can be clicked directly without frame switch
- Selector lookup is limited to current frame scope (default: `main`)
- Use [`/frame`](./frame.md) before selector-based iframe actions
- Missing selectors fail immediately; use `pinchtab wait` first for dynamic UI (see [`commands.md`](../commands.md))
- The API also accepts `selector` field: `{"kind":"click","selector":"#login"}`
- Click behavior works like this: omit `mode` for the normal click path, use `mode:"dom"` for `element.click()`, or `mode:"dispatch"` for synthetic click events.
- Treat `mode` as a broad, low-level escape hatch for click delivery. Occlusion bypass is the common case, but it can also help with pages that need a non-default click path.
- `mode` and humanize are mutually exclusive — whether humanize comes from `humanize:true` on the request or from `instanceDefaults.humanize:true`.
- To opt a click into the slower humanized path for a page that needs it, pass `humanize:true` in the action JSON or set `instanceDefaults.humanize:true`.
- `submit:true` is for terminal form actions where retrying could submit twice. For clicks it sends exactly one DOM click, disables recovery/retry delivery, and reports a bounded `postState` result (`succeeded` when the URL changes, an open modal closes, or a `role=status`/`role=alert`/`aria-live` region shows text it did not show before the click — signal `live_region_changed`, with that text, capped, in `postState.liveRegionText`; otherwise `pending`). It requires an element target and cannot be combined with coordinates, `waitNav`, `mode`, or `humanize:true`. It is accepted only on a single `/action` request, not a batch or macro.
- Ref vocabulary: an action that targets a ref may echo the snapshot's `X-PinchTab-Vocab` token as `vocab` in the body (or the `X-PinchTab-Vocab` request header), with `vocabTab` naming its tab. If a newer snapshot has renumbered that tab's refs, `/action` refuses with a `vocab_superseded` conflict instead of acting on the wrong node — re-snapshot and use the new refs. The CLI attaches the token from your last snapshot automatically. When an action re-epochs the tab's refs, the response carries the new `X-PinchTab-Vocab` header.
- While a JavaScript dialog is open on the tab, `/action` refuses with a `dialog_blocked` conflict (details carry `dialogType` and `dialogMessage`); answer it with `pinchtab dialog accept|dismiss`, or pass `--dialog-action` on the click that opens it. See [Dialog](./dialog.md).
- A click that itself opens a JavaScript dialog succeeds, because the click landed: the result carries `dialog` (`dialogType`, `dialogMessage`, `hint`, `remedy`, `tabId`) and the CLI prints `OK opened <type> dialog: "<message>"` and skips `--snap`/`--text`. Answer the dialog instead of clicking again.
- A ref that cannot be resolved answers `404 ref_not_found` with `details.dispatched: false`.

## Related Pages

- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
- [Navigate](./navigate.md)
