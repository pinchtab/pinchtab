#!/bin/bash
# tabs-smoke.sh — slower CLI tab lifecycle smoke scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

MAX_TABS=10

# ─────────────────────────────────────────────────────────────────
start_test "tab eviction: open tabs up to limit"

if ! wait_for_instance_ready "$E2E_SERVER" 30; then
  fail_assert "instance ready before tab eviction"
  TAB_IDS=()
  end_test
  return 0 2>/dev/null || exit 0
fi

TAB_IDS=()
for i in $(seq 1 $MAX_TABS); do
  pt_ok nav --new-tab "${FIXTURES_URL}/index.html?t=$i"
  if [ "$PT_CODE" -eq 0 ]; then
    TAB_IDS+=($(echo "$PT_OUT" | tr -d '[:space:]'))
  fi
done

pt_ok tab --json
TAB_COUNT=$(echo "$PT_OUT" | jq '.tabs | length')
if [ "$TAB_COUNT" -ge "$MAX_TABS" ]; then
  pass_assert "$TAB_COUNT tabs open (>= $MAX_TABS)"
else
  fail_assert "expected >= $MAX_TABS tabs, got $TAB_COUNT"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "tab eviction: new tab evicts oldest"

if [ "${#TAB_IDS[@]}" -eq 0 ]; then
  skip_assert "no tabs opened, skipping eviction assertion"
  end_test
  return 0 2>/dev/null || exit 0
fi

FIRST_TAB="${TAB_IDS[0]}"
sleep 0.1
pt_ok nav --new-tab "${FIXTURES_URL}/index.html?t=overflow"

pt_ok tab
assert_output_not_contains "$FIRST_TAB" "oldest tab evicted (LRU)"

end_test
