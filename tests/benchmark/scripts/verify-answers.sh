#!/usr/bin/env bash
#
# One-off verifier for Loop #31 answers. Reads the agent report, matches each
# step's `.answer` against an expected marker/substring set, and invokes
# verify-step.sh with pass/fail. The marker map below is hand-derived from
# BASELINE_TASKS.md "Pass if" criteria.
#
# Usage:
#   ./verify_answers_for_loop31.sh results/agent_benchmark_20260415_233421.json

set -euo pipefail

REPORT="${1:-results/agent_benchmark_20260415_233421.json}"

declare -A EXPECT
# Setup & diagnosis — tightened after manual audit (see Loop #33 notes)
EXPECT[0.1]='status.*ok|status:.*ok'
EXPECT[0.2]='401|missing_token|unauth'
EXPECT[0.3]='200|authed.*200'
EXPECT[0.4]='running'
EXPECT[0.5]='tab.*list|tabs.*returned|active tab|listed.*tab'
EXPECT[0.6]='cleaned|closed|no stale|reused.*tab|no cleanup|single active|already empty'
EXPECT[0.7]='VERIFY_HOME_LOADED_12345|navigated to fixtures|fixtures.*home'
EXPECT[0.8]='[0-9A-F]{32}|tab.*id.*captured'
# Reading & Extracting
EXPECT[1.1]='COUNT_LANGUAGES_12|Programming Languages 12|12 articles'
EXPECT[1.2]='wiki-go|Go \(programming language\)|409 success|clicked'
EXPECT[1.3]='Robert Griesemer.*2009|2009.*Griesemer'
EXPECT[1.4]='FEATURE_COUNT_6|6 key features|6 features'
EXPECT[1.5]='Artificial Intelligence.*Climate Action.*Mars|Artificial Intelligence, Climate Action'
EXPECT[1.6]='24,582.*1,284,930|24,582|Revenue \$1,284,930'
# Search
EXPECT[2.1]='wiki-go|VERIFY_WIKI_GO_LANG_88888'
EXPECT[2.2]='No results|no results|xyznonexistent'
EXPECT[2.3]='Artificial Intelligence|ARTIFICIAL_INTELLIGENCE'
# Form
EXPECT[3.1]='submitted|VERIFY_FORM_SUBMITTED_SUCCESS|FORM_SUBMITTED|SUBMISSION_DATA'
EXPECT[3.2]='[Rr]eset.*button|#reset-btn|[Rr]eset.*present|reset-btn'
# SPA
EXPECT[4.1]='TASK_STATS_TOTAL_3|Total.*3.*Active.*2.*Done.*1|3.*2.*1'
EXPECT[4.2]='TASK_ADDED|AUTOMATE|DEPLOYMENT|high'
EXPECT[4.3]='deleted.*task|TASK_STATS_TOTAL_3|All Tasks.*3|3.*tasks'
# Login
EXPECT[5.1]='INVALID_CREDENTIALS_ERROR|Invalid'
EXPECT[5.2]='VERIFY_LOGIN_SUCCESS_DASHBOARD|SESSION_TOKEN_ACTIVE_TRUE|login success|Dashboard'
# E-commerce
EXPECT[6.1]='149.99.*299.99|Wireless.*Smart Watch|Portable Charger'
EXPECT[6.2]='449.98|CART_ITEM_WIRELESS'
EXPECT[6.3]='VERIFY_CHECKOUT_SUCCESS_ORDER|checkout'
# Content + Interaction
EXPECT[7.1]='COMMENT_POSTED_RATING_5'
EXPECT[7.2]='Developer Tools.*15|15.*Developer|wiki-go|VERIFY_WIKI_GO'
# Error handling
EXPECT[8.1]='404|not found'
EXPECT[8.2]='element.*not found|no element found|no element|clear error'
# Export
EXPECT[9.1]='[Ss]creenshot|\.png.*[0-9]{4,} bytes'
EXPECT[9.2]='[Pp][Dd][Ff]|\.pdf.*[0-9]{4,} bytes'
# Modals
EXPECT[10.1]='Dashboard Settings'
EXPECT[10.2]='THEME_DARK_APPLIED'
# Persistence
EXPECT[11.1]='TASK_PERSISTENT_TEST_FOUND'
EXPECT[11.2]='SESSION_RENEWED|VERIFY_LOGIN_SUCCESS_DASHBOARD'
# Multi-page nav
EXPECT[12.1]='home|VERIFY_HOME_LOADED_12345|PinchTab Benchmark - Home'
EXPECT[12.2]='COUNT_LANGUAGES_12|12.*Programming|Artificial Intelligence|Total Articles|12\+8\+15|COMPARISON_DATA_FOUND'
# Form validation
EXPECT[13.1]='blocked|display:none|valueMissing|not submitted'
EXPECT[13.2]='VERIFY_FORM_SUBMITTED_SUCCESS|OPTIONAL_FIELD_SKIPPED_SUCCESS|submitted'
# Dynamic content
EXPECT[14.1]='ADDITIONAL_PRODUCTS_LOADED'
EXPECT[14.2]='CART_UPDATED_WITH_LAZY_PRODUCT|USB-C'
# Data aggregation
EXPECT[15.1]='1,284,930.*384,930|PROFIT_MARGIN_CALCULATED|Revenue.*Profit'
EXPECT[15.2]='FEATURE_COUNT_6.*7.*5|Go=6.*Python=7.*Rust=5|6, 7, 5|COMPARISON_TABLE_BUILT'
# Hover
EXPECT[16.1]='HOVER_REVEALED_USER_1'
EXPECT[16.2]='HOVER_REVEALED_USER_2'
# Scroll
EXPECT[17.1]='SCROLL_MIDDLE_MARKER'
EXPECT[17.2]='SCROLL_REACHED_FOOTER'
# Download
EXPECT[18.1]='DOWNLOAD_FILE_CONTENT_VERIFIED|143 bytes'
# iFrame
EXPECT[19.1]='IFRAME_INNER_CONTENT_LOADED'
EXPECT[19.2]='IFRAME_INPUT_RECEIVED_HELLO_WORLD'
# Dialogs
EXPECT[20.1]='DIALOG_ALERT_DISMISSED'
EXPECT[20.2]='DIALOG_CONFIRM_CANCELLED'
# Async / awaitPromise
EXPECT[21.1]='ASYNC_PAYLOAD_READY_42'
EXPECT[21.2]='ASYNC_USER_NAME_ADA'
# Drag
EXPECT[22.1]='DROP_ZONE_A_OK'
EXPECT[22.2]='DROP_SEQUENCE=DROP_ZONE_A_OK,DROP_ZONE_B_OK,DROP_ZONE_C_OK'
# Loading
EXPECT[23.1]='VERIFY_LOADING_COMPLETE_88888'
# Keyboard
EXPECT[24.1]='KEYBOARD_ESCAPE_PRESSED'
EXPECT[24.2]='KEYBOARD_KEY_A_PRESSED|KEYBOARD_ENTER_PRESSED|ESC.*A.*ENTER'
# Tabs
EXPECT[25.1]='TAB_SETTINGS_CONTENT'
EXPECT[25.2]='TAB_BILLING_CONTENT'
# Accordion
EXPECT[26.1]='ACCORDION_SECTION_A_OPEN'
EXPECT[26.2]='ACCORDION_SECTION_B_OPEN.*aria-expanded=false|Section A aria-expanded=false'
# Editor
EXPECT[27.1]='EDITOR_CHARS=15|Hello rich text'
EXPECT[27.2]='EDITOR_COMMITTED=Hello rich text'
# Range
EXPECT[28.1]='RANGE_VALUE_90.*BUCKET_HIGH|RANGE_VALUE_90'
EXPECT[28.2]='RANGE_VALUE_10.*BUCKET_LOW|RANGE_VALUE_10'
# Pagination
EXPECT[29.1]='PAGE_2_FIRST_ITEM|PAGE_2_OF_3'
EXPECT[29.2]='PAGE_3_FIRST_ITEM|disabled=true'
# Dropdown
EXPECT[30.1]='DROPDOWN_SELECTED=BETA'
EXPECT[30.2]='DROPDOWN_SELECTED=GAMMA'
# Iframe variants
EXPECT[31.1]='DEEP_CLICKED=YES_LEVEL_3'
EXPECT[32.1]='IFRAME_INPUT_RECEIVED_LATE_WORLD'
EXPECT[33.1]='INLINE_RECEIVED_SRCDOC'
EXPECT[34.1]='SANDBOX_CLICKED=YES'
# Text-heavy
EXPECT[35.1]='ARTICLE_PUBLISHED_2026_04_15.*ARTICLE_WORD_COUNT_MARKER_323|ARTICLE_WORD_COUNT_MARKER_323'
EXPECT[35.2]='FOOTER_COPYRIGHT_MARKER'
EXPECT[36.1]='RESULT_3_TITLE.*RESULT_3_SNIPPET_MARKER|RESULT_3'
EXPECT[36.2]='RESULT_1.*RESULT_6|SERP_RESULT_COUNT_6'
EXPECT[37.1]='a-2'
EXPECT[37.2]='ANSWER_2_BODY_MARKER.*ACCEPTED_ANSWER_ID_A2|ACCEPTED_ANSWER_ID_A2'
EXPECT[38.1]='PLAN_PRO_PRICE_29'
EXPECT[38.2]='PLAN_FREE_PRICE_0.*PLAN_PRO_PRICE_29.*PLAN_ENTERPRISE_PRICE_CUSTOM|PLAN_PRO_PRICE_29'

pass=0
fail=0
failed_ids=()

while IFS='|' read -r id answer; do
  pattern="${EXPECT[$id]:-}"
  if [[ -z "$pattern" ]]; then
    echo "skip $id — no expected pattern defined"
    "$(dirname "${BASH_SOURCE[0]}")/verify-step.sh" --report-file "$REPORT" \
      "${id%.*}" "${id#*.}" skip "no expected pattern" > /dev/null
    continue
  fi
  if printf '%s' "$answer" | grep -qE "$pattern"; then
    "$(dirname "${BASH_SOURCE[0]}")/verify-step.sh" --report-file "$REPORT" \
      "${id%.*}" "${id#*.}" pass "auto: matched /$pattern/" > /dev/null
    pass=$((pass + 1))
  else
    "$(dirname "${BASH_SOURCE[0]}")/verify-step.sh" --report-file "$REPORT" \
      "${id%.*}" "${id#*.}" fail "auto: answer did not match /$pattern/" > /dev/null
    fail=$((fail + 1))
    failed_ids+=("$id")
  fi
done < <(jq -r '.steps[] | "\(.id)|\(.answer)"' "$REPORT")

echo
echo "Verified: $pass pass, $fail fail"
if (( fail > 0 )); then
  echo "Failed ids:"
  printf '  %s\n' "${failed_ids[@]}"
fi
