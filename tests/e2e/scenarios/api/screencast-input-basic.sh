#!/bin/bash
# screencast-input.sh — API scenarios for dashboard screencast input mapping.
#
# The dashboard maps a click on the screencast canvas to FRAME-pixel coordinates
# and sends them with the frame dimensions (frameW/frameH). The server must scale
# those into the live CSS viewport so the click lands on the intended element,
# regardless of the frame's pixel scale (HiDPI frames are 2x+ the CSS viewport).
# These tests exercise that mapping across multiple frame scales and target
# positions, so it stays correct on every browser provider the suite runs against.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/api.sh"

SC_TAB_ID=""

sc_navigate() {
  local url="$1"
  local body
  if [ -n "$SC_TAB_ID" ]; then
    body=$(jq -nc --arg url "$url" --arg tabId "$SC_TAB_ID" '{url:$url, tabId:$tabId}')
  else
    body=$(jq -nc --arg url "$url" '{url:$url}')
  fi
  pt_post /navigate -d "$body"
  assert_ok "navigate"
  local tab_id
  tab_id=$(echo "$RESULT" | jq -r '.tabId // empty')
  [ -n "$tab_id" ] && [ "$tab_id" != "null" ] && SC_TAB_ID="$tab_id"
}

# sc_eval <expression> → echoes the evaluated .result value
sc_eval() {
  local body
  body=$(jq -nc --arg tabId "$SC_TAB_ID" --arg expression "$1" '{tabId:$tabId, expression:$expression}')
  pt_post /evaluate "$body" >/dev/null
  echo "$RESULT" | jq -r '.result // empty'
}

# sc_click_frame <x> <y> [frameW] [frameH] — click in frame-pixel space
sc_click_frame() {
  local x="$1" y="$2" fw="${3:-}" fh="${4:-}"
  local body
  if [ -n "$fw" ]; then
    body=$(jq -nc --arg t "$SC_TAB_ID" --argjson x "$x" --argjson y "$y" --argjson fw "$fw" --argjson fh "$fh" \
      '{kind:"click", tabId:$t, x:$x, y:$y, hasXY:true, frameW:$fw, frameH:$fh}')
  else
    body=$(jq -nc --arg t "$SC_TAB_ID" --argjson x "$x" --argjson y "$y" \
      '{kind:"click", tabId:$t, x:$x, y:$y, hasXY:true}')
  fi
  pt_post /action "$body" >/dev/null
}

# sc_press <key> <modifiers> — press a key with a CDP modifier bitmask
# (Alt=1, Ctrl=2, Meta=4, Shift=8), as the dashboard sends keyboard chords.
sc_press() {
  local body
  body=$(jq -nc --arg t "$SC_TAB_ID" --arg key "$1" --argjson mods "$2" \
    '{kind:"press", tabId:$t, key:$key, modifiers:$mods}')
  pt_post /action "$body" >/dev/null
}

# ─────────────────────────────────────────────────────────────────
start_test "screencast click mapping: every target at every frame scale"

sc_navigate "${FIXTURES_URL}/screencast-click-targets.html"

VP=$(sc_eval 'JSON.stringify([Math.round(window.innerWidth), Math.round(window.innerHeight)])')
CSSW=$(echo "$VP" | jq -r '.[0]')
CSSH=$(echo "$VP" | jq -r '.[1]')
echo "    css viewport: ${CSSW}x${CSSH}"

# Frame scales to simulate: 1x (no scaling), and HiDPI/odd ratios.
SCALES="1 1.5 2 3"
TARGETS="tl tr ml c mr bl br"

for id in $TARGETS; do
  CENTER=$(sc_eval "var r=document.getElementById('${id}').getBoundingClientRect(); JSON.stringify([Math.round(r.left+r.width/2), Math.round(r.top+r.height/2)])")
  CX=$(echo "$CENTER" | jq -r '.[0]')
  CY=$(echo "$CENTER" | jq -r '.[1]')
  for s in $SCALES; do
    read -r FX FY FW FH < <(awk -v cx="$CX" -v cy="$CY" -v cw="$CSSW" -v ch="$CSSH" -v s="$s" \
      'BEGIN{printf "%.0f %.0f %.0f %.0f", cx*s, cy*s, cw*s, ch*s}')
    sc_eval 'window.__lastClick=""; "ok"' >/dev/null
    sc_click_frame "$FX" "$FY" "$FW" "$FH"
    GOT=$(sc_eval 'window.__lastClick')
    if [ "$GOT" = "$id" ]; then
      pass_assert "scale ${s}x → click frame(${FX},${FY}) landed on '${id}'"
    else
      fail_assert "scale ${s}x → click frame(${FX},${FY}) landed on '${GOT}', want '${id}'"
    fi
  done
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screencast click focuses an input field (HiDPI frame)"

sc_navigate "${FIXTURES_URL}/screencast-click-targets.html"
CSSW=$(sc_eval 'Math.round(window.innerWidth)')
CSSH=$(sc_eval 'Math.round(window.innerHeight)')
CENTER=$(sc_eval "var r=document.getElementById('inp').getBoundingClientRect(); JSON.stringify([Math.round(r.left+r.width/2), Math.round(r.top+r.height/2)])")
CX=$(echo "$CENTER" | jq -r '.[0]')
CY=$(echo "$CENTER" | jq -r '.[1]')

# Simulate a 2x HiDPI frame.
read -r FX FY FW FH < <(awk -v cx="$CX" -v cy="$CY" -v cw="$CSSW" -v ch="$CSSH" \
  'BEGIN{printf "%.0f %.0f %.0f %.0f", cx*2, cy*2, cw*2, ch*2}')
sc_eval 'document.getElementById("inp").blur(); "ok"' >/dev/null
sc_click_frame "$FX" "$FY" "$FW" "$FH"
ACTIVE=$(sc_eval 'document.activeElement.id')
if [ "$ACTIVE" = "inp" ]; then
  pass_assert "2x-frame click focused the input"
else
  fail_assert "2x-frame click left focus on '${ACTIVE}', want 'inp'"
fi

# Close the loop: the click focused the field, so typing (as the dashboard sends
# it — insertText for printable chars) must land in that field. This is the exact
# end-to-end path that was broken when clicks missed their target.
TYPED_BODY=$(jq -nc --arg t "$SC_TAB_ID" '{kind:"keyboard-inserttext", tabId:$t, text:"Hello"}')
pt_post /action "$TYPED_BODY" >/dev/null
VAL=$(sc_eval 'document.getElementById("inp").value')
if [ "$VAL" = "Hello" ]; then
  pass_assert "typing after a scaled click lands in the focused input"
else
  fail_assert "typed text landed as '${VAL}', want 'Hello'"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screencast click without frameW treats coords as CSS pixels"

sc_navigate "${FIXTURES_URL}/screencast-click-targets.html"
CENTER=$(sc_eval "var r=document.getElementById('c').getBoundingClientRect(); JSON.stringify([Math.round(r.left+r.width/2), Math.round(r.top+r.height/2)])")
CX=$(echo "$CENTER" | jq -r '.[0]')
CY=$(echo "$CENTER" | jq -r '.[1]')
sc_eval 'window.__lastClick=""; "ok"' >/dev/null
sc_click_frame "$CX" "$CY"   # no frameW/frameH → coords are already CSS px
GOT=$(sc_eval 'window.__lastClick')
if [ "$GOT" = "c" ]; then
  pass_assert "CSS-pixel click (no frameW) landed on centre target"
else
  fail_assert "CSS-pixel click landed on '${GOT}', want 'c'"
fi

end_test

# ─────────────────────────────────────────────────────────────────
start_test "screencast keyboard chords (Ctrl+A, Shift+End) reach the page"

sc_navigate "${FIXTURES_URL}/screencast-click-targets.html"
# Put known text in the input and focus it with the caret mid-string.
sc_eval 'var b=document.getElementById("inp"); b.value="hello world"; b.focus(); b.setSelectionRange(3,3); "ok"' >/dev/null

# Ctrl+A (modifiers=2) → select the whole field.
sc_press "a" 2
SEL_START=$(sc_eval 'document.getElementById("inp").selectionStart')
SEL_END=$(sc_eval 'document.getElementById("inp").selectionEnd')
if [ "$SEL_START" = "0" ] && [ "$SEL_END" = "11" ]; then
  pass_assert "Ctrl+A selected the whole field (0..11)"
else
  fail_assert "Ctrl+A selection was ${SEL_START}..${SEL_END}, want 0..11"
fi

# Shift+End (modifiers=8) from the start → extend selection to the end.
sc_eval 'document.getElementById("inp").setSelectionRange(0,0); "ok"' >/dev/null
sc_press "End" 8
SEL_START=$(sc_eval 'document.getElementById("inp").selectionStart')
SEL_END=$(sc_eval 'document.getElementById("inp").selectionEnd')
if [ "$SEL_START" = "0" ] && [ "$SEL_END" = "11" ]; then
  pass_assert "Shift+End extended the selection (0..11)"
else
  fail_assert "Shift+End selection was ${SEL_START}..${SEL_END}, want 0..11"
fi

end_test
