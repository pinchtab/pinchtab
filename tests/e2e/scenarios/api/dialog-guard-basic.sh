#!/bin/bash
# dialog-guard-basic.sh — every route that drives the page refuses a
# dialog-blocked tab instantly with 409 dialog_blocked instead of hanging on
# CDP until its timeout, and a batch whose step opens a dialog stops with a
# coded failed step (PIN-425).

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# The CDP timeout the unguarded routes used to run into is tens of seconds; a
# guarded refusal is answered before the page is touched.
GUARD_MAX_MS=2000

# probe_blocked METHOD PATH [BODY] — asserts a quick 409 dialog_blocked with
# the hint and remedy. curl --max-time turns a regression into a hang-free
# failure instead of a stalled suite.
probe_blocked() {
  local method="$1" path="$2" body="${3:-}"
  local t0 t1 elapsed
  t0=$(get_time_ms)
  if [ -n "$body" ]; then
    pinchtab "$method" "$path" --max-time 5 -d "$body"
  else
    pinchtab "$method" "$path" --max-time 5
  fi
  t1=$(get_time_ms)
  elapsed=$((t1 - t0))

  assert_http_status 409 "$method $path refused"
  assert_json_eq "$RESULT" '.code' 'dialog_blocked' "$method $path code is dialog_blocked"
  assert_json_exists "$RESULT" '.details.remedy' "$method $path carries the remedy"
  assert_json_exists "$RESULT" '.details.hint' "$method $path carries the hint"
  if [ "$elapsed" -lt "$GUARD_MAX_MS" ]; then
    pass_assert "$method $path answered in ${elapsed}ms"
  else
    fail_assert "$method $path took ${elapsed}ms, want under ${GUARD_MAX_MS}ms"
  fi
}

result_holds() {
  local expr="$1" desc="$2"
  if echo "$RESULT" | jq -e "$expr" >/dev/null 2>&1; then
    pass_assert "$desc"
  else
    fail_assert "$desc (got: $RESULT)"
  fi
}

# ─────────────────────────────────────────────────────────────────
start_test "dialog guard: a batch whose click opens an alert stops with a dialog_blocked step"

pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/buttons.html\"}"
assert_ok "navigate to buttons fixture"
TAB_ID=$(get_tab_id)

# stopOnError is off, so only the dialog stop can end the run before the
# third step.
BATCH='{"stopOnError":false,"actions":[{"kind":"click","selector":"#trigger-alert"},{"kind":"press","key":"Tab"},{"kind":"press","key":"Tab"}]}'
pinchtab POST "/tabs/${TAB_ID}/actions" --max-time 10 -d "$BATCH"
assert_ok "batch answers with its step results"
result_holds '[.results[] | select(.success == false and .code == "dialog_blocked")] | length >= 1' \
  "a failed step carries the dialog_blocked code"
result_holds '(.results | length) < 3' "the run stopped before the last step"
result_holds '[.results[] | select(.code == "dialog_blocked")][0].details.remedy != null' \
  "the dialog_blocked step carries the remedy"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "dialog guard: page-driving routes refuse a blocked tab instantly"

# The alert opened by the batch is still pending.
probe_blocked POST "/tabs/${TAB_ID}/navigate" "{\"url\":\"${FIXTURES_URL}/form.html\"}"
probe_blocked POST /navigate "{\"tabId\":\"${TAB_ID}\",\"url\":\"${FIXTURES_URL}/form.html\"}"
probe_blocked POST "/tabs/${TAB_ID}/back" '{}'
probe_blocked POST "/tabs/${TAB_ID}/forward" '{}'
probe_blocked POST "/tabs/${TAB_ID}/reload" '{}'
probe_blocked POST "/tabs/${TAB_ID}/wait" '{"selector":"#trigger-alert","timeout":3000}'
probe_blocked POST "/tabs/${TAB_ID}/wait" '{"ms":50}'
probe_blocked POST "/tabs/${TAB_ID}/find" '{"query":"Trigger Alert button"}'
probe_blocked POST "/tabs/${TAB_ID}/evaluate" '{"expression":"1+1"}'
probe_blocked GET "/tabs/${TAB_ID}/attr?selector=%23trigger-alert&name=id"
probe_blocked GET "/tabs/${TAB_ID}/count?selector=button"
probe_blocked POST "/tabs/${TAB_ID}/actions" '{"actions":[{"kind":"press","key":"Tab"}]}'
probe_blocked GET "/tabs/${TAB_ID}/storage?type=local"
probe_blocked POST /state/save "{\"tabId\":\"${TAB_ID}\",\"name\":\"pin425-dialog-guard\"}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "dialog guard: the refusals leave the dialog pending, and answering it reopens the routes"

# /reload used to dismiss the dialog as a side effect; accepting it now proves
# nothing above answered it.
pt_post /dialog "{\"tabId\":\"${TAB_ID}\",\"action\":\"accept\"}"
assert_ok "the alert was still pending and is accepted"

pt_get "/tabs/${TAB_ID}/count?selector=button"
assert_ok "count answers after the dialog is accepted"

pt_post "/tabs/${TAB_ID}/evaluate" -d '{"expression":"document.getElementById(\"dialog-result\").textContent"}'
assert_ok "evaluate answers after the dialog is accepted"
assert_result_eq '.result' 'ALERT_DISMISSED' "the alert handler completed once accepted"

pt_post "/tabs/${TAB_ID}/wait" -d '{"ms":50}'
assert_ok "fixed-duration wait answers after the dialog is accepted"

pt_get "/tabs/${TAB_ID}/storage?type=local"
assert_ok "storage read answers after the dialog is accepted"

pt_post "/tabs/${TAB_ID}/navigate" -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate answers after the dialog is accepted"

end_test
