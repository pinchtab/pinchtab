# Snapshot

Get an accessibility snapshot of the current page, including element refs that can be reused by action commands.

Iframe content is detected automatically during snapshot capture. Same-origin iframe descendants are included beneath the iframe owner element, and their refs can be reused directly with action commands. Cross-origin iframes currently remain as owner nodes only.

Selector scoping is explicit. `selector=...` only searches the current frame scope, which defaults to `main`. To scope selector-based snapshots into an iframe, set the frame first with [`/frame`](./frame.md) or `pinchtab frame`.

```bash
curl "http://localhost:9867/snapshot?filter=interactive"
# CLI Alternative (defaults to compact text output)
pinchtab snap -i
# Output: a "# <title> | <url> | <N> nodes" header line, then one node per line
e5:link "More information..."

# --json (same as --compact=false) keeps the interactive filter; --full returns every node as JSON
pinchtab snap --full
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `-i`, `--interactive` | Filter to interactive elements + headings (default: true) |
| `-c`, `--compact` | Compact text output (default: true) |
| `--json` | JSON output, keeping the interactive filter (same as `--compact=false`) |
| `-d`, `--diff` | Show diff from previous snapshot |
| `--full` | Full JSON output (shorthand for `--interactive=false --json`) |
| `--text` | Text output format |
| `-s`, `--selector` | Selector to scope the snapshot (also accepted as the positional `[selector]` argument) |
| `--max-tokens` | Maximum token budget |
| `--depth` | Tree depth limit |
| `--tab` | Target specific tab |

## Examples

```bash
pinchtab snap                           # Interactive compact (default)
pinchtab snap -i -c                     # Same as above
pinchtab snap --full                    # Full JSON with all nodes
pinchtab snap -d                        # Show changes since last snapshot
pinchtab snap --selector "#main"        # Scope to element
pinchtab snap --max-tokens 2000         # Limit output size
```

## API Parameters

| Parameter | Description |
|-----------|-------------|
| `tabId` | Target tab (defaults to the current tab) |
| `filter` | `interactive` for interactive + headings, `all` (default) for the whole tree |
| `interactive` | Boolean alias for `filter`: `true` is `filter=interactive`, `false` is `filter=all`. Contradicting an explicit `filter` is a 400 |
| `format` | `compact`, `text`, `yaml`, or default JSON. The CLI and the MCP tool both ask for `compact`; the HTTP default is unchanged |
| `diff` | `true` for diff mode |
| `selector` | Unified selector (ref, CSS, XPath, text, …) to scope |
| `maxTokens` | Token budget limit (positive integer; anything else is a 400) |
| `depth` | Tree depth limit (`-1` = no limit; below `-1` is a 400) |
| `noAnimations` | `true` disables animations once before capturing |
| `output` | `file` writes the snapshot under the state dir's `snapshots/` and returns `{path, size, format, timestamp}` |
| `path` | With `output=file`, a file path inside the state dir (anything outside is a 400) |

`GET /tabs/{id}/snapshot` is the same handler with the tab in the path.

Unknown query parameters are not rejected; they are echoed back as `ignoredParams` (JSON/YAML) or a `# ignored params:` comment line (compact/text) so a mistyped flag is visible.

## Response

The default JSON body carries `url`, `title`, `route`, `nodes`, `count` and `vocabularyToken`,
plus `truncated`/`maxTokens` when the budget cut the tree and `hint` when a selector matched an
element with no accessible nodes. `vocabularyToken` names the ref vocabulary this snapshot
issued; every snapshot also sets it on the `X-PinchTab-Vocab` response header (with
`X-PinchTab-Tab-Id`), whatever the format. Echo it as `vocab` on ref-based actions (the CLI does this for you):
an action that targets a ref under a token other than the tab's current one is refused with
`409 vocab_superseded` — re-snapshot and use the new refs.

When a modal dialog is open in the page, the snapshot is scoped to the topmost dialog's
subtree. If the topmost dialog changes twice while the snapshot is being taken, the request
fails with `409` — retry after the page settles. A pending JavaScript dialog (alert,
confirm, prompt) blocks the read with `409 dialog_blocked` until it is answered with
`pinchtab dialog`.

A tab whose document is `hidden` (a background tab) is rendered before the accessibility read:
PinchTab enables focus emulation and waits for a painted frame (up to 1s), so content that
only lays out when visible still appears in the tree.

## What a format costs, and what it carries

The same 40 realistic interactive nodes, rendered through each output path:

```
  compact     1644 bytes  ~ 411 tokens   1.0x
  text        2060 bytes  ~ 515 tokens   1.3x
  json        8211 bytes  ~2052 tokens   5.0x
  yaml       17467 bytes  ~4366 tokens  10.6x
```

55% of that JSON body is the DOM-derived descriptor fields — `tag`, `label`, `placeholder`,
`alt`, `title`, `testid`, `text` — which exist to feed the server-side semantic matcher.
`compact` and `text` render `ref`, `role`, `name`, `value` and the state flags and nothing
else, so those fields cannot appear there at all. The DOM pass that fills them is therefore
skipped for `compact` and `text`, which also drops one CDP round trip per node from the
cheapest path.

Skipping it changes nothing the caller can see: the fields are absent from those formats
either way, and `find` and the semantic selectors enrich the cached nodes themselves before
matching, so a `compact` snapshot leaves them exactly as capable as a JSON one.

## What `maxTokens` guarantees

`maxTokens` is a ceiling, not a hint. The response never exceeds what you asked for, and
nothing is left on the table: every node left out was too big for what remained. Measured
across `compact`, `text`, `json` and `yaml` on a page of realistic interactive nodes, a
budget that actually constrains the result delivers 87–100% of it.

When the budget cannot fit the whole tree, it is spent on what the page can be **acted
on** with before what it can be read with: interactive roles first, then headings, media
and table cells, then everything else. Nodes are returned in document order, so the reply
still reads like the page.

This matters because document order is not value order. A page whose controls sit below
its copy — a form under terms, a search box under a nav blurb, pagination under results —
used to return a budget's worth of prose and no refs at all, which is a snapshot nothing
can be done with. On a 71-node page with 9 controls at the end, budgets of 120, 200 and
300 tokens each returned 0 of the 9; they now return all 9 in the same space.

Refs are assigned after the budget is applied and cached for the tab, so every ref in a
truncated snapshot resolves to the element it names — a truncated tree is fully
actionable.

The cost is measured, not modelled: each node is charged the bytes its own format emits —
rendered for `compact` and `text`, marshalled for `json` and `yaml` — so a change to a
formatter changes the budget with it. Tokens are estimated at four bytes each.

Formats are not interchangeable for a given budget. `yaml` is roughly three times the size
of `json` for the same nodes, because the node struct carries JSON field tags and no YAML
ones, so YAML emits every field including the empty ones. The same `maxTokens` therefore
returns far fewer nodes in `yaml` than in `json` — which is the budget working, not a
regression. Prefer `compact` when the budget is tight: it fits several times more nodes
into the same tokens than either structured format.

## Control state on a node

A snapshot reports the state of a control, not just its identity, so an agent can
verify its own action and read a page it did not set up:

- `value` — the current text of an input or the selection of a `select`
- `focused`, `disabled`, `hidden` — booleans, present only when true
- `checked` — `"true"`, `"false"` or `"mixed"` for a checkbox, radio,
  `menuitemcheckbox`, `menuitemradio`, or any element carrying `aria-checked`

`checked` is a three-value string rather than a boolean because `"mixed"` is a real
state: both a native indeterminate checkbox and `aria-checked="mixed"` report it.

**An absent `checked` means the node has no checkedness — never that it is off.**
Ordinary nodes do not carry the key at all, so a missing value must not be read as
unchecked. A control that is off says so explicitly with `"false"`.

The rendered formats carry all three states, so an unchecked option never looks
like a node the field does not apply to:

| State | `format=text` | `format=compact` |
|-------|---------------|------------------|
| checked | `[checked]` | `[x]` |
| unchecked | `[unchecked]` | `[ ]` |
| mixed | `[mixed]` | `[/]` |

A radio group is therefore readable from a single snapshot:

```
e4 radio "Standard shipping" [checked]
e5 radio "Express shipping" [unchecked]
e6 radio "Pickup" [unchecked]
```

Diff mode treats a change of `checked` as a change, so `pinchtab snap -d` after a
`check` shows the node as `[~]`.

## Related Pages

- [Click](./click.md)
- [Frame](./frame.md)
- [Tabs](./tabs.md)
