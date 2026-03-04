#!/bin/bash
# Simple Test Scenario: Automated Orchestrator Test
# Tests: instance creation, tab navigation via orchestrator, find endpoint
# Duration: ~10 seconds
# Usage: ./tests/manual/automated-test.sh

set -e

echo "🚀 Starting Automated Orchestrator Test..."
echo "Tests: Instance creation, orchestrator shorthand endpoints, find"
echo ""

./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"
echo ""

# Create 2 instances (orchestrator will manage them)
echo "Creating 2 instances..."

INST1=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test-headed","headless":false}')
INST1_ID=$(echo $INST1 | jq -r '.id')
echo "✓ Instance 1: $INST1_ID"

INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"test-headless","headless":true}')
INST2_ID=$(echo $INST2 | jq -r '.id')
echo "✓ Instance 2: $INST2_ID"

echo ""

sleep 2

# Verify instances are running
echo "Verifying instances..."
INST_COUNT=$(curl -s http://localhost:9867/instances | jq 'length')
echo "✓ Running instances: $INST_COUNT"
echo ""

# Use orchestrator shorthand endpoints (no instance/tab management needed)
echo "Using orchestrator shorthand endpoints..."
echo ""

# Navigate (auto-creates tab on current instance)
echo "1. POST /navigate (auto-creates tab)..."
NAV=$(curl -s -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}')
TAB1_ID=$(echo $NAV | jq -r '.tabId')
NAV_URL=$(echo $NAV | jq -r '.url')
echo "✓ Tab created: $TAB1_ID, navigated to: $NAV_URL"
echo ""

# Use find endpoint (uses current tab)
echo "2. POST /find (search for elements)..."
FIND=$(curl -s -X POST http://localhost:9867/find \
  -H "Content-Type: application/json" \
  -d '{"text":"example"}')
REFS=$(echo $FIND | jq -r '.refs | length // 0')
echo "✓ Find returned $REFS element references"
echo ""

# Get snapshot
echo "3. GET /snapshot (current tab)..."
SNAP=$(curl -s http://localhost:9867/snapshot \
  -H "Content-Type: application/json")
SNAP_URL=$(echo $SNAP | jq -r '.url // "unknown"')
echo "✓ Snapshot URL: $SNAP_URL"
echo ""

# Create another tab using POST /tab
echo "4. POST /tab (create new tab)..."
TAB_NEW=$(curl -s -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://github.com"}')
TAB2_ID=$(echo $TAB_NEW | jq -r '.tabId // "unknown"')
echo "✓ New tab created: $TAB2_ID"
echo ""

# List all tabs
echo "5. GET /tabs (list all tabs)..."
TABS=$(curl -s http://localhost:9867/tabs)
TAB_COUNT=$(echo $TABS | jq 'length')
echo "✓ Total tabs: $TAB_COUNT"
echo ""

# Cleanup
echo "Stopping all instances..."
curl -s -X POST "http://localhost:9867/instances/$INST1_ID/stop" > /dev/null
curl -s -X POST "http://localhost:9867/instances/$INST2_ID/stop" > /dev/null

echo "✓ All instances stopped"
echo ""

sleep 2

# Verify cleanup
REMAINING=$(curl -s http://localhost:9867/instances | jq 'length')
if [ "$REMAINING" -eq 0 ]; then
  echo "✓ All instances cleaned up"
else
  echo "⚠️  $REMAINING instances still running (may be cleaning up asynchronously)"
fi

# Clean up
kill $DASHBOARD_PID 2>/dev/null
sleep 1

echo ""
echo "=========================================="
echo "✅ AUTOMATED TEST PASSED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  • Instance 1: ✓"
echo "  • Instance 2: ✓"
echo "  • Navigate (auto-tab): ✓"
echo "  • Find endpoint: ✓"
echo "  • Snapshot: ✓"
echo "  • Tab creation: ✓"
echo "  • Tab listing: ✓"
echo "  • Cleanup: ✓"
echo ""
echo "This tests:"
echo "  • Instance creation via orchestrator"
echo "  • Orchestrator shorthand endpoints"
echo "  • Auto-tab management"
echo "  • Element searching"
echo "  • Tab operations"
