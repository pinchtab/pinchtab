#!/bin/bash
# inspect-basic.sh — API happy-path inspection + cookie clearing scenarios.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

# Navigate to form.html and take a snapshot to populate the ref cache.
pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/form.html\"}"
assert_ok "navigate to form.html"

pt_get /snapshot
assert_ok "snapshot to populate ref cache"
FORM_SNAPSHOT="$RESULT"

# Extract refs for various elements we will inspect.
# form.html has: textbox (username), textbox (email), textbox (password),
#                combobox (country), checkbox (terms), button (Submit/Reset).
INPUT_REF=$(find_ref_by_role "textbox")
CHECKBOX_REF=$(echo "$RESULT" | jq -r '[.nodes[] | select(.role == "checkbox") | .ref] | first // empty')
BUTTON_REF=$(find_ref_by_name "Submit")

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /count?selector=input"

pt_get "/count?selector=input"
assert_ok "count inputs"

COUNT=$(echo "$RESULT" | jq '.count')
if [ "$COUNT" -ge 3 ]; then
  echo -e "  ${GREEN}✓${NC} count >= 3 (got: $COUNT)"
  ((ASSERTIONS_PASSED++)) || true
else
  echo -e "  ${RED}✗${NC} count < 3 (got: $COUNT)"
  ((ASSERTIONS_FAILED++)) || true
fi

assert_json_eq "$RESULT" '.selector' 'input' "selector echoed back"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /value by ref"

if assert_ref_found "$INPUT_REF" "textbox ref for /value"; then
  pt_get "/value?ref=${INPUT_REF}"
  assert_ok "get value of textbox"
  assert_json_eq "$RESULT" '.ref' "$INPUT_REF" "ref echoed back"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /attr (type attribute)"

if assert_ref_found "$INPUT_REF" "textbox ref for /attr"; then
  pt_get "/attr?ref=${INPUT_REF}&name=type"
  assert_ok "get type attribute"
  assert_json_eq "$RESULT" '.ref' "$INPUT_REF" "ref echoed back"
  assert_json_eq "$RESULT" '.name' 'type' "attr name echoed back"
  assert_json_exists "$RESULT" '.value' "attr value present"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /box (bounding box)"

if assert_ref_found "$INPUT_REF" "textbox ref for /box"; then
  pt_get "/box?ref=${INPUT_REF}"
  assert_ok "get bounding box"
  assert_json_eq "$RESULT" '.ref' "$INPUT_REF" "ref echoed back"

  BOX_WIDTH=$(echo "$RESULT" | jq '.box.width')
  if [ "$(echo "$BOX_WIDTH > 0" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo -e "  ${GREEN}✓${NC} box width > 0 (got: $BOX_WIDTH)"
    ((ASSERTIONS_PASSED++)) || true
  else
    # Fallback check for environments without bc
    if echo "$BOX_WIDTH" | grep -qvE '^0(\.0+)?$'; then
      echo -e "  ${GREEN}✓${NC} box width > 0 (got: $BOX_WIDTH)"
      ((ASSERTIONS_PASSED++)) || true
    else
      echo -e "  ${RED}✗${NC} box width is 0 or invalid (got: $BOX_WIDTH)"
      ((ASSERTIONS_FAILED++)) || true
    fi
  fi
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /visible (element is visible)"

if assert_ref_found "$INPUT_REF" "textbox ref for /visible"; then
  pt_get "/visible?ref=${INPUT_REF}"
  assert_ok "get visible state"
  assert_json_eq "$RESULT" '.ref' "$INPUT_REF" "ref echoed back"
  assert_json_eq "$RESULT" '.visible' 'true' "input is visible"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /enabled (element is enabled)"

if assert_ref_found "$INPUT_REF" "textbox ref for /enabled"; then
  pt_get "/enabled?ref=${INPUT_REF}"
  assert_ok "get enabled state"
  assert_json_eq "$RESULT" '.ref' "$INPUT_REF" "ref echoed back"
  assert_json_eq "$RESULT" '.enabled' 'true' "input is enabled"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /checked (checkbox is not checked)"

if assert_ref_found "$CHECKBOX_REF" "checkbox ref for /checked"; then
  pt_get "/checked?ref=${CHECKBOX_REF}"
  assert_ok "get checked state"
  assert_json_eq "$RESULT" '.ref' "$CHECKBOX_REF" "ref echoed back"
  assert_json_eq "$RESULT" '.checked' 'false' "checkbox is not checked"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: GET /value missing ref returns 400"

pt_get "/value"
assert_http_status "400" "missing ref returns 400"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: password field values redacted in snapshot"

# Type a value into the password field so there's something to redact.
PASS_REF=$(find_ref_by_role_and_name "textbox" "Password:" "$FORM_SNAPSHOT")
if [ -z "$PASS_REF" ] || [ "$PASS_REF" = "null" ]; then
  PASS_REF=$(echo "$FORM_SNAPSHOT" | jq -r '[.nodes[] | select(.name | test("password";"i")) | .ref] | first // empty')
fi

if [ -n "$PASS_REF" ] && [ "$PASS_REF" != "null" ]; then
  pt_post /action "{\"kind\":\"type\",\"ref\":\"${PASS_REF}\",\"text\":\"supersecret\"}"

  # Take a fresh snapshot and check the password field value.
  pt_get /snapshot
  assert_ok "snapshot after typing password"

  PASS_VALUE=$(echo "$RESULT" | jq -r "[.nodes[] | select(.ref == \"$PASS_REF\")] | first | .value // empty")
  if [ "$PASS_VALUE" = "supersecret" ]; then
    echo -e "  ${RED}✗${NC} password value leaked: $PASS_VALUE"
    ((ASSERTIONS_FAILED++)) || true
  elif [ -n "$PASS_VALUE" ] && echo "$PASS_VALUE" | grep -qF "••••••••"; then
    echo -e "  ${GREEN}✓${NC} password value redacted"
    ((ASSERTIONS_PASSED++)) || true
  elif [ -z "$PASS_VALUE" ]; then
    echo -e "  ${GREEN}✓${NC} password value empty (redacted)"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} unexpected password value: $PASS_VALUE"
    ((ASSERTIONS_FAILED++)) || true
  fi
else
  echo -e "  ${YELLOW}⚠${NC} skipped: could not find password field ref"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "inspect: DELETE /cookies clears cookies"

pt_delete /cookies
assert_ok "clear cookies"
assert_json_eq "$RESULT" '.status' 'cleared' "cookies status cleared"

end_test
