#!/bin/bash
set -e

# check-dashboard.sh — Dashboard quality checks (matches CI: dashboard.yml)
# Runs: typecheck → eslint → prettier

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

ok()   { echo -e "  ${SUCCESS}✓${NC} $1"; return 0; }
fail() {
  echo -e "  ${ERROR}✗${NC} $1"
  if [ -n "${2:-}" ]; then
    echo -e "    ${MUTED}$2${NC}"
  fi
  return 0
}

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}

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
  if command -v npx &>/dev/null; then
    RUN="npx"
  else
    fail "Neither bun nor npx found"
    exit 1
  fi
fi

# Install deps if needed
if [ ! -d "node_modules" ]; then
  section "Dependencies"
  echo -e "  ${MUTED}Installing...${NC}"
  $RUN install 2>&1 | tail -1
fi

# Check tygo types are in sync (if tygo is installed)
TYGO="${GOPATH:-$HOME/go}/bin/tygo"
if [ -x "$TYGO" ] || command -v tygo &>/dev/null; then
  section "Types (tygo)"
  TYGO_CMD="${TYGO:-tygo}"
  GENERATED_TYPES="src/generated/types.ts"
  if [ -x "$TYGO" ]; then TYGO_CMD="$TYGO"; fi
  BEFORE_FIX="$(mktemp)"
  cp "$GENERATED_TYPES" "$BEFORE_FIX"
  $TYGO_CMD generate 2>/dev/null
  npx prettier --write "$GENERATED_TYPES" 2>/dev/null || true
  if cmp -s "$BEFORE_FIX" "$GENERATED_TYPES"; then
    ok "Types in sync"
  else
    fail "Types out of sync" "Run: cd dashboard && tygo generate && npx prettier --write src/generated/types.ts"
    if confirm "Fix generated types now?"; then
      ok "Types fixed"
    else
      rm -f "$BEFORE_FIX"
      exit 1
    fi
  fi
  rm -f "$BEFORE_FIX"
fi

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
