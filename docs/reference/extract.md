# Extract

`/extract` reads typed values off the current page against a JSON schema: a price comes
back as a JSON number, a checkbox as a boolean, a product grid as an array of objects. It
works against the tab's accessibility snapshot, so every field also carries the `ref` it
was read from, usable with `/action` straight away.

## Endpoints

PinchTab exposes two forms:

- `POST /extract`
- `POST /tabs/{id}/extract`

Use `POST /extract` with `tabId` in the body (or none, for the active tab). Use
`POST /tabs/{id}/extract` when you already know the tab ID and want the orchestrator to
route the request to the instance that owns it.

## Request Body

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `schema` | object or string | yes | - | JSON schema describing the data; see [Schema Subset](#schema-subset) |
| `tabId` | string | no | active tab | Tab ID when using `POST /extract` |
| `scope` | string | no | whole page | Confine every field to the subtree of one element: a bare ref (`e12`), `role:`, `text:` or a plain query. CSS and XPath are refused (`400`) |
| `threshold` | float | no | `0.3` | Minimum match score per field |
| `maxItems` | int | no | `100` | Cap on items per array |

## Main Example

Save the schema as `product.schema.json`:

```json
{
  "type": "object",
  "required": ["name", "price", "inStock"],
  "properties": {
    "name": {"type": "string", "description": "product name", "x-pinchtab-hint": "role:heading"},
    "price": {"type": "number", "description": "product price"},
    "rating": {"type": "number", "description": "product rating out of 5"},
    "inStock": {"type": "boolean", "description": "in stock availability", "x-pinchtab-hint": "role:checkbox"}
  }
}
```

```bash
curl -X POST http://localhost:9867/extract \
  -H "Content-Type: application/json" \
  -d "{\"schema\":$(cat product.schema.json)}"
# CLI Alternative
pinchtab extract --schema product.schema.json
```

On a product page the CLI prints the data alone:

```json
{
  "inStock": true,
  "name": "Sony WH-1000XM5 Wireless Headphones",
  "price": 1299,
  "rating": 4.7
}
```

## CLI

```bash
pinchtab extract --schema product.schema.json
cat product.schema.json | pinchtab extract --schema -
pinchtab extract --schema product.schema.json --fields
pinchtab extract --schema products.schema.json --max-items 2
pinchtab extract --schema amounts.schema.json --scope role:table
```

| Flag | Description |
| --- | --- |
| `--schema <file\|->` | Schema file, or `-` to read it from stdin (required) |
| `--scope <selector>` | Sent as `scope` |
| `--max-items <n>` | Sent as `maxItems` |
| `--fields` | Print a `field<TAB>ref<TAB>confidence` table on stderr, one row per field after a header line; array items appear as `products[0]` and `products[0].name`, with `-` in the cells they lack |
| `--explain` | The same table on stderr with `score`, `source` and `reason` columns |
| `--json` | Print the full response envelope instead of `data` |
| `--tab <id>` | Target a tab through `POST /tabs/{id}/extract` |

The field table and three diagnostics are reported on stderr, never on stdout, so piping
`data` into `jq` is unaffected, with or without `--fields`. The command still exits 0,
because the page answered:

| Stderr line | When |
| --- | --- |
| `field ref confidence` (tab-separated, plus `score source reason` with `--explain`) | Header of the `--fields` / `--explain` table; every row below it has the same columns |
| `unmatched: items (no_field_match); rerun with --explain` | One line per top-level property absent from `data`, carrying that field's `reason`; not printed with `--explain`, whose table already shows it |
| `missing required fields: name, price` | A `required` property did not resolve |
| `truncated: an array hit its item cap; ...` | An array stopped at `--max-items` or the schema's `maxItems` |

A schema the server refuses exits non-zero with the server's `400` message, which names
the offending path.

The CLI keeps the vocabulary the extract minted, so a field's ref can be clicked next
without a snapshot in between:

```bash
pinchtab extract --schema product.schema.json --fields
pinchtab click e7
```

## Response Fields

| Field | Description |
| --- | --- |
| `data` | Values coerced to the schema types; arrays hold one object per repeated group |
| `fields` | Per property: `ref`, `score`, `confidence` (`high`, `medium`, `low`), `source` (`value`, `text`, `name`, `checked`, `hint`) and `reason` when it did not resolve; arrays add `items` (each with its own `ref` and `fields`) and `truncated` |
| `missing` | Required properties that did not resolve |
| `truncated` | `true` when any array hit its item cap |
| `vocabularyToken` | The ref vocabulary the returned refs belong to (also in `X-PinchTab-Vocab`) |
| `latency_ms` | Resolution time in milliseconds |
| `element_count` | Number of snapshot nodes considered |
| `idpiWarning` | Advisory warning when IDPI is in warn mode |

## Schema Subset

- The root is an object with `properties`; `required` lists the fields reported in `missing`.
- A property is `string`, `number`, `integer` or `boolean`, or an `array` whose `items` is
  an object of such properties. Nested objects and arrays inside array items are refused.
- The matcher searches for the property name plus its `description`, so a descriptive
  `description` ("product price") matters more than the key.
- `maxItems` and `minItems` bound an array; an array below `minItems` does not resolve.
- An unsupported construct is a `400` naming the path, for example
  `properties.price.type: object is not supported`.

## Hints And Scope

- `x-pinchtab-hint` on a property pins where it is read from: a ref (`e12`), or a
  `role:`, `text:` or other semantic selector. Add one when a field comes back `low`.
- `x-pinchtab-scope` on an array pins its container, for example `role:table`, when the
  page has more than one repeated group.
- An array the page has no repeated group for is missing with reason `no_repeated_group`
  and no ref. When groups exist but no item field matches inside any of them, the reason
  is `no_field_match` and `ref` is the closest candidate container, which may be a nav
  menu rather than the list you meant; add an `x-pinchtab-hint` to the item fields.
- The request-level `scope` confines the whole schema to one subtree; a scope that
  matches nothing leaves every field unresolved with reason `scope_not_found`.

## When To Use It

- `extract` for typed values you would otherwise parse out of a snapshot: prices, flags,
  table rows, search results.
- `find` for one element to act on.
- `text --markdown` for prose to read.

## Error Cases

| Status | Condition |
| --- | --- |
| `400` | invalid JSON, missing `schema`, or an unsupported schema or `scope` (the message names the path) |
| `403` | blocked by IDPI in strict mode |
| `404` | tab not found |
| `409` | `dialog_blocked`: a JavaScript dialog is blocking the tab (answer it with `pinchtab dialog`) |
| `500` | Chrome not initialized, snapshot unavailable, or the snapshot has no elements |
