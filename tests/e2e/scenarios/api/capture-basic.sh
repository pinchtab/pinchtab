#!/bin/bash
# capture-basic.sh — /capture endpoint happy paths.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab capture (default JSON envelope)"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"

pt_get /capture
assert_ok "capture"

assert_json_exists "$RESULT" '.status' "response.status"
assert_json_exists "$RESULT" '.epoch.domEpoch' "epoch.domEpoch present"
# jq -e treats `false` as missing — use has() since navigated may be false.
assert_json_exists "$RESULT" '.pairing | has("navigated")' "pairing.navigated present"
assert_json_exists "$RESULT" '.image.path' "image written to disk"
assert_json_exists "$RESULT" '.snapshot.nodes' "snapshot.nodes present"
assert_json_exists "$RESULT" '.image.coordinateSpace' "image.coordinateSpace present"
assert_result_eq '.snapshot.filter' 'interactive' 'default filter=interactive'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "pinchtab capture --tab <id>"

pt_get /tabs
TAB_ID=$(get_first_tab)

pt_get "/tabs/${TAB_ID}/capture"
assert_ok "tab capture"
assert_json_exists "$RESULT" '.tabId' "tabId echoed"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: viewport coordinate space by default"

pt_get "/capture"
assert_ok "capture default"
SPACE=$(echo "$RESULT" | jq -r '.image.coordinateSpace')
if [ "$SPACE" = "viewport" ]; then
  pass_assert "coordinateSpace=viewport"
else
  fail_assert "expected coordinateSpace=viewport, got $SPACE"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: beyondViewport switches coordinateSpace to document"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/tall.html\"}"

pt_get "/capture?beyondViewport=true"
assert_ok "capture beyondViewport"
SPACE=$(echo "$RESULT" | jq -r '.image.coordinateSpace')
if [ "$SPACE" = "document" ]; then
  pass_assert "coordinateSpace=document under beyondViewport"
else
  fail_assert "expected coordinateSpace=document, got $SPACE"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: scale reduces image bytes"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"

FULL_BYTES=$(e2e_curl -s "${E2E_SERVER}/capture?output=inline" | jq -r '.image.bytes')
HALF_BYTES=$(e2e_curl -s "${E2E_SERVER}/capture?output=inline&scale=0.25" | jq -r '.image.bytes')

if [ "$HALF_BYTES" -lt "$FULL_BYTES" ]; then
  pass_assert "scale=0.25 ($HALF_BYTES bytes) < scale=1 ($FULL_BYTES bytes)"
else
  fail_assert "scale=0.25 did not shrink the image ($HALF_BYTES vs $FULL_BYTES)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: withBounds populates boundingBox on interactive nodes"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_get "/capture?withBounds=true&filter=interactive"
assert_ok "capture withBounds"

HAS_BOUNDS=$(echo "$RESULT" | jq '[.snapshot.nodes[] | select(.boundingBox != null)] | length')
if [ "$HAS_BOUNDS" -gt 0 ]; then
  pass_assert "$HAS_BOUNDS nodes with boundingBox"
else
  fail_assert "no nodes carry boundingBox"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: withBounds=false skips boundingBox"

pt_get "/capture?withBounds=false&filter=interactive"
assert_ok "capture withBounds=false"

HAS_BOUNDS=$(echo "$RESULT" | jq '[.snapshot.nodes[] | select(.boundingBox != null)] | length')
if [ "$HAS_BOUNDS" -eq 0 ]; then
  pass_assert "no boundingBox under withBounds=false"
else
  fail_assert "expected no boundingBox, found $HAS_BOUNDS"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: selector bounds are clip-relative"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

SELECTOR_ENC='%23test-form'
pt_get "/capture?selector=${SELECTOR_ENC}&withBounds=true&filter=interactive"
assert_ok "capture selector"

assert_result_eq '.image.coordinateSpace' 'clip' 'coordinateSpace=clip under selector'
assert_result_exists '.image.clip' 'image.clip present'
assert_result_jq \
  '[.snapshot.nodes[] | select(.boundingBox != null)] | length > 0' \
  'selector returned bounded nodes' \
  'selector capture returned no bounded nodes'
assert_result_jq \
  '[.snapshot.nodes[] | select(.boundingBox != null and .boundingBox.y < 200)] | length > 0' \
  'at least one selector bound near clip origin' \
  'selector bounds look unprojected'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: epoch token is unique across calls"

pt_get /capture
EPOCH_A=$(echo "$RESULT" | jq -r '.epoch.domEpoch')

pt_get /capture
EPOCH_B=$(echo "$RESULT" | jq -r '.epoch.domEpoch')

if [ -n "$EPOCH_A" ] && [ -n "$EPOCH_B" ] && [ "$EPOCH_A" != "$EPOCH_B" ]; then
  pass_assert "epochs differ ($EPOCH_A != $EPOCH_B)"
else
  fail_assert "epochs not unique ($EPOCH_A vs $EPOCH_B)"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "capture: output=file writes image to disk"

pt_get "/capture?output=file"
assert_ok "capture output=file"
assert_json_exists "$RESULT" '.image.path' "image.path returned"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "/screenshot ?scale= reduces image bytes"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/table.html\"}"
FULL=$(e2e_curl -s "${E2E_SERVER}/screenshot" | wc -c)
HALF=$(e2e_curl -s "${E2E_SERVER}/screenshot?scale=0.25" | wc -c)

if [ "$HALF" -lt "$FULL" ]; then
  pass_assert "screenshot scale=0.25 ($HALF bytes) < scale=1 ($FULL bytes)"
else
  fail_assert "screenshot scale=0.25 did not shrink ($HALF vs $FULL)"
fi

end_test
