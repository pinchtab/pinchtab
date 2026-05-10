#!/bin/bash
# screenshot-annotate-basic.sh — annotated screenshot pipeline.
#
# Covers the public contract of /screenshot?annotate=true:
#   - JSON envelope shape (format/base64/annotations)
#   - per-annotation fields (ref, role, box.x/y/w/h with positive size)
#   - selector clip projection (boxes are clip-relative, not viewport)
#   - cleanup of the in-page __pinchtab_annotations__ overlay
#   - returned ref is usable for follow-up actions in the same tab

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# ─────────────────────────────────────────────────────────────────
start_test "screenshot annotate: JSON envelope shape"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

pt_get "/screenshot?annotate=true&format=png"
assert_ok "annotated screenshot"

assert_result_eq '.format' 'png' 'envelope.format = "png"'
assert_result_exists '.base64'      'envelope.base64 present'
assert_result_exists '.annotations' 'envelope.annotations present'
assert_json_length_gte "$RESULT" '.annotations' 1 'has at least one annotation'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot annotate: annotation fields"

# Re-use the form fixture from the previous step.
pt_get "/screenshot?annotate=true&format=png"
assert_ok "annotated screenshot for field check"

FIRST_REF=$(echo "$RESULT" | jq -r '.annotations[0].ref')

assert_result_jq \
  '.annotations[0].ref | test("^[a-z]+[0-9]+$")' \
  'annotations[0].ref looks like e<digits>' \
  'annotations[0].ref is malformed'

# Every box must have positive width and height; otherwise the agent has no
# usable target. Stricter than "field present".
assert_result_jq \
  'all(.annotations[]; .box.w > 0 and .box.h > 0)' \
  'all annotation boxes have positive size' \
  'at least one annotation has zero/negative box size'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot annotate: overlay node removed after capture"

# The injector adds a single id="__pinchtab_annotations__" root and the
# orchestrator removes it on the way out. After a successful capture the
# node must not be present in the live DOM, otherwise it would leak ink
# into subsequent (non-annotated) screenshots and clicks.
pt_post /evaluate -d '{"expression":"document.getElementById(\"__pinchtab_annotations__\") === null"}'
assert_ok "evaluate overlay-removed probe"
assert_result_eq '.result' 'true' 'overlay node removed after capture'

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot annotate: returned ref is clickable"

# A non-annotated snapshot is intentionally NOT taken between the annotate
# call and the click — annotated capture should refresh the tab's ref map
# itself so follow-up actions resolve immediately.
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
pt_get "/screenshot?annotate=true&format=png"
assert_ok "annotated screenshot before click"

# Pick the first annotation that points at an interactive button or link;
# fall back to the first ref if none qualify.
TARGET_REF=$(echo "$RESULT" | jq -r '
  (.annotations[] | select(.role == "button" or .role == "link") | .ref) // .annotations[0].ref
' | head -n1)

if [ -n "$TARGET_REF" ] && [ "$TARGET_REF" != "null" ]; then
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"${TARGET_REF}\"}"
  # Some refs may target submit-only elements that error out for benign
  # reasons; we only require that the ref itself was recognised, i.e. the
  # response is not "ref not found".
  if echo "$RESULT" | grep -qi "ref.*not.*found\|unknown.*ref"; then
    fail_assert "ref ${TARGET_REF} not recognised after annotate"
  else
    pass_assert "ref ${TARGET_REF} resolved by click handler"
  fi
else
  fail_assert "no annotations returned to click"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screenshot annotate: selector clip projects boxes to clip origin"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"

# The fixture form has id="test-form" (see tests/e2e/fixtures/form.html).
# Selector + annotate must clip the screenshot AND narrow the annotation
# set to refs whose rect overlaps that clip. We assert this directly:
# the call must succeed, must return at least one annotation, and every
# returned box must be clip-relative (x >= 0 and y >= 0).
SELECTOR_ENC='%23test-form'
pt_get "/screenshot?annotate=true&format=png&selector=${SELECTOR_ENC}"
assert_ok "annotated selector screenshot"

assert_result_exists '.annotations' 'envelope.annotations present (selector mode)'
assert_json_length_gte "$RESULT" '.annotations' 1 'selector returned at least one annotation'

# Clip-relative invariant: after subtracting target origin, every box must
# start at non-negative coords. A regression that left boxes in viewport
# space would surface here as negative or far-from-zero values.
assert_result_jq \
  'all(.annotations[]; .box.x >= 0 and .box.y >= 0)' \
  'selector annotations are clip-relative (all x,y >= 0)' \
  'selector annotation projection emitted negative coords'

# Sanity: at least one annotation must sit close to the clip origin
# (within 200px). If projection were skipped, the smallest box origin
# would still be at the form's viewport y, well above 200.
assert_result_jq \
  '[.annotations[] | select(.box.y < 200)] | length > 0' \
  'at least one annotation near clip origin' \
  'no annotation near clip origin — projection looks unprojected'

end_test
