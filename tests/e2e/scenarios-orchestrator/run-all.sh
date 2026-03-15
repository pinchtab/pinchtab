#!/bin/bash
# run-all.sh - Run orchestrator-focused E2E scenarios

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_DIR="$(dirname "$SCRIPT_DIR")/scenarios"
source "${COMMON_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}PinchTab E2E Orchestrator Tests${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "PINCHTAB_URL: ${PINCHTAB_URL}"
echo "PINCHTAB_BRIDGE_URL: ${PINCHTAB_BRIDGE_URL:-}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Waiting for orchestrator services to become ready..."
wait_for_instance_ready "${PINCHTAB_URL}"
if [ -n "${PINCHTAB_BRIDGE_URL:-}" ]; then
  wait_for_instance_ready "${PINCHTAB_BRIDGE_URL}" 60 "${PINCHTAB_BRIDGE_TOKEN:-}"
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
