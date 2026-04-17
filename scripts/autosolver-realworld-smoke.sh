#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

source "${ROOT_DIR}/tests/e2e/helpers/api.sh"
source "${ROOT_DIR}/tests/e2e/helpers/autosolver.sh"

TARGETS_CSV="${REAL_WORLD_AUTOSOLVER_TARGETS:-https://bot.sannysoft.com}"
NAV_SLEEP_SEC="${REAL_WORLD_NAV_SLEEP_SEC:-4}"

autosolver_use_medium_server

cleanup() {
  autosolver_restore_server
}
trap cleanup EXIT

split_csv_targets() {
  local csv="$1"
  local old_ifs="$IFS"
  IFS=',' read -r -a TARGETS <<< "$csv"
  IFS="$old_ifs"
}

extract_text_for_tab() {
  local tab_id="$1"
  pt_get "/text?tabId=${tab_id}"
  if [ "$HTTP_STATUS" != "200" ]; then
    echo ""
    return 1
  fi
  echo "$RESULT" | jq -r '.text // ""'
}

contains_signal() {
  local text_lc="$1"
  local regex="$2"
  if printf '%s' "$text_lc" | grep -Eq "$regex"; then
    echo "true"
  else
    echo "false"
  fi
}

capture_signals_json() {
  local raw_text="$1"
  local text_lc
  text_lc=$(printf '%s' "$raw_text" | tr '[:upper:]' '[:lower:]')

  local bot_behavior cdp_detected navigator_webdriver tampered_functions
  bot_behavior=$(contains_signal "$text_lc" "bot behavior detected|bot detected")
  cdp_detected=$(contains_signal "$text_lc" "cdp detected|devtools protocol")
  navigator_webdriver=$(contains_signal "$text_lc" "navigator.?webdriver|webdriver")
  tampered_functions=$(contains_signal "$text_lc" "tampered functions|tamperedfunctions")

  jq -n \
    --argjson bot "$bot_behavior" \
    --argjson cdp "$cdp_detected" \
    --argjson wd "$navigator_webdriver" \
    --argjson tf "$tampered_functions" \
    '{
      botBehaviorDetected: $bot,
      cdpDetected: $cdp,
      navigatorWebdriver: $wd,
      tamperedFunctions: $tf
    }'
}

log_signal_delta() {
  local label="$1"
  local before_json="$2"
  local after_json="$3"

  echo -e "  ${MUTED}${label}${NC}"
  local key
  for key in botBehaviorDetected cdpDetected navigatorWebdriver tamperedFunctions; do
    local before after marker
    before=$(echo "$before_json" | jq -r ".${key}")
    after=$(echo "$after_json" | jq -r ".${key}")
    marker="="
    if [ "$before" != "$after" ]; then
      marker="->"
    fi
    echo "    ${key}: ${before} ${marker} ${after}"
  done
}

require_server() {
  if e2e_curl -sf "${E2E_SERVER}/health" >/dev/null 2>&1; then
    return 0
  fi

  echo "autosolver smoke: server not reachable at ${E2E_SERVER}" >&2
  echo "Set E2E_SERVER/E2E_SERVER_TOKEN to a running PinchTab instance." >&2
  exit 1
}

run_site() {
  local url="$1"

  echo -e "${BLUE}[autosolver-smoke] navigate:${NC} ${url}"
  pt_post /navigate "{\"url\":\"${url}\"}"
  if [ "$HTTP_STATUS" != "200" ] && [ "$HTTP_STATUS" != "201" ]; then
    echo -e "  ${RED}✗${NC} navigate failed (${HTTP_STATUS})"
    if [ "$HTTP_STATUS" = "403" ] && echo "$RESULT" | grep -qi "Domain not in allowlist"; then
      echo -e "  ${YELLOW}⚠${NC} IDPI allowlist is blocking this host"
    fi
    ((ASSERTIONS_FAILED++)) || true
    return
  fi

  local tab_id
  tab_id=$(echo "$RESULT" | jq -r '.tabId // ""')
  if [ -z "$tab_id" ]; then
    echo -e "  ${RED}✗${NC} missing tabId in navigate response"
    ((ASSERTIONS_FAILED++)) || true
    return
  fi
  echo -e "  ${GREEN}✓${NC} navigated to target"
  ((ASSERTIONS_PASSED++)) || true

  sleep "$NAV_SLEEP_SEC"

  local before_text
  before_text=$(extract_text_for_tab "$tab_id")
  if [ "$HTTP_STATUS" != "200" ]; then
    echo -e "  ${RED}✗${NC} failed to extract pre-solve text (${HTTP_STATUS})"
    ((ASSERTIONS_FAILED++)) || true
    return
  fi
  echo -e "  ${GREEN}✓${NC} captured pre-solve text"
  ((ASSERTIONS_PASSED++)) || true

  local before_signals
  before_signals=$(capture_signals_json "$before_text")

  echo -e "${BLUE}[autosolver-smoke] solve:${NC} ${url}"
  pt_post /solve "{\"tabId\":\"${tab_id}\",\"maxAttempts\":3,\"timeout\":45000}"
  if [ "$HTTP_STATUS" != "200" ]; then
    echo -e "  ${RED}✗${NC} solve failed (${HTTP_STATUS})"
    ((ASSERTIONS_FAILED++)) || true
    return
  fi

  local solver_name solved attempts challenge_type
  solver_name=$(echo "$RESULT" | jq -r '.solver // ""')
  solved=$(echo "$RESULT" | jq -r '.solved // false')
  attempts=$(echo "$RESULT" | jq -r '.attempts // 0')
  challenge_type=$(echo "$RESULT" | jq -r '.challengeType // ""')
  echo -e "  ${MUTED}solver=${solver_name:-auto} solved=${solved} attempts=${attempts} challengeType=${challenge_type}${NC}"
  echo -e "  ${GREEN}✓${NC} solve request completed"
  ((ASSERTIONS_PASSED++)) || true

  sleep 2

  local after_text
  after_text=$(extract_text_for_tab "$tab_id")
  if [ "$HTTP_STATUS" != "200" ]; then
    echo -e "  ${RED}✗${NC} failed to extract post-solve text (${HTTP_STATUS})"
    ((ASSERTIONS_FAILED++)) || true
    return
  fi
  echo -e "  ${GREEN}✓${NC} captured post-solve text"
  ((ASSERTIONS_PASSED++)) || true

  local after_signals
  after_signals=$(capture_signals_json "$after_text")
  log_signal_delta "Detections (before -> after):" "$before_signals" "$after_signals"
}

require_server

start_test "autosolver-realworld-smoke"

split_csv_targets "$TARGETS_CSV"
for target in "${TARGETS[@]}"; do
  target="${target#"${target%%[![:space:]]*}"}"
  target="${target%"${target##*[![:space:]]}"}"
  [ -n "$target" ] || continue
  run_site "$target"
done

end_test
print_summary
