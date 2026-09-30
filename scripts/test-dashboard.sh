#!/bin/bash
set -euo pipefail

# test-dashboard.sh — Run dashboard tests (matches CI: dashboard.yml)
# Runs: vitest run

cd "$(dirname "$0")/.."

source scripts/lib/term.sh

if [ ! -d "dashboard" ]; then
  fail "Dashboard directory not found"
  exit 1
fi

cd dashboard

# Use bun as in workflow
if ! command -v bun &>/dev/null; then
  fail "bun not found (required for dashboard tests)"
  exit 1
fi

if bun run test:run; then
  echo ""
  echo -e "  ${SUCCESS}${BOLD}Dashboard tests passed!${NC}"
  echo ""
else
  echo ""
  fail "Dashboard tests failed"
  exit 1
fi
