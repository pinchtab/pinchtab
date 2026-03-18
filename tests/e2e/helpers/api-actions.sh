#!/bin/bash
# api-actions.sh — Action helpers (click, type, humanClick, humanType)
#
# Higher-level wrappers around pt_post /action for readable tests.

# Click an element by ref.
# Usage: action_click "$ref"
action_click() {
  local ref="$1"
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"$ref\"}" > /dev/null
  assert_ok "click ref=$ref"
}

# Double-click an element by ref.
# Usage: action_dblclick "$ref"
action_dblclick() {
  local ref="$1"
  pt_post /action -d "{\"kind\":\"dblclick\",\"ref\":\"$ref\"}" > /dev/null
  assert_ok "dblclick ref=$ref"
}

# Type text into an element by ref (standard CDP type).
# Usage: action_type "$ref" "hello"
action_type() {
  local ref="$1"
  local text="$2"
  pt_post /action -d "{\"kind\":\"type\",\"ref\":\"$ref\",\"text\":\"$text\"}" > /dev/null
  assert_ok "type '$text' into ref=$ref"
}

# Human-like click by ref (uses mouse events).
# Usage: action_human_click "$ref"
action_human_click() {
  local ref="$1"
  pt_post /action -d "{\"kind\":\"humanClick\",\"ref\":\"$ref\"}" > /dev/null
  assert_ok "humanClick ref=$ref"
}

# Human-like type by ref (character-by-character key events).
# Usage: action_human_type "$ref" "hello"
action_human_type() {
  local ref="$1"
  local text="$2"
  pt_post /action -d "{\"kind\":\"humanType\",\"ref\":\"$ref\",\"text\":\"$text\"}" > /dev/null
  assert_ok "humanType '$text' into ref=$ref"
}

# Human-like type by CSS selector.
# Usage: action_human_type_selector "#email" "hello"
action_human_type_selector() {
  local selector="$1"
  local text="$2"
  pt_post /action -d "{\"kind\":\"humanType\",\"selector\":\"$selector\",\"text\":\"$text\"}" > /dev/null
  assert_ok "humanType '$text' into $selector"
}

# Navigate to a fixture page and wait for load.
# Usage: navigate_fixture "human-type.html"
navigate_fixture() {
  local page="$1"
  local wait="${2:-1}"
  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/$page\"}" > /dev/null
  sleep "$wait"
}

# Take a fresh snapshot (sets $RESULT).
# Usage: fresh_snapshot
fresh_snapshot() {
  pt_get /snapshot > /dev/null
  assert_ok "snapshot"
}
