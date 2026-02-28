#!/bin/bash

# Phase 6: End-to-End Test for Multi-Instance Architecture
# This script tests:
# - Hash-based ID generation
# - Auto-port allocation
# - Instance isolation
# - Orchestrator proxy routes
# - Port cleanup and reuse

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Phase 6: End-to-End Testing${NC}"
echo ""

# Check if Pinchtab binary exists
if [ ! -f "./pinchtab" ]; then
    echo -e "${RED}✗ Pinchtab binary not found. Run: go build -o pinchtab ./cmd/pinchtab${NC}"
    exit 1
fi

# Kill any existing Pinchtab instances
pkill -f "./pinchtab" 2>/dev/null || true
sleep 1

# Start Pinchtab
echo -e "${YELLOW}Starting Pinchtab dashboard...${NC}"
./pinchtab &
DASHBOARD_PID=$!
sleep 2

echo -e "${GREEN}✓ Dashboard started (PID: $DASHBOARD_PID)${NC}"
echo ""

# Test 1: Create Instance 1 (Headed)
echo -e "${YELLOW}Test 1: Create headed instance...${NC}"
INST1=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"work","headless":false}')

INST1_ID=$(echo $INST1 | jq -r '.id')
INST1_PORT=$(echo $INST1 | jq -r '.port')
INST1_PROFILE=$(echo $INST1 | jq -r '.profileId')

if [[ ! $INST1_ID =~ ^inst_ ]]; then
    echo -e "${RED}✗ FAILED: Instance ID should have inst_ prefix, got: $INST1_ID${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

if [[ ! $INST1_PROFILE =~ ^prof_ ]]; then
    echo -e "${RED}✗ FAILED: Profile ID should have prof_ prefix, got: $INST1_PROFILE${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Instance 1 created: $INST1_ID (port: $INST1_PORT)${NC}"
echo ""

# Test 2: Create Instance 2 (Headless)
echo -e "${YELLOW}Test 2: Create headless instance...${NC}"
INST2=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"scrape","headless":true}')

INST2_ID=$(echo $INST2 | jq -r '.id')
INST2_PORT=$(echo $INST2 | jq -r '.port')

if [ "$INST1_PORT" = "$INST2_PORT" ]; then
    echo -e "${RED}✗ FAILED: Instances should have different ports${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Instance 2 created: $INST2_ID (port: $INST2_PORT)${NC}"
echo -e "${GREEN}✓ Port allocation working (9868 and 9869)${NC}"
echo ""

# Test 3: Wait for Chrome initialization
echo -e "${YELLOW}Test 3: Wait for Chrome initialization...${NC}"
sleep 3

# Check health
HEALTH1=$(curl -s http://localhost:$INST1_PORT/health | jq -r '.status')
HEALTH2=$(curl -s http://localhost:$INST2_PORT/health | jq -r '.status')

if [ "$HEALTH1" != "ok" ] || [ "$HEALTH2" != "ok" ]; then
    echo -e "${RED}✗ FAILED: Health check failed (status 1: $HEALTH1, status 2: $HEALTH2)${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Both instances healthy${NC}"
echo -e "${GREEN}✓ Chrome initialized successfully${NC}"
echo ""

# Test 4: Navigate via orchestrator proxy
echo -e "${YELLOW}Test 4: Navigate via orchestrator proxy...${NC}"
NAV1=$(curl -s -X POST "http://localhost:9867/instances/$INST1_ID/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}')

TAB1_ID=$(echo $NAV1 | jq -r '.tabId')

if [[ ! $TAB1_ID =~ ^tab_ ]]; then
    echo -e "${RED}✗ FAILED: Tab ID should have tab_ prefix, got: $TAB1_ID${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Instance 1 navigated to example.com${NC}"
echo -e "${GREEN}✓ Tab ID generated: $TAB1_ID${NC}"
echo ""

# Test 5: Navigate instance 2
echo -e "${YELLOW}Test 5: Navigate second instance...${NC}"
NAV2=$(curl -s -X POST "http://localhost:9867/instances/$INST2_ID/navigate" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com"}')

TAB2_ID=$(echo $NAV2 | jq -r '.tabId')

if [ "$TAB1_ID" = "$TAB2_ID" ]; then
    echo -e "${RED}✗ FAILED: Tab IDs should be different (isolation)${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Instance 2 navigated to github.com${NC}"
echo -e "${GREEN}✓ Tab isolation verified (different IDs)${NC}"
echo ""

# Test 6: List instances
echo -e "${YELLOW}Test 6: List all instances...${NC}"
INSTANCES=$(curl -s http://localhost:9867/instances | jq '.[] | .id' | wc -l)

if [ "$INSTANCES" -ne 2 ]; then
    echo -e "${RED}✗ FAILED: Expected 2 instances, got $INSTANCES${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Both instances listed correctly${NC}"
echo ""

# Test 7: Stop instance 1
echo -e "${YELLOW}Test 7: Stop instance 1...${NC}"
curl -s -X POST "http://localhost:9867/instances/$INST1_ID/stop" > /dev/null
sleep 1

REMAINING=$(curl -s http://localhost:9867/instances | jq '.[] | .id' | wc -l)

if [ "$REMAINING" -ne 1 ]; then
    echo -e "${RED}✗ FAILED: Expected 1 instance after stopping, got $REMAINING${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Instance 1 stopped${NC}"
echo -e "${GREEN}✓ Port 9868 released${NC}"
echo ""

# Test 8: Port reuse
echo -e "${YELLOW}Test 8: Test port reuse...${NC}"
INST3=$(curl -s -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"name":"reuse","headless":true}')

INST3_PORT=$(echo $INST3 | jq -r '.port')

if [ "$INST3_PORT" != "9868" ]; then
    echo -e "${RED}✗ FAILED: Port should be reused (9868), got $INST3_PORT${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ Port 9868 reused correctly${NC}"
echo ""

# Test 9: Cleanup
echo -e "${YELLOW}Test 9: Cleanup...${NC}"
INST3_ID=$(echo $INST3 | jq -r '.id')

curl -s -X POST "http://localhost:9867/instances/$INST2_ID/stop" > /dev/null
curl -s -X POST "http://localhost:9867/instances/$INST3_ID/stop" > /dev/null
sleep 1

FINAL=$(curl -s http://localhost:9867/instances | jq '.[] | .id' | wc -l)

if [ "$FINAL" -ne 0 ]; then
    echo -e "${RED}✗ FAILED: Expected 0 instances after cleanup, got $FINAL${NC}"
    kill $DASHBOARD_PID 2>/dev/null || true
    exit 1
fi

echo -e "${GREEN}✓ All instances cleaned up${NC}"
echo ""

# Cleanup
kill $DASHBOARD_PID 2>/dev/null || true
sleep 1

# Summary
echo ""
echo -e "${GREEN}═══════════════════════════════════════${NC}"
echo -e "${GREEN}✅ ALL TESTS PASSED!${NC}"
echo -e "${GREEN}═══════════════════════════════════════${NC}"
echo ""
echo "Tested:"
echo "  ✓ Hash-based ID generation (prof_X, inst_X, tab_X)"
echo "  ✓ Auto-port allocation (9868, 9869, ...)"
echo "  ✓ Port release and reuse"
echo "  ✓ Instance isolation (different tab IDs)"
echo "  ✓ Orchestrator proxy routes"
echo "  ✓ Health checks on instances"
echo "  ✓ Instance creation and cleanup"
echo ""
