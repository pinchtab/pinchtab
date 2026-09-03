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

`--wait-nav` is not permission to navigate — it makes the click *wait* for the
navigation to settle before returning, which is what you want when the next action
depends on the new page having loaded.

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
- `mode` and `humanize:true` are mutually exclusive.
- To opt a click into the slower humanized path for a page that needs it, pass `humanize:true` in the action JSON or set `instanceDefaults.humanize:true`.
- `submit:true` is for terminal form actions where retrying could submit twice. For clicks it sends exactly one DOM click, disables recovery/retry delivery, and reports a bounded `postState` result (`succeeded` when the URL changes or an open modal closes; otherwise `pending`). It requires an element target and cannot be combined with coordinates, `waitNav`, `mode`, or `humanize:true`. It is accepted only on a single `/action` request, not a batch or macro.

## Related Pages

- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
- [Navigate](./navigate.md)
