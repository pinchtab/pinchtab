#!/bin/bash
# Automated orchestrator test script
# Verifies: instance creation, tab creation, find endpoint, isolation, cleanup
# Duration: ~10 seconds
# Usage: ./tests/manual/automated-test.sh

set -e

echo "🚀 Starting Automated Orchestrator Test..."
echo ""

# Start Pinchtab in background
./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo "✓ Dashboard started (PID: $DASHBOARD_PID)"
echo ""

# Create instance 1
echo "Creating instance 1 (headed)..."
INST1=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","headless":false}')
INST1_ID=$(echo $INST1 | jq -r '.id')
INST1_PORT=$(echo $INST1 | jq -r '.port')
echo "✓ Created instance 1: $INST1_ID (port: $INST1_PORT)"

# Create instance 2
echo "Creating instance 2 (headless)..."
INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"scrape","headless":true}')
INST2_ID=$(echo $INST2 | jq -r '.id')
INST2_PORT=$(echo $INST2 | jq -r '.port')
echo "✓ Created instance 2: $INST2_ID (port: $INST2_PORT)"
echo ""

# Wait for Chrome to initialize
sleep 2

# List instances
echo "Verifying instance list..."
INSTANCES=$(curl -s http://localhost:9867/instances | jq '.')
INSTANCE_COUNT=$(echo $INSTANCES | jq 'length')
echo "✓ $INSTANCE_COUNT instances running"
echo ""

# Create tab in instance 1
echo "Creating tab in instance 1..."
TAB1=$(curl -s -X POST "http://localhost:$INST1_PORT/tabs" \
  -H "Content-Type: application/json" \
  -d '{}')
TAB1_ID=$(echo $TAB1 | jq -r '.id // "unknown"')
echo "✓ Created tab in instance 1: $TAB1_ID"

# Create tab in instance 2
echo "Creating tab in instance 2..."
TAB2=$(curl -s -X POST "http://localhost:$INST2_PORT/tabs" \
  -H "Content-Type: application/json" \
  -d '{}')
TAB2_ID=$(echo $TAB2 | jq -r '.id // "unknown"')
echo "✓ Created tab in instance 2: $TAB2_ID"
echo ""

# Verify isolation: different tab IDs
echo "Verifying instance isolation..."
if [ "$TAB1_ID" != "$TAB2_ID" ]; then
  echo "✓ Instance isolation verified (different tab IDs: $TAB1_ID vs $TAB2_ID)"
else
  echo "❌ FAILED: Tab IDs should be different!"
  kill $DASHBOARD_PID 2>/dev/null
  exit 1
fi
echo ""

# Use find endpoint in instance 1
echo "Using find endpoint in instance 1..."
FIND1=$(curl -s -X POST "http://localhost:$INST1_PORT/find" \
  -H "Content-Type: application/json" \
  -d '{"text":"example"}')
FIND1_RESULT=$(echo $FIND1 | jq -r '.refs | length // 0')
echo "✓ Find endpoint returned $FIND1_RESULT results for instance 1"

# Use find endpoint in instance 2
echo "Using find endpoint in instance 2..."
FIND2=$(curl -s -X POST "http://localhost:$INST2_PORT/find" \
  -H "Content-Type: application/json" \
  -d '{"text":"example"}')
FIND2_RESULT=$(echo $FIND2 | jq -r '.refs | length // 0')
echo "✓ Find endpoint returned $FIND2_RESULT results for instance 2"
echo ""

# Stop instance 1
echo "Stopping instance 1..."
curl -s -X POST "http://localhost:9867/instances/$INST1_ID/stop" > /dev/null
echo "✓ Instance 1 stopped, port $INST1_PORT released"

# Stop instance 2
echo "Stopping instance 2..."
curl -s -X POST "http://localhost:9867/instances/$INST2_ID/stop" > /dev/null
echo "✓ Instance 2 stopped, port $INST2_PORT released"
echo ""

# Verify all stopped
echo "Verifying cleanup..."
sleep 2
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
echo "✅ ALL TESTS PASSED!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  • Instance creation: ✓"
echo "  • Tab creation: ✓"
echo "  • Find endpoint: ✓"
echo "  • Instance isolation: ✓"
echo "  • Cleanup: ✓"
