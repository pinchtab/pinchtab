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
| `best_ref` | Highest-scoring element reference to use with `/action` |
| `confidence` | `high`, `medium`, or `low` |
| `score` | Score of the best match |
| `matches` | Top matches above threshold |
| `strategy` | Matching strategy used |
| `threshold` | Threshold used for the request |
| `latency_ms` | Matching time in milliseconds |
| `element_count` | Number of elements evaluated |
| `idpiWarning` | Advisory warning when IDPI is in warn mode |

When `explain` is enabled, each match may also include lexical and embedding score details.

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
  -d '{"ref":"e14","kind":"type","text":"user@example.com"}'
```

## Operational Notes

- `/find` uses the tab's accessibility snapshot, not raw DOM selectors.
- If there is no cached snapshot, PinchTab tries to refresh it automatically before matching.
- Successful matches are useful inputs to `/action`, `/actions`, and higher-level recovery logic.
- A `200` response can still return an empty `best_ref` if nothing met the threshold.

## Error Cases

| Status | Condition |
| --- | --- |
| `400` | invalid JSON or missing `query` |
| `403` | blocked by IDPI in strict mode |
| `404` | tab not found |
| `500` | Chrome not initialized, snapshot unavailable, or matcher failure |
