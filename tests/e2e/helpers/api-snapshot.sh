#!/bin/bash
# api-snapshot.sh — Snapshot ref helpers
#
# Provides functions to find and interact with elements from snapshot results.
# All functions operate on $RESULT (set by pt_get /snapshot).

# Find a ref by role and name. Returns empty string if not found.
# Usage: ref=$(find_ref "textbox" "Email")
find_ref() {
  local role="$1"
  local name="$2"
  echo "$RESULT" | jq -r "[.nodes[] | select(.role==\"$role\" and .name==\"$name\")][0].ref // empty"
}

# Find a ref by name only (any role).
# Usage: ref=$(find_ref_by_name "Submit")
find_ref_by_name() {
  local name="$1"
  echo "$RESULT" | jq -r "[.nodes[] | select(.name==\"$name\")][0].ref // empty"
}

# Find a ref by role only (first match).
# Usage: ref=$(find_ref_by_role "textbox")
find_ref_by_role() {
  local role="$1"
  echo "$RESULT" | jq -r "[.nodes[] | select(.role==\"$role\")][0].ref // empty"
}

# Get the value of a node by role and name.
# Usage: val=$(get_value "textbox" "Email")
get_value() {
  local role="$1"
  local name="$2"
  echo "$RESULT" | jq -r "[.nodes[] | select(.role==\"$role\" and .name==\"$name\")][0].value // empty"
}

# Assert a ref exists, set it to a variable, and log result.
# Usage: require_ref "textbox" "Email" EMAIL_REF
# Returns 1 if not found (caller should skip dependent tests).
require_ref() {
  local role="$1"
  local name="$2"
  local varname="$3"
  local ref
  ref=$(find_ref "$role" "$name")

  if [ -z "$ref" ]; then
    echo -e "  ${RED}✗${NC} could not find $role '$name'"
    ((ASSERTIONS_FAILED++)) || true
    eval "$varname=''"
    return 1
  fi

  echo -e "  ${GREEN}✓${NC} found $role '$name' → $ref"
  ((ASSERTIONS_PASSED++)) || true
  eval "$varname='$ref'"
  return 0
}

# Assert a field has the expected value (substring match).
# Usage: assert_value "textbox" "Email" "test@example.com"
assert_value() {
  local role="$1"
  local name="$2"
  local expected="$3"
  local actual
  actual=$(get_value "$role" "$name")

  if echo "$actual" | grep -qF "$expected"; then
    echo -e "  ${GREEN}✓${NC} $name = '$actual'"
    ((ASSERTIONS_PASSED++)) || true
  else
    echo -e "  ${RED}✗${NC} $name: expected '$expected', got '$actual'"
    ((ASSERTIONS_FAILED++)) || true
  fi
}
