#!/bin/bash
# run-stealth.sh — Run stealth level e2e tests
#
# Tests all three stealth levels (light, medium, full) and verifies
# each provides the expected bot detection evasion features.
#
# Prerequisites:
#   docker compose -f ../docker-compose-stealth.yml up -d --build
#
# Or run against existing servers:
#   LIGHT_SERVER=http://localhost:9901 \
#   MEDIUM_SERVER=http://localhost:9902 \
#   FULL_SERVER=http://localhost:9903 \
#   ./run-stealth.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FIXTURES_URL="${FIXTURES_URL:-http://localhost:8080}"

LIGHT_SERVER="${LIGHT_SERVER:-http://localhost:9901}"
MEDIUM_SERVER="${MEDIUM_SERVER:-http://localhost:9902}"
FULL_SERVER="${FULL_SERVER:-http://localhost:9903}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}STEALTH LEVEL E2E TESTS${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Servers:"
echo "  Light:  ${LIGHT_SERVER}"
echo "  Medium: ${MEDIUM_SERVER}"
echo "  Full:   ${FULL_SERVER}"
echo ""

FAILED=0

# Test light level
echo -e "${BLUE}▶ Testing LIGHT stealth level${NC}"
if STEALTH_LEVEL=light E2E_SERVER="${LIGHT_SERVER}" FIXTURES_URL="${FIXTURES_URL}" \
   "${SCRIPT_DIR}/51-stealth-levels.sh"; then
  echo -e "${GREEN}✓ Light level passed${NC}\n"
else
  echo -e "${RED}✗ Light level failed${NC}\n"
  FAILED=1
fi

# Test medium level
echo -e "${BLUE}▶ Testing MEDIUM stealth level${NC}"
if STEALTH_LEVEL=medium E2E_SERVER="${MEDIUM_SERVER}" FIXTURES_URL="${FIXTURES_URL}" \
   "${SCRIPT_DIR}/51-stealth-levels.sh"; then
  echo -e "${GREEN}✓ Medium level passed${NC}\n"
else
  echo -e "${RED}✗ Medium level failed${NC}\n"
  FAILED=1
fi

# Test full level
echo -e "${BLUE}▶ Testing FULL stealth level${NC}"
if STEALTH_LEVEL=full E2E_SERVER="${FULL_SERVER}" FIXTURES_URL="${FIXTURES_URL}" \
   "${SCRIPT_DIR}/51-stealth-levels.sh"; then
  echo -e "${GREEN}✓ Full level passed${NC}\n"
else
  echo -e "${RED}✗ Full level failed${NC}\n"
  FAILED=1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ "$FAILED" -eq 0 ]; then
  echo -e "${GREEN}All stealth level tests passed${NC}"
else
  echo -e "${RED}Some stealth level tests failed${NC}"
  exit 1
fi
