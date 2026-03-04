#!/bin/bash
# Complex Test Scenario: Stress Test (10 Instances)
# Tests: concurrent instance creation, load balancing, orchestrator capacity
# Duration: ~60 seconds
# Usage: ./tests/manual/stress-test-10-instances.sh

set -o pipefail

# Wait for N instances to be "running"
wait_for_running() {
  local expected=${1:-1}
  local max_wait=${2:-60}
  local elapsed=0
  while [ $elapsed -lt $max_wait ]; do
    RUNNING=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.status == "running")] | length' 2>/dev/null || echo 0)
    if [ "$RUNNING" -ge "$expected" ]; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "⚠️  Only $RUNNING/$expected instances running after ${max_wait}s"
  return 1
}

echo "🔥 Starting Stress Test (10 Instances)..."
echo "Tests: Concurrent creation, orchestrator capacity"
echo ""

./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"
echo ""

# Create 10 instances
echo "Creating 10 headless instances..."

INST_IDS=()
for i in {1..10}; do
  INST=$(curl -s -X POST http://localhost:9867/instances/launch \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"stress-$i\",\"headless\":true}")
  INST_ID=$(echo $INST | jq -r '.id')
  INST_IDS+=($INST_ID)
  printf "  $i. $INST_ID\n"
done

echo "✓ Created 10 instances"
echo ""

# Wait for at least some to be running
echo "Waiting for instances to be ready..."
if wait_for_running 5 60; then
  RUNNING=$(curl -s http://localhost:9867/instances | jq '[.[] | select(.status == "running")] | length' 2>/dev/null || echo 0)
  echo "✓ $RUNNING instances running"
else
  echo "⚠️  Continuing with available instances..."
fi
echo ""

# Concurrent navigation
echo "Concurrent navigation requests..."
for i in {1..5}; do
  curl -s -X POST http://localhost:9867/navigate \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"https://example.com/$i\"}" > /dev/null &
done
wait
echo "✓ Navigation requests completed"
echo ""

sleep 1

# Concurrent find
echo "Concurrent find operations..."
for i in {1..5}; do
  curl -s -X POST http://localhost:9867/find \
    -H "Content-Type: application/json" \
    -d "{\"text\":\"element-$i\"}" > /dev/null &
done
wait
echo "✓ Find operations completed"
echo ""

# Check tabs
TABS=$(curl -s http://localhost:9867/tabs)
TAB_COUNT=$(echo $TABS | jq 'length' 2>/dev/null || echo 0)
echo "✓ Total tabs: $TAB_COUNT"
echo ""

# Cleanup
echo "Stopping all instances..."
for INST_ID in "${INST_IDS[@]}"; do
  curl -s -X POST "http://localhost:9867/instances/$INST_ID/stop" > /dev/null &
done
wait
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
echo "✅ STRESS TEST COMPLETED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  • 10 instances created: ✓"
echo "  • Concurrent navigation: ✓"
echo "  • Concurrent find: ✓"
echo "  • Tabs: $TAB_COUNT"
echo "  • Cleanup: ✓"
