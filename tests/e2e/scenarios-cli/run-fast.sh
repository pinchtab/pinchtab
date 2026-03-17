#!/bin/bash
# run-fast.sh - Run the curated PR-fast CLI scenarios.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SUITE_FILE="${E2E_SUITE_FILE:-/e2e-suites/cli-fast.txt}"

source "${SCRIPT_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🦀 PinchTab CLI Fast E2E Suite"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Server: $E2E_SERVER"
echo "  Fixtures: $FIXTURES_URL"
echo "  Suite file: $SUITE_FILE"
echo ""

if [ ! -f "${SUITE_FILE}" ]; then
  echo "suite manifest not found: ${SUITE_FILE}" >&2
  exit 1
fi

wait_for_instance_ready "$E2E_SERVER"

if ! command -v pinchtab &> /dev/null; then
  echo "ERROR: pinchtab CLI not found in PATH"
  exit 1
fi

echo ""
echo "Running CLI fast tests..."
echo ""

while IFS= read -r script_name || [ -n "${script_name}" ]; do
  case "${script_name}" in
    ''|'#'*)
      continue
      ;;
  esac

  script_path="${SCRIPT_DIR}/${script_name}"
  if [ ! -f "${script_path}" ]; then
    echo "suite entry not found: ${script_path}" >&2
    exit 1
  fi

  echo -e "${YELLOW}Running: $(basename "${script_name}")${NC}"
  echo ""
  source "${script_path}"
  echo ""
done < "${SUITE_FILE}"

print_summary

if [ -d "${RESULTS_DIR:-}" ]; then
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary-cli-fast.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary-cli-fast.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary-cli-fast.txt"
fi
