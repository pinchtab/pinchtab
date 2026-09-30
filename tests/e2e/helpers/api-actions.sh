#!/bin/bash
# api-actions.sh — Action helpers for click/type variants.
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

# Humanized click by ref (uses mouse events).
# Usage: action_click_humanized "$ref"
action_click_humanized() {
  local ref="$1"
  pt_post /action -d "{\"kind\":\"click\",\"ref\":\"$ref\",\"humanize\":true}" > /dev/null
  assert_ok "humanized click ref=$ref"
}

# Humanized type by ref (character-by-character key events with shorter E2E delay).
# Usage: action_type_humanized "$ref" "hello"
action_type_humanized() {
  local ref="$1"
  local text="$2"
  pt_post /action -d "{\"kind\":\"type\",\"ref\":\"$ref\",\"text\":\"$text\",\"humanize\":true,\"fast\":true}" > /dev/null
  assert_ok "humanized type '$text' into ref=$ref"
}

# Humanized type by CSS selector (character-by-character key events with shorter E2E delay).
# Usage: action_type_humanized_selector "#email" "hello"
action_type_humanized_selector() {
  local selector="$1"
  local text="$2"
  pt_post /action -d "{\"kind\":\"type\",\"selector\":\"$selector\",\"text\":\"$text\",\"humanize\":true,\"fast\":true}" > /dev/null
  assert_ok "humanized type '$text' into $selector"
}

# Navigate to a fixture page; /navigate returns once the page has loaded.
# Usage: navigate_fixture "human-type.html"
navigate_fixture() {
  local page="$1"
  pt_post /navigate -d "{\"url\":\"${FIXTURES_URL}/$page\"}" > /dev/null
}

# Take a fresh snapshot (sets $RESULT).
# Usage: fresh_snapshot
fresh_snapshot() {
  pt_get /snapshot > /dev/null
  assert_ok "snapshot"
}
