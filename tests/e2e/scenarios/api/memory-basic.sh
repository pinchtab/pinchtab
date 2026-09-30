#!/bin/bash
# memory-basic.sh — GET /memory tracks a leak and its release; POST /memory/snapshot
# writes a heap snapshot the summary endpoint reads. The default e2e server runs with
# security.allowMemory on; memory-extended.sh covers the refusal on a server with it off.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

MB=$((1024 * 1024))

# ─────────────────────────────────────────────────────────────────
start_test "memory: usedJSHeapSize grows with three leaks and falls back after release + gc"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/memory-leak.html\"}"
assert_ok "navigate to memory-leak.html"
TAB_ID=$(get_tab_id)

pt_get "/memory?tabId=${TAB_ID}&gc=true"
assert_ok "baseline read with gc"
for key in usedJSHeapSize totalJSHeapSize jsHeapSizeLimit documents nodes listeners frames; do
  assert_json_jq "$RESULT" "(.${key} | type) == \"number\"" "usage reports numeric ${key}" "${key} missing or not a number"
done
assert_json_jq "$RESULT" '.usedJSHeapSize > 0 and .jsHeapSizeLimit >= .totalJSHeapSize' "heap sizes are positive and ordered" "heap sizes missing or out of order"
BASELINE=$(echo "$RESULT" | jq -r '.usedJSHeapSize')

for i in 1 2 3; do
  pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#leak\"}"
  assert_ok "leak click ${i}"
done

pt_get "/memory?tabId=${TAB_ID}"
assert_ok "read after three leaks"
LEAKED=$(echo "$RESULT" | jq -r '.usedJSHeapSize')
if [ "$LEAKED" -ge $((BASELINE + 10 * MB)) ]; then
  pass_assert "usedJSHeapSize grew by at least 10 MB ($BASELINE → $LEAKED)"
else
  fail_assert "usedJSHeapSize grew only from $BASELINE to $LEAKED, want at least +10 MB"
fi

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#release\"}"
assert_ok "release click"

pt_get "/memory?tabId=${TAB_ID}&gc=true"
assert_ok "read after release with gc"
assert_json_eq "$RESULT" '.gc' 'true' "reading says it collected first"
RELEASED=$(echo "$RESULT" | jq -r '.usedJSHeapSize')
if [ "$RELEASED" -lt $((BASELINE + 2 * MB)) ]; then
  pass_assert "usedJSHeapSize back under baseline + 2 MB ($RELEASED < $BASELINE + 2 MB)"
else
  fail_assert "usedJSHeapSize $RELEASED after release + gc, want under $((BASELINE + 2 * MB))"
fi

pt_get "/tabs/${TAB_ID}/memory"
assert_ok "tab-scoped usage route"
assert_json_eq "$RESULT" '.tabId' "$TAB_ID" "tab-scoped read names the tab"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "memory: snapshot writes a server-side file whose summary lists Array"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#leak\"}"
assert_ok "leak one array so the snapshot holds it"

pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "take heap snapshot"
SNAP_ID=$(echo "$RESULT" | jq -r '.id')
assert_json_contains "$RESULT" '.path' '/heapsnapshots/' "path is under the server-controlled heapsnapshots dir"
assert_json_contains "$RESULT" '.path' '.heapsnapshot' "path carries the .heapsnapshot extension"
assert_json_jq "$RESULT" '.bytes > 0' "snapshot has bytes" "snapshot reported no bytes"
assert_json_jq "$RESULT" '.nodeCount > 0' "snapshot reports a node count" "snapshot reported no nodes"
assert_json_exists "$RESULT" '.durationMs' "snapshot reports its duration"

pt_get "/memory/snapshot/${SNAP_ID}/summary?top=50"
assert_ok "summary reads the file the snapshot wrote"
assert_json_jq "$RESULT" '.nodeCount > 0 and .edgeCount > 0' "summary counts nodes and edges" "summary counted no nodes or edges"
assert_json_jq "$RESULT" '([.topBySize[].name] + [.topByCount[].name]) | index("Array") != null' "Array is among the top constructors" "Array missing from the top constructors"
assert_json_exists "$RESULT" '.duplicateStrings' "summary lists duplicate strings"

pt_post "/tabs/${TAB_ID}/memory/snapshot" -d '{}'
assert_ok "tab-scoped snapshot route"
assert_json_eq "$RESULT" '.tabId' "$TAB_ID" "tab-scoped snapshot names the tab"
TAB_SNAP_ID=$(echo "$RESULT" | jq -r '.id')
if [ -n "$TAB_SNAP_ID" ] && [ "$TAB_SNAP_ID" != "$SNAP_ID" ]; then
  pass_assert "tab-scoped snapshot got its own id ($TAB_SNAP_ID)"
else
  fail_assert "tab-scoped snapshot id '$TAB_SNAP_ID' is empty or reuses '$SNAP_ID'"
fi

pt_get /health
assert_ok "health"
assert_json_jq "$RESULT" '(.security.enabledSensitiveEndpoints // []) | index("memory") != null' \
  "health lists memory among the enabled capabilities" "health does not list memory although allowMemory is on"

pt_get "/memory/snapshot/heap_does_not_exist/summary"
assert_http_status 404 "unknown snapshot id"
assert_json_eq "$RESULT" '.code' 'memory_snapshot_not_found' "unknown id is memory_snapshot_not_found"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#release\"}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "memory: compare puts (array) growth of three leaks at the top, and a negative delta after release"

pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "baseline snapshot"
BASE_ID=$(echo "$RESULT" | jq -r '.id')

for i in 1 2 3; do
  pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#leak\"}"
  assert_ok "leak click ${i}"
done

pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "snapshot after three leaks"
LEAK_ID=$(echo "$RESULT" | jq -r '.id')

pt_get "/memory/compare?base=${BASE_ID}&head=${LEAK_ID}&top=20"
assert_ok "compare baseline to leaked"
assert_json_eq "$RESULT" '.base.id' "$BASE_ID" "compare names the base snapshot"
assert_json_eq "$RESULT" '.head.id' "$LEAK_ID" "compare names the head snapshot"
assert_json_eq "$RESULT" '.constructors[0].name' '(array)' "the leaked double arrays' backing stores top the table"
assert_json_jq "$RESULT" ".constructors[0].sizeDelta >= $((10 * MB))" \
  "top row grew by at least 10 MB" "top row grew by less than 10 MB"
assert_json_jq "$RESULT" '.sizeDelta > 0 and .changed >= 1' "totals report growth" "totals report no growth"
assert_json_jq "$RESULT" '[.constructors[] | has("retainedSize")] | any | not' \
  "no retainedSize without retained=true" "retainedSize present without retained=true"
assert_json_exists "$RESULT" '.newDuplicateStrings' "compare lists new duplicate strings"

pt_get "/memory/compare?base=${BASE_ID}&head=${LEAK_ID}&top=5&retained=true"
assert_ok "compare with retained sizes"
assert_json_eq "$RESULT" '.retained' 'true' "response says retained"
assert_json_jq "$RESULT" "[.constructors[] | has(\"retainedSize\")] | all" \
  "every row carries retainedSize" "a row lacks retainedSize"
assert_json_jq "$RESULT" ".constructors[0].retainedSize >= $((10 * MB))" \
  "(array) retains at least 10 MB" "(array) retains under 10 MB"

pt_post /action -d "{\"tabId\":\"${TAB_ID}\",\"kind\":\"click\",\"selector\":\"#release\"}"
assert_ok "release click"
pt_get "/memory?tabId=${TAB_ID}&gc=true"
assert_ok "collect garbage after release"

pt_post /memory/snapshot -d "{\"tabId\":\"${TAB_ID}\"}"
assert_ok "snapshot after release"
RELEASED_ID=$(echo "$RESULT" | jq -r '.id')

pt_get "/memory/compare?base=${LEAK_ID}&head=${RELEASED_ID}&top=20"
assert_ok "compare leaked to released"
assert_json_eq "$RESULT" '.constructors[0].name' '(array)' "the released backing stores top the table"
assert_json_jq "$RESULT" ".constructors[0].sizeDelta <= -$((10 * MB))" \
  "top row shrank by at least 10 MB" "top row did not shrink by 10 MB"

pt_get "/memory/compare?base=${BASE_ID}&head=heap_does_not_exist"
assert_http_status 404 "unknown head id"
assert_json_eq "$RESULT" '.code' 'memory_snapshot_not_found' "unknown id is memory_snapshot_not_found"
assert_json_eq "$RESULT" '.details.id' 'heap_does_not_exist' "refusal names the missing id"

pt_get "/memory/compare?base=heap_does_not_exist&head=${LEAK_ID}"
assert_http_status 404 "unknown base id"
assert_json_eq "$RESULT" '.details.id' 'heap_does_not_exist' "refusal names the missing base id"

pt_get "/memory/compare?base=..%2F..%2Fetc%2Fpasswd&head=${LEAK_ID}"
assert_http_status 400 "traversal-looking base id refused"
assert_json_eq "$RESULT" '.code' 'bad_snapshot_id' "traversal id is bad_snapshot_id"

pt_get "/memory/compare?base=${BASE_ID}&head=..%2F${LEAK_ID}"
assert_http_status 400 "traversal-looking head id refused"
assert_json_eq "$RESULT" '.code' 'bad_snapshot_id' "traversal head id is bad_snapshot_id"

pt_get "/memory/compare?head=${LEAK_ID}"
assert_http_status 400 "missing base refused"
assert_json_eq "$RESULT" '.code' 'bad_snapshot_id' "missing base is bad_snapshot_id"

pt_get "/memory/compare?base=${BASE_ID}&head=${LEAK_ID}&retained=maybe"
assert_http_status 400 "non-boolean retained refused"
assert_json_eq "$RESULT" '.code' 'bad_retained' "non-boolean retained is bad_retained"

end_test
