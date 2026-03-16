#!/bin/bash
# 37-idpi.sh — IDPI (Indirect Prompt Injection Detection) on /find and /pdf
#
# Tests content-based IDPI scanning:
#   - E2E_SERVER (main): IDPI enabled, scanContent=true (default), warn mode
#   - E2E_SECURE_SERVER (secure): IDPI enabled, strictMode=true

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
# IDPI test helpers
# ─────────────────────────────────────────────────────────────────

# Navigate to a page on a specific server, return tab ID
# Usage: TAB_ID=$(idpi_setup <base_url> <page_url>)
idpi_setup() {
  local base_url="$1" page_url="$2"
  local old_url="$E2E_SERVER"
  E2E_SERVER="$base_url"
  pt_post /navigate "{\"url\":\"$page_url\"}" >/dev/null
  E2E_SERVER="$old_url"
  sleep 1
  echo "$RESULT" | jq -r '.tabId'
}

# Close a tab on a specific server
idpi_cleanup() {
  local base_url="$1" tab_id="$2"
  curl -sf -X POST "${base_url}/tab" \
    -H "Content-Type: application/json" \
    -d "{\"tabId\":\"$tab_id\",\"action\":\"close\"}" >/dev/null 2>&1 || true
}

# Make a request and capture a response header
# Usage: idpi_request POST <base_url> <path> <body> <header_name>
#        idpi_request GET  <base_url> <path> ""     <header_name>
# Sets: RESULT, HTTP_STATUS, HDR_VALUE
idpi_request() {
  local method="$1" base_url="$2" path="$3" body="$4" header_name="$5"
  echo -e "${BLUE}→ curl -X $method ${base_url}${path}${NC}" >&2
  local tmpheaders=$(mktemp)
  local curl_args=(-s -w "\n%{http_code}" -X "$method" "${base_url}${path}" -H "Content-Type: application/json" -D "$tmpheaders")
  [ -n "$body" ] && curl_args+=(-d "$body")
  local response
  response=$(curl "${curl_args[@]}")
  RESULT=$(echo "$response" | head -n -1)
  HTTP_STATUS=$(echo "$response" | tail -n 1)
  HDR_VALUE=$(grep -i "^${header_name}:" "$tmpheaders" | sed 's/^[^:]*: *//' | tr -d '\r' | head -1)
  rm -f "$tmpheaders"
}

# Assert header is present or absent
assert_header_present() {
  local desc="$1"
  if [ -n "$HDR_VALUE" ]; then
    echo -e "  ${GREEN}✓${NC} $desc: $HDR_VALUE"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (header missing)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

assert_header_absent() {
  local desc="$1"
  if [ -z "$HDR_VALUE" ]; then
    echo -e "  ${GREEN}✓${NC} $desc"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $desc (unexpected: $HDR_VALUE)"
    ((ASSERTIONS_FAILED++)) || true
  fi
}

FIND_BODY='{"query":"continue button","threshold":0.1,"topK":5}'
FIND_CLEAN='{"query":"safe action button","threshold":0.1,"topK":5}'

# ═══════════════════════════════════════════════════════════════════
# /find — WARN MODE (main instance)
# ═══════════════════════════════════════════════════════════════════

start_test "idpi: /find clean page — no warning (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request POST "$E2E_SERVER" "/tabs/${TAB_ID}/find" "$FIND_CLEAN" "X-IDPI-Warning"
assert_ok "/find clean page"
assert_header_absent "no X-IDPI-Warning on clean page"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /find injection page — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SERVER" "/tabs/${TAB_ID}/find" "$FIND_BODY" "X-IDPI-Warning"
assert_ok "/find injection (warn mode returns 200)"
assert_header_present "X-IDPI-Warning header present"
assert_json_exists "$RESULT" ".idpiWarning" "idpiWarning field in body"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: POST /find injection — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SERVER" "/find" "{\"query\":\"malicious paragraph\",\"tabId\":\"$TAB_ID\",\"threshold\":0.1}" "X-IDPI-Warning"
assert_ok "POST /find (warn mode)"
assert_header_present "X-IDPI-Warning on POST /find"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ═══════════════════════════════════════════════════════════════════
# /find — STRICT MODE (secure instance)
# ═══════════════════════════════════════════════════════════════════

start_test "idpi: /find clean page — allowed (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request POST "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/find" "$FIND_CLEAN" "X-IDPI-Warning"
assert_ok "/find clean page (strict mode)"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /find injection page — blocked (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request POST "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/find" "$FIND_BODY" "X-IDPI-Warning"
assert_http_status 403 "/find blocked in strict mode"
assert_contains "$RESULT" "idpi" "403 body mentions IDPI"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ═══════════════════════════════════════════════════════════════════
# /pdf — WARN MODE (main instance)
# ═══════════════════════════════════════════════════════════════════

start_test "idpi: /pdf clean page — no warning (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request GET "$E2E_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf clean page"
assert_header_absent "no X-IDPI-Warning on clean PDF"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf injection page — warns (warn mode)"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf injection (warn mode returns 200)"
assert_header_present "X-IDPI-Warning on injection PDF"
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ═══════════════════════════════════════════════════════════════════
# /pdf — STRICT MODE (secure instance)
# ═══════════════════════════════════════════════════════════════════

start_test "idpi: /pdf clean page — allowed (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-clean.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_ok "/pdf clean page (strict mode)"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf injection page — blocked (strict mode)"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf" "" "X-IDPI-Warning"
assert_http_status 403 "/pdf blocked in strict mode"
assert_contains "$RESULT" "idpi" "403 body mentions IDPI"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test

# ═══════════════════════════════════════════════════════════════════
# EDGE CASES
# ═══════════════════════════════════════════════════════════════════

start_test "idpi: multiple injection phrases — single warning header"
TAB_ID=$(idpi_setup "$E2E_SERVER" "${FIXTURES_URL}/idpi-inject.html")
tmpheaders=$(mktemp)
curl -s -X POST "${E2E_SERVER}/tabs/${TAB_ID}/find" \
  -H "Content-Type: application/json" \
  -D "$tmpheaders" \
  -d "$FIND_BODY" >/dev/null
HDR_COUNT=$(grep -ci "^X-IDPI-Warning:" "$tmpheaders" 2>/dev/null || echo "0")
rm -f "$tmpheaders"
if [ "$HDR_COUNT" -eq 1 ]; then
  echo -e "  ${GREEN}✓${NC} exactly one X-IDPI-Warning header"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} expected 1 X-IDPI-Warning, got $HDR_COUNT"
  ((ASSERTIONS_FAILED++)) || true
fi
idpi_cleanup "$E2E_SERVER" "$TAB_ID"
end_test

# ─────────────────────────────────────────────────────────────────
start_test "idpi: /pdf?raw=true blocked in strict mode"
TAB_ID=$(idpi_setup "$E2E_SECURE_SERVER" "${FIXTURES_URL}/idpi-inject.html")
idpi_request GET "$E2E_SECURE_SERVER" "/tabs/${TAB_ID}/pdf?raw=true" "" "X-IDPI-Warning"
assert_http_status 403 "raw PDF blocked in strict mode"
idpi_cleanup "$E2E_SECURE_SERVER" "$TAB_ID"
end_test
