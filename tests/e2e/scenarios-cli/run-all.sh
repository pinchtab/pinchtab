#!/bin/bash
# Run all CLI E2E test scenarios

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Source common utilities (initializes counters)
source "$SCRIPT_DIR/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🦀 PinchTab CLI E2E Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Server: $E2E_SERVER"
echo "  Fixtures: $FIXTURES_URL"
echo ""

# Wait for instance to be ready (same as curl-based tests)
wait_for_instance_ready "$E2E_SERVER"

# Verify pinchtab CLI is available
if ! command -v pinchtab &> /dev/null; then
  echo "ERROR: pinchtab CLI not found in PATH"
  exit 1
fi

echo ""
echo "Running CLI tests..."
echo ""

# Find and run all test scripts in order
for script in "$SCRIPT_DIR"/[0-9][0-9]-*.sh; do
  if [ -f "$script" ]; then
    echo -e "${YELLOW}Running: $(basename "$script")${NC}"
    echo ""
    source "$script"
    echo ""
  fi
done

print_summary

if [ -d "${RESULTS_DIR:-}" ]; then
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary-cli-full.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary-cli-full.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary-cli-full.txt"
fi
