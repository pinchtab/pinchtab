# Find

`/find` lets PinchTab locate elements by natural-language description instead of CSS selectors or XPath.

It works against the accessibility snapshot for a tab and returns the best matching `ref`, which you can pass to `/action`.

## Endpoints

PinchTab exposes two forms:

- `POST /find`
- `POST /tabs/{id}/find`

Use `POST /find` when you are talking directly to a bridge-style runtime or shorthand route and want to pass `tabId` in the request body.

Use `POST /tabs/{id}/find` when you already know the tab ID and want the orchestrator to route the request to the correct instance.

## Request Body

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `query` | string | yes | - | Natural-language description of the target element |
| `tabId` | string | no | active tab | Tab ID when using `POST /find` |
| `threshold` | float | no | `0.3` | Minimum similarity score |
| `topK` | int | no | `3` | Maximum number of matches to return |
| `lexicalWeight` | float | no | matcher default | Override lexical score weight |
| `embeddingWeight` | float | no | matcher default | Override embedding score weight |
| `explain` | bool | no | `false` | Include per-match score breakdown |

## Main Example

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/find \
  -H "Content-Type: application/json" \
  -d '{"query":"login button","threshold":0.3,"topK":3}'
# CLI Alternative
pinchtab find --tab <tabId> "login button"
```

There is a dedicated CLI `find` command:

```bash
pinchtab find "login button"
pinchtab find --threshold 0.5 --explain "primary submit button"
pinchtab find --ref-only "search input"
```

When nothing matches, `find` prints `No elements matched "<query>"` and exits 0 — an empty
result is an answer, the same way `console` and `errors` report having nothing to show.
`--ref-only` prints that line to stderr and exits non-zero instead, deliberately: it is
built for `REF=$(pinchtab find … --ref-only)`, where an empty `REF` spent on a click is
worse than a failed command.

`--ref-only` also exits non-zero when the top matches are tied (`confidence` is `ambiguous`),
naming the tied refs and the response's `retry_query`, or each tied ref's role and name when
they differ and there is no `retry_query`. Printing one of the tied refs would be a guess.

## Using `POST /find`

```bash
curl -X POST http://localhost:9867/find \
  -H "Content-Type: application/json" \
  -d '{"tabId":"<tabId>","query":"search input"}'
```

If `tabId` is omitted, PinchTab uses the active tab in the current bridge context.

## Response Fields

| Field | Description |
| --- | --- |
| `best_ref` | Highest-scoring element reference to use with `/action`; not safe to act on when `tied_refs` is present |
| `confidence` | `high`, `medium`, `low`, or `ambiguous` when the top matches are tied |
| `tied_refs` | Present only on a tie: the refs whose role and name equal the top match's, or whose score is within 0.01 of it |
| `retry_query` | Present only on a tie whose refs all share the top match's role and name: `nth:<i>:role:<role> <name>`, where `i` counts from 1 in page order. Omitted when the tied refs differ in role or name, since no single positional query reaches them all; pick the ref you mean from `matches` |
| `score` | Score of the best match |
| `matches` | Top matches above threshold |
| `strategy` | Matching strategy used |
| `threshold` | Threshold used for the request |
| `latency_ms` | Matching time in milliseconds |
| `element_count` | Number of elements evaluated |
| `idpiWarning` | Advisory warning when IDPI is in warn mode |

When `explain` is enabled, each match may also include lexical and embedding score details.

## Query Syntax

Beyond plain natural-language descriptions, the matcher understands three query modifiers:

### Ordinal Queries

Use ordinal words to pick a position from otherwise similar matches:

```bash
pinchtab find "first button"
pinchtab find "second search result"
pinchtab find "last link"
```

Ordinal matching is applied after semantic scoring. When the snapshot has no coordinates, document order is used as the stable order.

### Negative Queries

Use `not`, `without`, `exclude`, `excluding`, `except`, `no`, or `ignore` to push elements away from the match:

```bash
# Picks Cancel over Submit
pinchtab find "button not submit"

# Compose multiple exclusions
pinchtab find "input no password no username"

# Exclude a phrase
pinchtab find "button without sign in"
```

Tokens before the trigger are positive; everything after is negative until the next trigger or end of query.

### Visual / Location Queries

Directional and relative phrases bias the match toward elements at the matching position on the page:

```bash
# Directional (top / bottom / left / right / corner)
pinchtab find "bottom button"
pinchtab find "button in top right corner"
pinchtab find "sidebar on the left"

# Anchor-relative (above / below / under / over)
pinchtab find "link below the search box"
pinchtab find "button above the footer"
```

When the accessibility snapshot has no coordinates, document order is used as a fallback for vertical position — so `"bottom button"` selects the last matching button in the snapshot. When element bounding boxes are available they take precedence.

Visual hints are applied by the combined matcher (the default). They do not affect the lexical-only matcher.

## Confidence Levels

| Level | Score Range | Meaning |
| --- | --- | --- |
| `high` | `>= 0.80` | Usually safe to act on directly |
| `medium` | `0.60 - 0.79` | Reasonable match, but verify for critical actions |
| `low` | `< 0.60` | Weak match; rephrase the query or lower the threshold carefully |

## Common Flow

The standard pattern is:

```text
navigate -> find -> action
```

Example:

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/find \
  -H "Content-Type: application/json" \
  -d '{"query":"username input"}'
```

Then use the returned ref:

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/action \
  -H "Content-Type: application/json" \
  -d '{"ref":"e14","kind":"type","text":"user@pinchtab.com"}'
```

## Operational Notes

- `/find` uses the tab's accessibility snapshot, not raw DOM selectors.
- The query is natural language, not a selector: CSS, XPath and refs are not resolved, only scored as text. Prefix a query with `find:` or `semantic:` to force natural-language matching.
- Structured `/find` queries such as `role:button Save`, `text:Submit`, `label:Email`, `placeholder:Search`, `alt:Logo`, `title:Close`, `testid:submit`, `first:role:button`, `last:text:Submit`, and `nth:2:label:Email` are matched by the semantic engine against enriched descriptors. `/find` hands the query to the matcher unchanged, so its `nth:` index counts from 1 (`nth:1:label:Email` is the first match); `nth:0` or an index past the last match returns `200` with an empty `best_ref`. The zero-based index and the out-of-range refusal apply to `nth:` in action selectors, not to `/find`.
- In action commands, `role:`, `label:`, `placeholder:`, `alt:`, `title:`, `testid:`, and wrappers around those forms use semantic matching. CSS, XPath, refs, the existing `text:` action selector, and bare CSS/text wrappers such as `first:button` remain browser-side selector resolution.
- If there is no cached snapshot, PinchTab tries to refresh it automatically before matching.
- Every successful response carries the tab's ref vocabulary token in the `X-PinchTab-Vocab` header; the CLI and MCP capture it, so a returned ref can be acted on without a snapshot in between.
- Successful matches are useful inputs to `/action`, `/actions`, and higher-level recovery logic.
- A `200` response can still return an empty `best_ref` if nothing met the threshold.

## Error Cases

| Status | Condition |
| --- | --- |
| `400` | invalid JSON or missing `query` |
| `403` | blocked by IDPI in strict mode |
| `404` | tab not found |
| `409` | `dialog_blocked`: a JavaScript dialog is blocking the tab |
| `500` | Chrome not initialized, no elements in the snapshot, or matcher failure |
