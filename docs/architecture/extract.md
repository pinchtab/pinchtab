# Extract Architecture

`internal/extract` fills a JSON schema — flat fields plus arrays of objects —
against a captured accessibility snapshot. It sits between `find` (one element) and `scrape` (a whole site): an
agent hands over a schema and gets typed data back, with per-field confidence so
it can fall back to `find` where a field is uncertain.

It is model-free and browserless. It reuses the same semantic matcher that backs
`find` (`github.com/pinchtab/semantic`, lexical + feature-hash embedding, no
network, no model download) over a node list it is handed. It never touches CDP.

## Pipeline

```text
JSON schema ──ParseSchema──► Schema (ordered properties, resolved hints)
node list   ──canonical order (by ref)──► descriptors (semdesc.Build)
                                    │
        per property: build query ──► matcher.Find ──► best ref + score
                                    │         (score desc, then document order)
                read value (Value|Text|Name, Checked for bool) ──► coerce to type
                                    │
                    data + per-field {ref, score, confidence, source, reason}

array property: choose container (scope | group detection) ──► items
                                    │
        per item: the same field resolution over the item's subtree only
                                    │
     data[prop] = [ {…}, … ]  fields[prop] = {ref: container, items: [{ref, fields}], truncated}
```

## Schema subset

The accepted schema is an `object` with `properties`. Each property is one of
`string`, `number`, `integer`, `boolean`, or `array`. Per property the extractor
honours:

- `description` — matched alongside the property name as query text.
- `required` — a required field that cannot be filled is listed in `missing`,
  never invented.
- `x-pinchtab-hint` — an explicit query or selector that overrides the
  name-based query (see below).

An `array` property must carry `items` of type `object` with its own flat
`properties`; per array the extractor honours `minItems`, `maxItems`, and
`x-pinchtab-scope` (a selector naming the container, same grammar as a hint).

`ParseSchema` returns a typed `*UnsupportedError` naming the offending path for
constructs outside the subset:

- a non-`object` root (`type: array is not supported`),
- a property typed `object` (`properties.price.type: object is not supported`),
- an unknown type (`properties.price.type: decimal is not supported`),
- nested `properties` on a property (`properties.price.properties: ...`),
- a `css:` or `xpath:` hint or scope, which needs a browser
  (`properties.price.x-pinchtab-hint: ...`),
- an array without object `items` (`properties.tags.items: ...`), an array
  nested inside `items` (`properties.rows.items.properties.tags.type: ...`), or
  a negative `minItems`/`maxItems`.

Property order is alphabetical, so `missing` and iteration are deterministic.

## Field resolution

For each property the extractor builds a query and takes the single best node
above the threshold (default `0.3`, same as `find`):

1. **Query.** A resolved `x-pinchtab-hint` wins. Otherwise the query is the
   property name joined with its description.
2. **Match.** The query runs through the shared combined matcher against every
   descriptor. The winner is picked here, not taken from the matcher's own
   `best_ref`: scores are rounded to six decimals and the highest wins, ties
   broken by document order, because the combined matcher merges its halves
   concurrently and its tie ordering is not stable. When the winner is label
   text (`StaticText`/`LabelText`) naming a value-bearing control — a
   textbox, searchbox, combobox, listbox, spinbutton or slider whose
   accessible name equals the label or starts with it — the field binds to
   the nearest such control instead, so a wrapping `<label>` yields the
   control's ref and value rather than its own text. A `ref:` hint is never
   redirected; text that labels no control resolves as matched.
3. **Read.** The matched node's value is read in priority order `Value`, `Text`,
   `Name`; booleans consult the accessibility `Checked` state first.
4. **Coerce.** The raw string is coerced to the schema type. A coercion failure
   leaves the field missing with a reason — never a wrong-typed value.

### Hints

`x-pinchtab-hint` accepts either a bare query or a selector, restricted to the
kinds a node list can answer without a browser:

- a bare string → used verbatim as the query (except a bare `eN`, which is a
  ref, and a bare `//…` XPath, which is rejected),
- `find:` → the natural-language query,
- `role:`, `label:`, `placeholder:`, `alt:`, `title:`, `testid:`, and
  `first:` / `last:` / `nth:` wrapping one of those → routed through
  `selector.SemanticQuery` so the grammar stays single-sourced with the
  action and `find` paths,
- `text:` → the text as a query,
- `ref:` (or a bare `eN`) → selects that node verbatim; a ref that is not in the
  node list leaves the field missing with reason `ref_not_found`,
- `css:` / `xpath:` → rejected by `ParseSchema` (they need a live DOM).

A hint that resolves beats the name-based query, so an agent can pin an ambiguous
field (e.g. a sale price among several prices) without renaming the schema.

### Request scope

`POST /extract` also takes an optional top-level `scope`, parsed with the same
grammar as a hint (`Schema.WithScope`). When set, it is resolved once over the
whole node list and the entire schema then resolves inside the matched node's
subtree only. A scope that matches nothing reports every field with reason
`scope_not_found` and lists the required ones in `missing`.

## Arrays of objects

An `array` property resolves to one object per repeated group in the snapshot.

### Tree derivation

`A11yNode` carries no child links; the tree is derived from the pre-order node
list and `Depth`:

- the **subtree** of a node is the contiguous run of following nodes whose
  `Depth` is greater than the node's,
- its **direct children** are the nodes in that run at exactly `Depth + 1`,
- its **ancestors** are found by walking backwards, taking each earlier node
  whose `Depth` is smaller than the last one taken.

Every step below (candidates, items, per-item scoping, header lookup) is built
on those three derivations and nothing else.

### Container

1. **Scope.** If the property carries `x-pinchtab-scope`, that selector is
   resolved over the whole node list exactly like a hint (`ref:` verbatim, the
   semantic kinds through the matcher) and detection runs inside the matched
   node's subtree only, the node itself admitted as a candidate with as few as
   one item, so a scope on a `table` yields its rows and a scope on a `list`
   its items. A scope that matches nothing leaves the property missing with
   reason `scope_not_found`; no detection runs outside it.
2. **Detection.** Otherwise every node is a candidate whose direct children
   share a dominant role (the most frequent role among them) at least **three**
   times, or at least **two** times when the node's own role is `list`,
   `table`, `rowgroup`, `grid`, or `feed`.
3. **Scoring.** Each candidate is scored by resolving the item schema inside
   its first three items (fewer if it has fewer) and taking the fraction of
   fields filled: `score = filled / (sampled items × fields)`. Candidates are
   ranked by score, then by item count, then by document order. A page with
   no candidate at all reports `no_repeated_group` with no ref. A best score of
   `0` — candidates exist but no item field resolves in any of them — reports
   `no_field_match` with the top-ranked candidate's container as `ref`. That is
   the closest candidate, which may be a nav menu rather than the list the
   agent meant; the remedy is an `x-pinchtab-hint` on the item fields, not a
   different scope. A scoped container skips the `0` check: the agent chose
   it.

The ranking is what disambiguates a page with several repeated groups: a nav
menu of links scores `0` against a product item schema whose fields want a
heading and a price, while the product region scores `1`.

### Items

The items are the container's direct children carrying the dominant role
(`listitem`, `article`, `row`, …). A `row` whose children include a
`columnheader` is a header, never an item.

Two per-item modes exist:

- **Subtree.** The flat resolver runs over the item's own subtree only (the
  item node included, since a pruned tree often leaves a menu's or result's
  text on the item node itself), so `price` in item 3 can never match item 1
  and a field absent from one item stays missing there even when every other
  item has it.
- **Columnar.** When the items are `row`s and the nearest enclosing `table` or
  `grid` (or the container itself) holds a header row, each field is resolved
  once against the header cells to a column index, and every row reads the cell
  at that index. Table cells are named by their value, not their column, so
  matching them per row would find nothing. A row shorter than the index leaves
  the field missing with `no_match`.

Items where every field is missing are dropped. The output is capped at the
smaller of the schema's `maxItems` and `Options.MaxItems` (default `100`); the
cap is tested after empties are dropped, so `truncated: true` is reported only
when a non-empty item was actually left out.
Fewer items than `minItems` leaves the property missing with reason
`too_few_items`.

### Result shape

`data[prop]` is the array of objects. `fields[prop]` carries the container
`ref`, the group `score` and its confidence band, `items` — one `{ref, fields}`
per returned item with the same per-field diagnostics as a flat field — and
`truncated`.

## Coercion rules

- **string** — the trimmed raw value.
- **number / integer** — only the **first numeric token** is parsed: a leading
  currency symbol and sign are stripped, the token's thousands separators are
  dropped, sign and decimal are kept, and parsing stops at the first character
  after the token so digits from a trailing word are never glued on. Both ASCII
  `-` and the Unicode minus `−` (U+2212) are honoured. `integer` truncates toward
  zero. Examples: `"$1,299.00"` → `1299`, `"−3.5 kg"` → `-3.5`,
  `"4.7 out of 5"` → `4.7` (not `4.75`), `"2 of 3"` → `2`, `"call for price"` →
  missing with reason `not_numeric`.
- **boolean** — the accessibility `Checked` state (`true`/`false`) first, then
  the words `yes`/`no`/`true`/`false` in the read value. A `mixed` checkbox or an
  unrelated string leaves the field missing with reason `not_boolean`.

## Confidence

Each field carries `ref`, `score`, and `confidence`. The confidence band comes
from `semantic.CalibrateConfidence` — `high` (score ≥ 0.8), `medium` (≥ 0.6),
`low` (below) — the same bands `find` reports, so the two cannot drift. An agent
can fall back to `find` on a `low` field.

## Determinism

`Resolve` is deterministic. Before matching, the node list is copied and stably
sorted into document order by ref (snapshot refs `e1`, `e2`, … are assigned in
pre-order, so numeric ref order is document order). The same schema over the same
nodes — in any input order — yields identical output; ties break on document
order. Array resolution inherits this: candidates and items are enumerated in
document order and ties rank by document order.

## Descriptors

Nodes are converted to matcher descriptors by `internal/semdesc`, the single
source of the node-to-descriptor mapping shared with the `find` handler. The
extractor has no browser, so it skips the DOM-metadata enrichment `find` applies
to live snapshots and matches the node list as handed over.
