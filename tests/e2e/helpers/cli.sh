#!/bin/bash
# Common CLI E2E entrypoint.

HELPERS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${HELPERS_DIR}/base.sh"

E2E_SUMMARY_TITLE="CLI E2E Test Summary"
E2E_SUMMARY_FILE="summary.txt"
E2E_REF_JSON_VAR="PT_OUT"

pt() {
  local tmpout
  tmpout=$(mktemp)
  local tmperr
  tmperr=$(mktemp)
  local first_arg="${1:-}"
  local -a cmd_prefix=()

  case "$first_arg" in
    ""|config|daemon|security|help|--help|version|--version)
      ;;
    *)
      if [ -n "${E2E_SERVER_TOKEN:-}" ]; then
        cmd_prefix=(env "PINCHTAB_TOKEN=${E2E_SERVER_TOKEN}")
      fi
      ;;
  esac

  echo -e "  ${BLUE}→ pinchtab --server $E2E_SERVER $@${NC}"

  set +e
  "${cmd_prefix[@]}" pinchtab --server "$E2E_SERVER" "$@" > "$tmpout" 2> "$tmperr"
  PT_CODE=$?
  set -e

  PT_OUT=$(cat "$tmpout")
  PT_ERR=$(cat "$tmperr")
  rm -f "$tmpout" "$tmperr"

  if [ -n "$PT_OUT" ]; then
    echo "$PT_OUT" | head -5
  fi
}

pt_ok() {
  pt "$@"
  if [ "$PT_CODE" -eq 0 ]; then
    echo -e "  ${GREEN}✓${NC} exit 0"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} expected exit 0, got $PT_CODE"
    echo -e "  ${RED}stderr: $PT_ERR${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

pt_fail() {
  pt "$@"
  if [ "$PT_CODE" -ne 0 ]; then
    echo -e "  ${GREEN}✓${NC} exit $PT_CODE (expected failure)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} expected non-zero exit, got 0"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_output_contains() {
  local expected="$1"
  local desc="${2:-output contains '$expected'}"

  if echo "$PT_OUT" | grep -q "$expected"; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_exit_code() {
  local expected="$1"
  local desc="${2:-exit code is $expected}"
  if [ "$PT_CODE" -eq "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc (exit $PT_CODE)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (expected $expected, got $PT_CODE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_exit_code_lte() {
  local max="$1"
  local desc="${2:-exit code <= $max}"
  if [ "$PT_CODE" -le "$max" ]; then
    echo -e "  ${GREEN}✓${NC} $desc (exit $PT_CODE)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got $PT_CODE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_json_field_contains() {
  local path="$1"
  local needle="$2"
  local desc="${3:-$path contains '$needle'}"
  local actual
  actual=$(echo "$PT_OUT" | jq -r "$path" 2>/dev/null)
  if [[ "$actual" == *"$needle"* ]]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got '$actual')"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_file_exists() {
  local path="$1"
  local desc="${2:-file exists: $path}"
  if [ -f "$path" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (not found)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

config_version_of() {
  local path="$1"
  jq -r '.configVersion // "none"' "$path"
}

assert_config_version() {
  local path="$1"
  local expected="$2"
  local success_desc="${3:-configVersion is $expected}"
  local actual
  actual=$(config_version_of "$path")

  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $success_desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} expected configVersion $expected, got $actual"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_config_version_one_of() {
  local path="$1"
  shift
  local actual
  actual=$(config_version_of "$path")

  while [ "$#" -gt 1 ]; do
    local expected="$1"
    local success_desc="$2"
    shift 2
    if [ "$actual" = "$expected" ]; then
      echo -e "  ${GREEN}✓${NC} $success_desc"
      ((ASSERTIONS_PASSED++)) || true
      return 0
    fi
  done

  echo -e "  ${RED}✗${NC} unexpected configVersion: $actual"
  ((ASSERTIONS_FAILED++)) || true
  return 1
}

assert_output_not_contains() {
  local forbidden="$1"
  local desc="${2:-output does not contain '$forbidden'}"

  if echo "$PT_OUT" | grep -q "$forbidden"; then
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  else
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  fi
}

assert_output_json() {
  local desc="${1:-output is valid JSON}"

  if echo "$PT_OUT" | jq . >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc"
    echo -e "  ${RED}  output was: $PT_OUT${NC}"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_json_field() {
  local path="$1"
  local expected="$2"
  local desc="${3:-$path equals '$expected'}"
  local actual
  actual=$(echo "$PT_OUT" | jq -r "$path" 2>/dev/null)

  if [ "$actual" = "$expected" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (got '$actual')"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_output_jq() {
  local expr="$1"
  local success_desc="$2"
  local fail_desc="${3:-$2}"
  shift 3
  assert_ref_json_jq "$expr" "$success_desc" "$fail_desc" "$@"
}

assert_output_has_tab_event() {
  local tab_id="$1"
  local path="$2"
  local success_desc="$3"
  local fail_desc="$4"
  assert_output_jq \
    '.events[] | select(.tabId == $tab and .path == $path)' \
    "$success_desc" \
    "$fail_desc" \
    --arg tab "$tab_id" \
    --arg path "$path"
}

assert_output_all_events_for_tab() {
  local tab_id="$1"
  local success_desc="$2"
  local fail_desc="$3"
  assert_output_jq \
    'all(.events[]?; .tabId == $tab)' \
    "$success_desc" \
    "$fail_desc" \
    --arg tab "$tab_id"
}
