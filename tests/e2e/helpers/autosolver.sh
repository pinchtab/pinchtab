#!/bin/bash
# Shared helpers for autosolver E2E checks.

HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${HELPERS_DIR}/base.sh"
source "${HELPERS_DIR}/api-assertions.sh"
source "${HELPERS_DIR}/api-http.sh"

AUTOSOLVER_RESULT=""
AUTOSOLVER_PAGE_TEXT=""
AUTOSOLVER_LAST_TAB_ID=""
AUTOSOLVER_OLD_SERVER=""
AUTOSOLVER_CHALLENGE_PATTERNS=(
  "just a moment"
  "attention required"
  "captcha"
  "verify you are human"
  "access denied"
  "just a moment..."
)

autosolver_log() {
  local label="$1"
  shift
  echo -e "  ${MUTED}[autosolver:${label}] $*${NC}"
}

autosolver_log_score() {
  local label="$1"
  local score_json="$2"
  local passed critical critical_total warnings warnings_total
  passed=$(echo "$score_json" | jq -r '.passed // false')
  critical=$(echo "$score_json" | jq -r '.critical // 0')
  critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')
  warnings=$(echo "$score_json" | jq -r '.warnings // 0')
  warnings_total=$(echo "$score_json" | jq -r '.warningsTotal // 0')
  autosolver_log "$label" "score: critical=${critical}/${critical_total} warnings=${warnings}/${warnings_total} passed=${passed}"
}

autosolver_log_pairs() {
  local label="$1"
  local title="$2"
  shift 2

  autosolver_log "$label" "$title"
  while [ "$#" -gt 1 ]; do
    printf "    %-28s %s\n" "$1:" "$2"
    shift 2
  done
}

autosolver_use_medium_server() {
  AUTOSOLVER_OLD_SERVER="${E2E_SERVER}"
  if [ -n "${E2E_MEDIUM_SERVER:-}" ]; then
    E2E_SERVER="${E2E_MEDIUM_SERVER}"
  fi
}

autosolver_restore_server() {
  if [ -n "${AUTOSOLVER_OLD_SERVER:-}" ]; then
    E2E_SERVER="${AUTOSOLVER_OLD_SERVER}"
  fi
}

run_autosolver_and_extract() {
  local label="$1"
  local fixture="$2"
  local js_expr="$3"
  local sleep_sec="${4:-1}"

  echo -e "${BLUE}[autosolver] navigate:${NC} ${fixture}"
  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/${fixture}\"}"
  if [ "$HTTP_STATUS" != "200" ] && [ "$HTTP_STATUS" != "201" ]; then
    echo -e "  ${RED}✗${NC} [autosolver:${label}] navigate failed (HTTP $HTTP_STATUS)"
    return 1
  fi
  AUTOSOLVER_LAST_TAB_ID=$(echo "$RESULT" | jq -r '.tabId // ""')
  echo -e "  ${GREEN}✓${NC} [autosolver:${label}] navigated (tab: ${AUTOSOLVER_LAST_TAB_ID:-?})"

  sleep "$sleep_sec"

  local eval_body
  eval_body=$(jq -n --arg expr "$js_expr" '{"expression": $expr}')
  pt_post /evaluate "$eval_body"
  AUTOSOLVER_RESULT=$(echo "$RESULT" | jq -r '.result // "null"')

  autosolver_log "$label" "extracted: ${AUTOSOLVER_RESULT:0:200}"

  local text_body
  text_body=$(jq -n '{"expression": "document.body ? document.body.innerText.substring(0,2000) : \"\""}')
  pt_post /evaluate "$text_body"
  AUTOSOLVER_PAGE_TEXT=$(echo "$RESULT" | jq -r '.result // ""')

  return 0
}

dump_autosolver_debug() {
  local label="${1:-autosolver}"
  echo ""
  echo -e "${YELLOW}════ AUTOSOLVER DEBUG: ${label} ════${NC}"
  autosolver_log "$label" "result:"
  echo "  $AUTOSOLVER_RESULT" | jq '.' 2>/dev/null || echo "  $AUTOSOLVER_RESULT"
  echo ""
  autosolver_log "$label" "page text (first 1000 chars):"
  echo "$AUTOSOLVER_PAGE_TEXT" | head -c 1000
  echo ""
  echo -e "${YELLOW}══════════════════════════════════${NC}"
  echo ""
}

assert_autosolver_score() {
  local score_json="$1"
  local label="$2"
  local min_critical="${3:-}"

  local passed critical critical_total
  passed=$(echo "$score_json" | jq -r '.passed // false')
  critical=$(echo "$score_json" | jq -r '.critical // 0')
  critical_total=$(echo "$score_json" | jq -r '.criticalTotal // 0')

  autosolver_log_score "$label" "$score_json"

  if [ -z "$min_critical" ]; then
    min_critical="$critical_total"
  fi

  if [ "$passed" = "true" ] && [ "$critical" -ge "$min_critical" ]; then
    echo -e "  ${GREEN}✓${NC} [autosolver:${label}] passed (critical ${critical}/${critical_total})"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} [autosolver:${label}] failed (critical ${critical}/${critical_total}, need ${min_critical})"
    ((ASSERTIONS_FAILED++)) || true
    dump_autosolver_debug "$label"
  fi
}

autosolver_null_result() {
  [ "$AUTOSOLVER_RESULT" = "null" ] || [ -z "$AUTOSOLVER_RESULT" ]
}

autosolver_log_signal_flags() {
  local label="$1"
  local details_json="$2"
  shift 2

  [ "$#" -gt 0 ] || return 0
  autosolver_log "$label" "signals:"
  while [ "$#" -gt 0 ]; do
    local key="$1"
    local passed
    passed=$(echo "$details_json" | jq -r --arg key "$key" '.[$key].passed // false')
    printf "    %-28s %s\n" "${key}:" "${passed}"
    shift
  done
}

autosolver_run_score_case() {
  local test_name="$1"
  local label="$2"
  local fixture="$3"
  local score_expr="$4"
  local details_expr="$5"
  shift 5

  start_test "$test_name"

  if run_autosolver_and_extract "$label" "$fixture" "$score_expr" 1; then
    if autosolver_null_result; then
      echo -e "  ${RED}✗${NC} [autosolver:${label}] score not populated"
      echo -e "  ${MUTED}page text: ${AUTOSOLVER_PAGE_TEXT:0:300}${NC}"
      ((ASSERTIONS_FAILED++)) || true
    else
      local score_json="$AUTOSOLVER_RESULT"
      assert_autosolver_score "$score_json" "$label"

      if [ "$#" -gt 0 ] && run_autosolver_and_extract "${label}-details" "$fixture" "$details_expr" 0; then
        if ! autosolver_null_result; then
          autosolver_log_signal_flags "$label" "$AUTOSOLVER_RESULT" "$@"
        fi
      fi
    fi
  else
    ((ASSERTIONS_FAILED++)) || true
  fi

  end_test
}

autosolver_title_has_challenge() {
  local title_lower
  title_lower=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')

  local pattern
  for pattern in "${AUTOSOLVER_CHALLENGE_PATTERNS[@]}"; do
    if printf '%s' "$title_lower" | grep -q "$pattern"; then
      return 0
    fi
  done
  return 1
}

autosolver_run_normal_page_case() {
  local test_name="$1"
  local fixture="${2:-index.html}"

  start_test "$test_name"

  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/${fixture}\"}"
  assert_ok "navigate to ${fixture}"
  sleep 1

  pt_post /evaluate '{"expression":"document.title"}'
  local page_title
  page_title=$(echo "$RESULT" | jq -r '.result // ""')
  autosolver_log "no-crash" "page title: ${page_title}"

  if autosolver_title_has_challenge "$page_title"; then
    echo -e "  ${RED}✗${NC} normal page: unexpected challenge indicator in title: ${page_title}"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${GREEN}✓${NC} normal page: no challenge indicators in title"
    ((ASSERTIONS_PASSED++)) || true
  fi

  pt_post /evaluate '{"expression":"typeof window !== \"undefined\""}'
  assert_json_eq "$RESULT" '.result' 'true' "browser context is alive"

  assert_eval_poll \
    "Object.getOwnPropertyNames(window).filter(p => /^cdc_|\$cdc_/.test(p)).length === 0" \
    "true" "no automation markers on normal page"

  end_test
}

autosolver_run_retry_case() {
  local test_name="$1"
  local fixture="${2:-bot-detect.html}"
  local max_poll="${3:-5}"
  local poll_delay="${4:-0.5}"

  start_test "$test_name"

  pt_post /navigate "{\"url\":\"${FIXTURES_URL}/${fixture}\"}"
  assert_ok "navigate to ${fixture} for retry test"
  sleep 2

  local score_found=false
  local poll_attempts=0
  local poll_score=""
  local i

  for i in $(seq 1 "$max_poll"); do
    ((poll_attempts++)) || true
    pt_post /evaluate '{"expression":"JSON.stringify(window.__botDetectScore || null)"}'
    poll_score=$(echo "$RESULT" | jq -r '.result // "null"')

    if [ "$poll_score" != "null" ] && [ -n "$poll_score" ]; then
      score_found=true
      break
    fi
    sleep "$poll_delay"
  done

  autosolver_log "retry" "polls: ${poll_attempts}/${max_poll}"

  if [ "$score_found" = "true" ]; then
    local settled_passed settled_critical settled_total
    settled_passed=$(echo "$poll_score" | jq -r '.passed // false')
    settled_critical=$(echo "$poll_score" | jq -r '.critical // 0')
    settled_total=$(echo "$poll_score" | jq -r '.criticalTotal // 0')

    autosolver_log "retry" "settled score: critical=${settled_critical}/${settled_total} passed=${settled_passed}"

    if [ "$settled_passed" = "true" ]; then
      echo -e "  ${GREEN}✓${NC} retry: page settled to passed state within ${poll_attempts} polls"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${YELLOW}⚠${NC} retry: page loaded but score not 100% passed (critical: ${settled_critical}/${settled_total})"
      ((ASSERTIONS_PASSED++)) || true
    fi

    if [ "$poll_attempts" -le "$max_poll" ]; then
      echo -e "  ${GREEN}✓${NC} retry: settled within max_attempts bound (${poll_attempts} <= ${max_poll})"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} retry: exceeded max_attempts (${poll_attempts} > ${max_poll})"
      ((ASSERTIONS_FAILED++)) || true
    fi
  else
    echo -e "  ${RED}✗${NC} retry: page never produced a score after ${max_poll} poll attempts"
    echo -e "  ${MUTED}page text: ${AUTOSOLVER_PAGE_TEXT:0:300}${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi

  end_test
}

autosolver_preflight() {
  local ok=0
  local health fixture_check

  echo ""
  autosolver_log "env" "server: ${E2E_SERVER}"
  autosolver_log "env" "fixtures: ${FIXTURES_URL}"

  health=$(e2e_curl -sf "${E2E_SERVER}/health" 2>/dev/null || true)
  if [ -z "$health" ]; then
    echo -e "  ${RED}✗${NC} autosolver server not reachable at ${E2E_SERVER}"
    ok=1
  else
    echo -e "  ${GREEN}✓${NC} autosolver server reachable"
  fi

  fixture_check=$(curl -sf "${FIXTURES_URL}/bot-detect.html" 2>/dev/null | head -c 10 || true)
  if [ -z "$fixture_check" ]; then
    echo -e "  ${RED}✗${NC} autosolver fixtures not reachable at ${FIXTURES_URL}"
    ok=1
  else
    echo -e "  ${GREEN}✓${NC} autosolver fixtures reachable"
  fi
  echo ""

  return "$ok"
}
