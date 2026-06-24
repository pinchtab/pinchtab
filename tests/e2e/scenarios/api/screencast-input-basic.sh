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
    read -r FX FY FW FH < <(python3 -c "cx,cy,cw,ch,s=$CX,$CY,$CSSW,$CSSH,$s; print(round(cx*s), round(cy*s), round(cw*s), round(ch*s))")
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
read -r FX FY FW FH < <(python3 -c "cx,cy,cw,ch=$CX,$CY,$CSSW,$CSSH; print(cx*2, cy*2, cw*2, ch*2)")
sc_eval 'document.getElementById("inp").blur(); "ok"' >/dev/null
sc_click_frame "$FX" "$FY" "$FW" "$FH"
ACTIVE=$(sc_eval 'document.activeElement.id')
if [ "$ACTIVE" = "inp" ]; then
  pass_assert "2x-frame click focused the input"
else
  fail_assert "2x-frame click left focus on '${ACTIVE}', want 'inp'"
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
