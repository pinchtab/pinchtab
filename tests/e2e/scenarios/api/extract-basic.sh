#!/bin/bash
# extract-basic.sh — POST /extract returns schema-typed data with live refs.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

PRODUCT_SCHEMA='{"type":"object","required":["name","price","inStock"],"properties":{"name":{"type":"string","description":"product name","x-pinchtab-hint":"role:heading"},"price":{"type":"number","description":"product price"},"rating":{"type":"number","description":"product rating out of 5"},"inStock":{"type":"boolean","description":"in stock availability","x-pinchtab-hint":"role:checkbox"}}}'
PRODUCTS_SCHEMA='{"type":"object","properties":{"products":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string","x-pinchtab-hint":"role:heading"},"price":{"type":"number","description":"product price"}}}}}}'
ROWS_SCHEMA='{"type":"object","properties":{"orders":{"type":"array","x-pinchtab-scope":"role:table","items":{"type":"object","properties":{"order":{"type":"integer","description":"order number"},"customer":{"type":"string","description":"customer name"}}}}}}'
CAPPED_SCHEMA='{"type":"object","properties":{"products":{"type":"array","maxItems":2,"items":{"type":"object","properties":{"name":{"type":"string","x-pinchtab-hint":"role:heading"}}}}}}'

# ─────────────────────────────────────────────────────────────────
start_test "extract: product schema returns typed data and live refs"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/extract-product.html\"}"
assert_ok "navigate to extract-product.html"
TAB_ID=$(get_tab_id)

pt_post /extract -d "{\"schema\":${PRODUCT_SCHEMA}}"
assert_ok "extract product"
assert_json_eq "$RESULT" '.data.price' '1299' "price is 1299"
assert_json_eq "$RESULT" '.data.price | type' 'number' "price is a JSON number"
assert_json_eq "$RESULT" '.data.inStock' 'true' "inStock is true"
assert_json_eq "$RESULT" '.data.inStock | type' 'boolean' "inStock is a JSON boolean"
assert_json_eq "$RESULT" '.data.name' 'Sony WH-1000XM5 Wireless Headphones' "name matches the fixture text"
assert_json_eq "$RESULT" '[.fields[].ref | select(. != null) | test("^e[0-9]+$")] | all' 'true' "every field ref is a snapshot ref"
NAME_REF=$(echo "$RESULT" | jq -r '.fields.name.ref')

pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${NAME_REF}\"}"
assert_ok "click on the name ref proves the ref is live"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "extract: vocabulary token and tab id match between headers and body"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/extract-product.html\"}"
assert_ok "navigate to extract-product.html"
TAB_ID=$(get_tab_id)

EXTRACT_HEADERS=$(mktemp)
RESULT=$(e2e_curl -s -D "$EXTRACT_HEADERS" -X POST "${E2E_SERVER}/extract" -H "Content-Type: application/json" -d "{\"schema\":${PRODUCT_SCHEMA}}")
HEADER_VOCAB=$(grep -i '^X-PinchTab-Vocab:' "$EXTRACT_HEADERS" | cut -d' ' -f2 | tr -d '\r')
HEADER_TAB=$(grep -i '^X-PinchTab-Tab-Id:' "$EXTRACT_HEADERS" | cut -d' ' -f2 | tr -d '\r')
rm -f "$EXTRACT_HEADERS"
BODY_VOCAB=$(echo "$RESULT" | jq -r '.vocabularyToken')

if [ -n "$BODY_VOCAB" ] && [ "$BODY_VOCAB" != "null" ] && [ "$BODY_VOCAB" = "$HEADER_VOCAB" ]; then
  pass_assert "body vocabularyToken $BODY_VOCAB matches X-PinchTab-Vocab"
else
  fail_assert "body vocabularyToken '$BODY_VOCAB' vs X-PinchTab-Vocab '$HEADER_VOCAB'"
fi
if [ "$HEADER_TAB" = "$TAB_ID" ]; then
  pass_assert "X-PinchTab-Tab-Id names the navigated tab"
else
  fail_assert "X-PinchTab-Tab-Id '$HEADER_TAB', want '$TAB_ID'"
fi

NAME_REF=$(echo "$RESULT" | jq -r '.fields.name.ref')
pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${NAME_REF}\",\"vocab\":\"${BODY_VOCAB}\"}"
assert_ok "an action echoing the extract's own token is accepted"

pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${NAME_REF}\",\"vocab\":\"stale-pre-extract-token\"}"
assert_http_status 409 "a pre-extract token is refused vocab_superseded"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "extract: array schema, scoped table, and maxItems on extract-list.html"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/extract-list.html\"}"
assert_ok "navigate to extract-list.html"

pt_post /extract -d "{\"schema\":${PRODUCTS_SCHEMA}}"
assert_ok "extract product grid"
assert_json_eq "$RESULT" '.data.products | length' '6' "6 products"
assert_json_eq "$RESULT" '[.data.products[].price | type == "number"] | all' 'true' "every price is numeric"
assert_json_eq "$RESULT" '.truncated' 'false' "not truncated"

pt_post /extract -d "{\"schema\":${ROWS_SCHEMA}}"
assert_ok "extract scoped table"
assert_json_eq "$RESULT" '.data.orders | length' '5' "5 rows under the table scope"
assert_json_eq "$RESULT" '.data.orders[0].customer' 'Alice Johnson' "first row customer"

pt_post /extract -d "{\"schema\":${CAPPED_SCHEMA}}"
assert_ok "extract with maxItems 2"
assert_json_eq "$RESULT" '.data.products | length' '2' "capped to 2"
assert_json_eq "$RESULT" '.truncated' 'true' "reports truncated"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "extract: error cases and the tab-scoped route"

pt_post /extract -d '{}'
assert_http_error 400 "schema" "missing schema is a 400"

pt_post /extract -d '{"schema":{"type":"object","properties":{"price":{"type":"object"}}}}'
assert_http_error 400 "properties.price.type" "nested object names the path"

pt_post /extract -d "{\"tabId\":\"no-such-tab\",\"schema\":${PRODUCT_SCHEMA}}"
assert_http_status 404 "bogus tabId"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/extract-list.html\"}"
assert_ok "navigate to extract-list.html for the tab route"
LIST_TAB=$(get_tab_id)
pt_get /tabs
assert_ok "list tabs"
assert_json_eq "$RESULT" "[.tabs[] | (.id // .tabId)] | index(\"${LIST_TAB}\") != null" 'true' "/tabs lists the navigated tab"
pt_post "/tabs/${LIST_TAB}/extract" -d "{\"schema\":${PRODUCTS_SCHEMA}}"
assert_ok "tab-scoped extract"
assert_json_eq "$RESULT" '.data.products | length' '6' "tab-scoped route resolves the same page"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "extract: request-level scope confines every field to one subtree"

AMOUNTS_SCHEMA='{"type":"object","properties":{"entries":{"type":"array","items":{"type":"object","properties":{"amount":{"type":"number","description":"price"}}}}}}'

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/extract-list.html\"}"
assert_ok "navigate to extract-list.html"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA}}"
assert_ok "unscoped extract"
assert_json_eq "$RESULT" '.data.entries | length' '6' "unscoped entries come from the 6-product grid"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"role:table\"}"
assert_ok "extract scoped to role:table"
assert_json_eq "$RESULT" '.data.entries | length' '5' "scope role:table reads the 5 table rows"
TABLE_REF=$(echo "$RESULT" | jq -r '.fields.entries.ref')

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"ref:${TABLE_REF}\"}"
assert_ok "extract scoped to ref:${TABLE_REF}"
assert_json_eq "$RESULT" '.data.entries | length' '5' "scope ref:<table ref> reads the same 5 rows"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"${TABLE_REF}\"}"
assert_ok "extract scoped to the bare ref ${TABLE_REF}"
assert_json_eq "$RESULT" '.data.entries | length' '5' "a bare ref scope reads the same 5 rows as ref:"

pt_post /extract -d "{\"schema\":${PRODUCT_SCHEMA},\"scope\":\"ref:e99999\"}"
assert_ok "a scope that matches nothing still answers 200"
assert_json_eq "$RESULT" '.data' '{}' "no data outside a missing scope"
assert_json_eq "$RESULT" '[.fields[].reason] | unique | join(",")' 'scope_not_found' "every field reports scope_not_found"
assert_json_eq "$RESULT" '.missing | sort | join(",")' 'inStock,name,price' "required fields are listed as missing"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"css:table\"}"
assert_http_error 400 "scope: css" "a CSS scope is a 400 naming scope"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"xpath://table\"}"
assert_http_error 400 "scope: xpath" "an XPath scope is a 400 naming scope"

pt_post /extract -d "{\"schema\":${AMOUNTS_SCHEMA},\"scope\":\"//table\"}"
assert_http_error 400 "scope: xpath" "a bare XPath scope is a 400 naming scope"

end_test
