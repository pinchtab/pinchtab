#!/bin/bash
set -e

# test-dashboard.sh — Run dashboard tests (matches CI: dashboard.yml)
# Runs: vitest run

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
NC=$'\033[0m'

fail() { echo -e "  ${ERROR}✗${NC} $1"; exit 1; }

if [ ! -d "dashboard" ]; then
  fail "Dashboard directory not found"
fi

cd dashboard

# Use bun as in workflow
if ! command -v bun &>/dev/null; then
  fail "bun not found (required for dashboard tests)"
fi

if bun run test:run; then
  echo ""
  echo -e "  ${SUCCESS}${BOLD}Dashboard tests passed!${NC}"
  echo ""
else
  echo ""
  fail "Dashboard tests failed"
fi
