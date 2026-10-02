#!/bin/bash
# network-retain-body.sh — retained network response body smoke.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

start_test "network detail: retained response body"

if [ -z "${E2E_RETAIN_SERVER:-}" ]; then
  skip_test "E2E_RETAIN_SERVER not set"
  end_test
  return 0 2>/dev/null || exit 0
fi

retained_body_checks() {
  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/network-retain-body.html\"}"
  assert_ok "navigate to retention fixture"

  # The fixture uses fetch(), which CDP classifies as Fetch rather than XHR.
  NETWORK_JSON=$(e2e_curl -s "${E2E_SERVER}/network?type=Fetch&limit=20")
  REQ_ID=$(echo "$NETWORK_JSON" | jq -r '.entries[] | select(.url | contains("network-retain-body.json")) | .requestId' | head -n1)
  if [ -z "$REQ_ID" ] || [ "$REQ_ID" = "null" ]; then
    fail_assert "could not find retained-body request in network buffer"
    return 1
  fi

  pass_assert "found request id: $REQ_ID"

  DETAIL=$(e2e_curl -s "${E2E_SERVER}/network/${REQ_ID}?body=true&bodyMode=retained-preferred&timeoutMs=2000")

  echo "$DETAIL" | jq -e '.bodyRetained == true' >/dev/null 2>&1
  if [ $? -eq 0 ]; then
    pass_assert "bodyRetained=true"
  else
    fail_assert "expected bodyRetained=true"
    echo "$DETAIL" | jq .
  fi

  echo "$DETAIL" | jq -e '.bodySource == "retained"' >/dev/null 2>&1
  if [ $? -eq 0 ]; then
    pass_assert "bodySource=retained"
  else
    fail_assert "expected bodySource=retained"
    echo "$DETAIL" | jq .
  fi

  echo "$DETAIL" | jq -e '.responseBody | contains("retained-body-ok")' >/dev/null 2>&1
  if [ $? -eq 0 ]; then
    pass_assert "retained response body contains expected payload"
  else
    fail_assert "retained response body missing expected payload"
    echo "$DETAIL" | jq .
  fi

  echo "$DETAIL" | jq -e '(.bodyPending // false) == false' >/dev/null 2>&1
  if [ $? -eq 0 ]; then
    pass_assert "bounded wait completed without leaving bodyPending=true"
  else
    fail_assert "expected bodyPending=false after bounded wait"
    echo "$DETAIL" | jq .
  fi
}

with_server "$E2E_RETAIN_SERVER" retained_body_checks

end_test
