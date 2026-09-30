#!/bin/bash
set -euo pipefail

# check-dashboard.sh — Dashboard quality checks (matches CI: dashboard.yml)
# Runs: typecheck → eslint → prettier

cd "$(dirname "$0")/.."

source scripts/lib/term.sh

confirm() {
  if [ -r /dev/tty ]; then
    echo -n "$1 [y/N] " > /dev/tty
    read -r answer < /dev/tty
    [[ "$answer" =~ ^[Yy]$ ]]
    return
  fi

  if [ ! -t 0 ]; then
    return 1
  fi

  echo -n "$1 [y/N] "
  read -r answer
  [[ "$answer" =~ ^[Yy]$ ]]
}

if [ ! -d "dashboard" ]; then
  fail "Dashboard directory not found"
  exit 1
fi

cd dashboard

# Detect runner
RUN="bun"
if ! command -v bun &>/dev/null; then
  if command -v npm &>/dev/null; then
    RUN="npm"
  else
    fail "Neither bun nor npm found"
    exit 1
  fi
fi

# Install deps if needed
if [ ! -d "node_modules" ]; then
  section "Dependencies"
  echo -e "  ${MUTED}Installing...${NC}"
  $RUN install 2>&1 | tail -1
fi

section "Types (tygo)"
GENERATED_TYPES="src/generated"
BEFORE_FIX="$(mktemp -d)"
cp -R "$GENERATED_TYPES/." "$BEFORE_FIX"
go tool -modfile=../tools/go.mod tygo generate
$RUN run types:format
if diff -rq "$BEFORE_FIX" "$GENERATED_TYPES" >/dev/null; then
  ok "Types in sync"
else
  fail "Types out of sync" "Run: cd dashboard && go tool -modfile=../tools/go.mod tygo generate && $RUN run types:format"
  if confirm "Fix generated types now?"; then
    ok "Types fixed"
  else
    rm -rf "${BEFORE_FIX:?}"
    exit 1
  fi
fi
rm -rf "${BEFORE_FIX:?}"

section "TypeScript"
if $RUN run typecheck 2>&1; then
  ok "Type check"
else
  fail "Type errors"
  exit 1
fi

section "ESLint"
if $RUN run lint 2>&1; then
  ok "ESLint"
else
  fail "Lint errors"
  exit 1
fi

section "Prettier"
if $RUN run format:check 2>&1; then
  ok "Formatting"
else
  fail "Files not formatted"
  echo -e "    ${MUTED}Run: cd dashboard && $RUN run format${NC}"
  if confirm "Fix formatting now?"; then
    if $RUN run format 2>&1; then
      if $RUN run format:check 2>&1; then
        ok "Formatting fixed"
      else
        fail "Formatting still not clean"
        exit 1
      fi
    else
      fail "Formatting fix failed"
      exit 1
    fi
  else
    exit 1
  fi
fi

section "Summary"
echo ""
echo -e "  ${SUCCESS}${BOLD}Dashboard checks passed!${NC}"
echo ""
