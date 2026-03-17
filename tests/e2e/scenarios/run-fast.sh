#!/bin/bash
# run-fast.sh - Run the curated PR-fast API scenarios.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SUITE_FILE="${E2E_SUITE_FILE:-/e2e-suites/api-fast.txt}"

source "${SCRIPT_DIR}/common.sh"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}PinchTab E2E API Fast Suite${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "E2E_SERVER: ${E2E_SERVER}"
echo "FIXTURES_URL: ${FIXTURES_URL}"
echo "SUITE_FILE: ${SUITE_FILE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ ! -f "${SUITE_FILE}" ]; then
  echo "suite manifest not found: ${SUITE_FILE}" >&2
  exit 1
fi

echo "Waiting for instances to become ready..."
wait_for_instance_ready "${E2E_SERVER}"
wait_for_instance_ready "${E2E_SECURE_SERVER}"
if [ -n "${E2E_LITE_SERVER:-}" ]; then
  wait_for_instance_ready "${E2E_LITE_SERVER}"
fi
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

  echo -e "${YELLOW}Running: ${script_name}${NC}"
  echo ""
  source "${script_path}"
  echo ""
done < "${SUITE_FILE}"

print_summary

if [ -d "${RESULTS_DIR:-}" ]; then
  echo "passed=$TESTS_PASSED" > "${RESULTS_DIR}/summary-api-fast.txt"
  echo "failed=$TESTS_FAILED" >> "${RESULTS_DIR}/summary-api-fast.txt"
  echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${RESULTS_DIR}/summary-api-fast.txt"
fi
