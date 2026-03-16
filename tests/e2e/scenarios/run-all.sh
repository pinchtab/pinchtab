#!/bin/bash
# run-all.sh - Run all E2E test scenarios

set -uo pipefail

SCRIPT_DIR="$(dirname "$0")"
source "${SCRIPT_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}PinchTab E2E Test Suite${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "E2E_SERVER: ${E2E_SERVER}"
echo "FIXTURES_URL: ${FIXTURES_URL}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Waiting for instances to become ready..."
wait_for_instance_ready "${E2E_SERVER}"
wait_for_instance_ready "${E2E_SECURE_SERVER}"
if [ -n "${E2E_LITE_SERVER:-}" ]; then
  wait_for_instance_ready "${E2E_LITE_SERVER}"
fi
echo ""

# Find and run all test scripts in order
for script in "${SCRIPT_DIR}"/[0-9][0-9]-*.sh; do
  if [ -f "$script" ]; then
    echo -e "${YELLOW}Running: $(basename "$script")${NC}"
    echo ""
    source "$script"
    echo ""
  fi
done

print_summary

# Save results if results dir exists
if [ -d "${RESULTS_DIR:-}" ]; then
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary.txt"
fi
