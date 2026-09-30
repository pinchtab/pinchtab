# shellcheck shell=bash

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
INFO=$'\033[38;2;136;146;176m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

ok() { echo -e "  ${SUCCESS}✓${NC} $1"; }

fail() {
  echo -e "  ${ERROR}✗${NC} $1"
  if [ -n "${2:-}" ]; then
    echo -e "    ${MUTED}$2${NC}"
  fi
  return 0
}

hint() { echo -e "    ${MUTED}$1${NC}"; }

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}
