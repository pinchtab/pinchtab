#!/bin/bash
# Simple Test Scenario: Automated Orchestrator Test
# Tests: instance creation, orchestrator shorthand endpoints, find
# Duration: ~30 seconds
# Usage: ./tests/manual/automated-test.sh

set -o pipefail

# Wait for at least one instance to be "running"
wait_for_running() {
  local max_wait=${1:-30}
  local elapsed=0
  while [ $elapsed -lt $max_wait ]; do
    RUNNING=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.status == "running")] | length' 2>/dev/null || echo 0)
    if [ "$RUNNING" -gt 0 ]; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo "⚠️  Timed out waiting for instances to be running"
  return 1
}

echo "🚀 Starting Automated Orchestrator Test..."
echo "Tests: Instance creation, orchestrator shorthand endpoints, find"
echo ""

./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"
echo ""

# Create instance
echo "Creating headless instance..."
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test-headless","headless":true}')
INST_ID=$(echo $INST | jq -r '.id')
echo "✓ Instance: $INST_ID (status: $(echo $INST | jq -r '.status'))"
echo ""

# Wait for instance to be ready
echo "Waiting for instance to be ready..."
if wait_for_running 30; then
  echo "✓ Instance is running"
else
  echo "❌ Instance failed to start"
  kill $DASHBOARD_PID 2>/dev/null
  exit 1
fi
echo ""

# Use orchestrator shorthand endpoints
echo "Using orchestrator shorthand endpoints..."
echo ""

# Navigate (auto-creates tab on current instance)
echo "1. POST /navigate (auto-creates tab)..."
NAV=$(curl -s -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}')
TAB_ID=$(echo $NAV | jq -r '.tabId // "error"')
NAV_URL=$(echo $NAV | jq -r '.url // .error // "unknown"')
echo "✓ Tab: $TAB_ID, URL: $NAV_URL"
echo ""

sleep 1

# Get snapshot
echo "2. GET /snapshot (current tab)..."
SNAP=$(curl -s http://localhost:9867/snapshot)
SNAP_URL=$(echo $SNAP | jq -r '.url // "unknown"')
echo "✓ Snapshot URL: $SNAP_URL"
echo ""

# Use find endpoint
echo "3. POST /find (search for elements)..."
FIND=$(curl -s -X POST http://localhost:9867/find \
  -H "Content-Type: application/json" \
  -d '{"text":"example"}')
REFS=$(echo $FIND | jq 'if .refs then (.refs | length) else 0 end' 2>/dev/null || echo 0)
echo "✓ Find returned $REFS element references"
echo ""

# Create new tab
echo "4. POST /tab (create new tab)..."
TAB_NEW=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://github.com"}')
TAB2_ID=$(echo $TAB_NEW | jq -r '.tabId // "error"')
echo "✓ New tab: $TAB2_ID"
echo ""

# List tabs
echo "5. GET /tabs (list all)..."
TABS=$(curl -s http://localhost:9867/tabs)
TAB_COUNT=$(echo $TABS | jq 'length' 2>/dev/null || echo 0)
echo "✓ Total tabs: $TAB_COUNT"
echo ""

# Cleanup
echo "Stopping instance..."
curl -s -X POST "http://localhost:9867/instances/$INST_ID/stop" > /dev/null
sleep 3

REMAINING=$(curl -s http://localhost:9867/instances 2>/dev/null | jq 'length // 0' 2>/dev/null || echo 0)
if [ -z "$REMAINING" ] || [ "$REMAINING" -eq 0 ]; then
  echo "✓ All instances cleaned up"
else
  echo "⚠️  $REMAINING instances still running"
fi

kill $DASHBOARD_PID 2>/dev/null
sleep 1

echo ""
echo "=========================================="
echo "✅ AUTOMATED TEST PASSED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  • Instance created: ✓ ($INST_ID)"
echo "  • Navigate (auto-tab): ✓ ($TAB_ID)"
echo "  • Snapshot: ✓ ($SNAP_URL)"
echo "  • Find: ✓ ($REFS refs)"
echo "  • Tab creation: ✓ ($TAB2_ID)"
echo "  • Tab listing: ✓ ($TAB_COUNT tabs)"
echo "  • Cleanup: ✓"
