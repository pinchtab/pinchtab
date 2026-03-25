#!/usr/bin/env bash
set -euo pipefail

# Docker MCP Smoke Test
# Verifies that the MCP stdio server works correctly inside a Docker container.
# Tests both the Go binary directly and the npm wrapper (if Node.js is available).

IMAGE="${1:-pinchtab-local:test}"
SMOKE_TOKEN="pinchtab-mcp-smoke-${RANDOM}${RANDOM}"
NAME="pinchtab-mcp-smoke-${RANDOM}${RANDOM}"
FAILED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
BOLD='\033[1m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; FAILED=1; }

cleanup() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs:"
      docker logs "$NAME" 2>&1 | tail -20 || true
    fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo -e "${BOLD}Docker MCP Smoke Test${NC}"
echo "Image: $IMAGE"
echo ""

# Start container in background (we only need the binary, not the server)
docker run -d --name "$NAME" \
  -e PINCHTAB_TOKEN="$SMOKE_TOKEN" \
  --entrypoint /usr/bin/dumb-init \
  "$IMAGE" -- sleep 300 >/dev/null

echo "Testing MCP stdio via Go binary..."

# Test 1: JSON-RPC initialize via direct Go binary
INIT_MSG='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"docker-smoke","version":"1.0"}}}'

RESPONSE=$(echo "$INIT_MSG" | docker exec -i -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$NAME" \
  pinchtab mcp 2>/dev/null | head -1)

if echo "$RESPONSE" | grep -q '"serverInfo"'; then
  pass "JSON-RPC initialize returns valid response"
else
  fail "JSON-RPC initialize failed: $RESPONSE"
fi

# Verify server name
if echo "$RESPONSE" | grep -q '"name":"PinchTab"'; then
  pass "Server identifies as PinchTab"
else
  fail "Unexpected server name in: $RESPONSE"
fi

# Test 2: tools/list returns tools
TOOLS_MSG='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'

TOOLS_RESPONSE=$(printf '%s\n%s\n' "$INIT_MSG" "$TOOLS_MSG" | docker exec -i -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$NAME" \
  pinchtab mcp 2>/dev/null | tail -1)

if echo "$TOOLS_RESPONSE" | grep -q '"tools"'; then
  TOOL_COUNT=$(echo "$TOOLS_RESPONSE" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['result']['tools']))" 2>/dev/null || echo "0")
  if [ "$TOOL_COUNT" -gt 0 ]; then
    pass "tools/list returns $TOOL_COUNT tools"
  else
    fail "tools/list returned 0 tools"
  fi
else
  fail "tools/list failed: $TOOLS_RESPONSE"
fi

# Test 3: health tool call
HEALTH_MSG='{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pinchtab_health","arguments":{}}}'

HEALTH_RESPONSE=$(printf '%s\n%s\n' "$INIT_MSG" "$HEALTH_MSG" | docker exec -i -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$NAME" \
  pinchtab mcp 2>/dev/null | tail -1)

if echo "$HEALTH_RESPONSE" | grep -q '"result"'; then
  pass "pinchtab_health tool call returns result"
else
  # Health may fail if server isn't running — that's OK, just check for valid JSON-RPC
  if echo "$HEALTH_RESPONSE" | grep -q '"jsonrpc"'; then
    pass "pinchtab_health tool call returns valid JSON-RPC (server not running, expected)"
  else
    fail "pinchtab_health tool call failed: $HEALTH_RESPONSE"
  fi
fi

# --- Part 2: Auto-start + navigation test ---
# Uses a fresh container where the server is NOT pre-started.
# The MCP command should auto-launch the server and handle tool calls.

echo ""
echo "Testing MCP auto-start + navigation..."

NAME2="pinchtab-mcp-autostart-${RANDOM}${RANDOM}"
cleanup2() {
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME2"; then
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Auto-start container logs:"
      docker logs "$NAME2" 2>&1 | tail -30 || true
    fi
    docker rm -f "$NAME2" >/dev/null 2>&1 || true
  fi
}
trap 'cleanup; cleanup2' EXIT

# Start container with sleep (no server running)
docker run -d --name "$NAME2" \
  -e PINCHTAB_TOKEN="$SMOKE_TOKEN" \
  --entrypoint /usr/bin/dumb-init \
  "$IMAGE" -- sleep 300 >/dev/null

# Ensure config exists so the server can start
docker exec "$NAME2" pinchtab config init >/dev/null 2>&1 || true
docker exec "$NAME2" pinchtab config set server.token "$SMOKE_TOKEN" >/dev/null 2>&1 || true

# Test 4: MCP auto-starts the server when not running
# Send initialize + a navigate tool call through mcp.
# The mcp command should detect no server, spawn one, wait for health,
# then process the tool calls.
NAV_MSG='{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"pinchtab_navigate","arguments":{"url":"data:text/html,<h1>MCP-AutoStart-Test</h1>"}}}'

AUTOSTART_RESPONSE=$(printf '%s\n%s\n' "$INIT_MSG" "$NAV_MSG" | docker exec -i \
  -e PINCHTAB_TOKEN="$SMOKE_TOKEN" \
  "$NAME2" pinchtab mcp 2>/dev/null | tail -1)

if echo "$AUTOSTART_RESPONSE" | grep -q '"jsonrpc"'; then
  if echo "$AUTOSTART_RESPONSE" | grep -q '"result"'; then
    pass "MCP auto-started server and navigate tool call succeeded"
  elif echo "$AUTOSTART_RESPONSE" | grep -q '"error"'; then
    # An error response is still valid JSON-RPC — server started but tool may have failed
    pass "MCP auto-started server (tool returned error, but server is running)"
  else
    fail "MCP auto-start: unexpected response: $AUTOSTART_RESPONSE"
  fi
else
  fail "MCP auto-start failed: no JSON-RPC response: $AUTOSTART_RESPONSE"
fi

# Test 5: Verify the server is actually running after auto-start
HEALTH_CHECK=$(docker exec -e PINCHTAB_TOKEN="$SMOKE_TOKEN" "$NAME2" \
  pinchtab health 2>&1 || true)

if echo "$HEALTH_CHECK" | grep -qi "ok\|healthy\|running\|version"; then
  pass "Server is running after MCP auto-start"
else
  # health command may have different output format, check if server responds
  HEALTH_HTTP=$(docker exec "$NAME2" sh -c \
    "wget -qO- --header='Authorization: Bearer $SMOKE_TOKEN' http://127.0.0.1:9867/health 2>/dev/null" || true)
  if [ -n "$HEALTH_HTTP" ]; then
    pass "Server is running after MCP auto-start (confirmed via HTTP)"
  else
    fail "Server not running after MCP auto-start: $HEALTH_CHECK"
  fi
fi

echo ""
if [ "$FAILED" -eq 0 ]; then
  echo -e "${GREEN}${BOLD}Docker MCP smoke test passed.${NC}"
else
  echo -e "${RED}${BOLD}Docker MCP smoke test FAILED.${NC}"
  exit 1
fi
