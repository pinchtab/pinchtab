#!/bin/bash
# run-recent.sh - Run only recently added/changed E2E test scenarios
# This runs a fast subset for fail-fast CI before the full suites.

set -uo pipefail

SCRIPT_DIR="$(dirname "$0")"
COMMON_DIR="$(dirname "$SCRIPT_DIR")/scenarios"
source "${COMMON_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}PinchTab E2E Recent Tests${NC}"
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

# Recent test files — add new scenarios here for fast CI feedback.
# Graduate stable coverage into api-fast/cli-fast or full-extended once the
# feature settles, so "recent" stays intentionally small.
# All test files in this directory run as the recent suite.
for script in "${SCRIPT_DIR}"/[0-9][0-9]-*.sh; do
  if [ -f "$script" ]; then
    echo -e "${YELLOW}Running: $(basename "$script")${NC}"
    echo ""
    source "$script"
    echo ""
  fi
done

print_summary

if [ -d "${RESULTS_DIR:-}" ]; then
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary.txt"
fi
