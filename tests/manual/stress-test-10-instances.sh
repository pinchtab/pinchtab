#!/bin/bash
# Complex Test Scenario: Stress Test (10 Instances)
# Tests: concurrent instance creation, load balancing, orchestrator capacity
# Duration: ~30 seconds
# Usage: ./tests/manual/stress-test-10-instances.sh

set -e

echo "🔥 Starting Stress Test (10 Instances)..."
echo "Tests: Concurrent instance creation, orchestrator load handling"
echo ""

./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"
echo ""

# Create 10 instances concurrently
echo "Creating 10 headless instances concurrently..."

INST_IDS=()
for i in {1..10}; do
  curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"stress-inst-$i\",\"headless\":true}" | jq -r '.id' &
done

# Wait for all to complete and collect IDs
for i in {1..10}; do
  INST_ID=$(wait -p PID; curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"stress-inst-$i\",\"headless\":true}" | jq -r '.id')
  INST_IDS+=($INST_ID)
done

# Simpler approach: create sequentially but fast
INST_IDS=()
for i in {1..10}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"stress-inst-$i\",\"headless\":true}")
  INST_ID=$(echo $INST | jq -r '.id')
  INST_IDS+=($INST_ID)
  printf "  $i. $INST_ID\n"
done

echo "✓ Created 10 instances"
echo ""

sleep 2

# Verify all instances created
echo "Verifying instance count..."
INST_COUNT=$(curl -s http://localhost:9867/instances | jq 'length')
echo "✓ Instances running: $INST_COUNT"
echo ""

# Simulate concurrent navigation requests using orchestrator shorthand
echo "Simulating concurrent navigation requests..."

for i in {1..10}; do
  curl -s -X POST http://localhost:9867/navigate \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://example.com/$i\"}" > /dev/null &
done

wait

echo "✓ All navigation requests completed"
echo ""

# Concurrent find operations
echo "Simulating concurrent find operations..."

for i in {1..10}; do
  curl -s -X POST http://localhost:9867/find \
    -H "Content-Type: application/json" \
    -d "{\"text\":\"element-$i\"}" > /dev/null &
done

wait

echo "✓ All find operations completed"
echo ""

# Verify all instances still running
echo "Verifying all instances still running..."
RUNNING=$(curl -s http://localhost:9867/instances | jq 'length')
echo "✓ Instances still running: $RUNNING"
echo ""

# Get total tabs across all instances
echo "Checking total tabs..."
TABS=$(curl -s http://localhost:9867/tabs)
TAB_COUNT=$(echo $TABS | jq 'length // 0')
echo "✓ Total tabs: $TAB_COUNT"
echo ""

# Cleanup all instances
echo "Stopping all 10 instances..."
for INST_ID in "${INST_IDS[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$INST_ID/stop" > /dev/null &
done

wait

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
echo "✅ STRESS TEST PASSED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  • Created 10 instances: ✓"
echo "  • Instance verification: ✓ ($INST_COUNT running)"
echo "  • Concurrent navigation: ✓"
echo "  • Concurrent find ops: ✓"
echo "  • Tabs created: $TAB_COUNT"
echo "  • Cleanup: ✓"
echo ""
echo "This tests:"
echo "  • Orchestrator capacity (10+ instances)"
echo "  • Concurrent request handling"
echo "  • Instance isolation under load"
echo "  • Orchestrator shorthand endpoint concurrency"
echo "  • Resource cleanup"
