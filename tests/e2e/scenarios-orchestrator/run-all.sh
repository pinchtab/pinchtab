#!/bin/bash
# run-all.sh - Run orchestrator-focused E2E scenarios

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")/scenarios"
source "${COMMON_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}PinchTab E2E Orchestrator Tests${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "E2E_SERVER: ${E2E_SERVER}"
echo "E2E_BRIDGE_URL: ${E2E_BRIDGE_URL:-}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Waiting for orchestrator services to become ready..."
wait_for_instance_ready "${E2E_SERVER}"
if [ -n "${E2E_BRIDGE_URL:-}" ]; then
  wait_for_instance_ready "${E2E_BRIDGE_URL}" 60 "${E2E_BRIDGE_TOKEN:-}"
fi
echo ""

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
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary-orchestrator.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary-orchestrator.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary-orchestrator.txt"
fi
