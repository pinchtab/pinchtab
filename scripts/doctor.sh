#!/usr/bin/env bash
set -euo pipefail

# doctor.sh — Verify and setup development environment for pinchtab
# Interactive: asks before installing anything

BOLD='\033[1m'
ACCENT='\033[38;2;251;191;36m'      # yellow #fbbf24
INFO='\033[38;2;136;146;176m'       # muted #8892b0
SUCCESS='\033[38;2;0;229;204m'      # cyan #00e5cc
ERROR='\033[38;2;230;57;70m'        # red #e63946
MUTED='\033[38;2;90;100;128m'       # text-muted #5a6480
NC='\033[0m'

CRITICAL=0
WARNINGS=0
BUN_MIN_VERSION="1.2.0"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_BIN="$ROOT_DIR/.tools/bin"

# ── Helpers ──────────────────────────────────────────────────────────

ok()      { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail()    { echo -e "  ${ERROR}✗${NC} $1"; [ -n "${2:-}" ] && echo -e "    ${MUTED}$2${NC}"; CRITICAL=$((CRITICAL + 1)); }
warn()    { echo -e "  ${ACCENT}·${NC} $1"; [ -n "${2:-}" ] && echo -e "    ${MUTED}$2${NC}"; WARNINGS=$((WARNINGS + 1)); }
hint()    { echo -e "    ${MUTED}$1${NC}"; }

confirm() {
  echo -ne "    ${BOLD}$1 [y/N]${NC} "
  read -r answer
  [[ "$answer" =~ ^[Yy]$ ]]
}

section() {
  echo ""
  echo -e "${ACCENT}${BOLD}$1${NC}"
}

version_ge() {
  local current="${1#v}"
  local minimum="${2#v}"
  local current_major current_minor current_patch
  local minimum_major minimum_minor minimum_patch

  IFS='.' read -r current_major current_minor current_patch <<< "${current}"
  IFS='.' read -r minimum_major minimum_minor minimum_patch <<< "${minimum}"

  current_minor=${current_minor:-0}
  current_patch=${current_patch:-0}
  minimum_minor=${minimum_minor:-0}
  minimum_patch=${minimum_patch:-0}

  if [ "$current_major" -ne "$minimum_major" ]; then
    [ "$current_major" -gt "$minimum_major" ]
    return
  fi

  if [ "$current_minor" -ne "$minimum_minor" ]; then
    [ "$current_minor" -gt "$minimum_minor" ]
    return
  fi

  [ "$current_patch" -ge "$minimum_patch" ]
}

# ── Detect package manager ───────────────────────────────────────────

HAS_BREW=false
HAS_APT=false
command -v brew &>/dev/null && HAS_BREW=true
command -v apt-get &>/dev/null && HAS_APT=true

# ── Start ────────────────────────────────────────────────────────────

echo ""
echo -e "  ${ACCENT}${BOLD}🦀 Pinchtab Doctor${NC}"
echo -e "  ${INFO}Verifying and setting up development environment...${NC}"

section "Go Backend"

# ── Go ───────────────────────────────────────────────────────────────

if command -v go &>/dev/null; then
  GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
  GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
  GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)

  if [ "$GO_MAJOR" -ge 1 ] && [ "$GO_MINOR" -ge 25 ]; then
    ok "Go $GO_VERSION"
  else
    fail "Go $GO_VERSION — requires 1.25+"
    if $HAS_BREW && confirm "Install latest Go via brew?"; then
      brew install go && ok "Go installed" && CRITICAL=$((CRITICAL - 1))
    else
      hint "Install from https://go.dev/dl/"
    fi
  fi
else
  fail "Go not found"
  if $HAS_BREW && confirm "Install Go via brew?"; then
    brew install go && ok "Go installed" && CRITICAL=$((CRITICAL - 1))
  else
    hint "Install from https://go.dev/dl/"
  fi
fi

# ── golangci-lint ────────────────────────────────────────────────────

GOLANGCI_LINT=""
if command -v golangci-lint &>/dev/null; then
  GOLANGCI_LINT="golangci-lint"
elif [ -x "${GOPATH:-$HOME/go}/bin/golangci-lint" ]; then
  GOLANGCI_LINT="${GOPATH:-$HOME/go}/bin/golangci-lint"
fi

if [ -n "$GOLANGCI_LINT" ]; then
  LINT_VERSION=$($GOLANGCI_LINT version --short 2>/dev/null || $GOLANGCI_LINT --version 2>/dev/null | head -1 | awk '{print $4}')
  ok "golangci-lint $LINT_VERSION"
else
  fail "golangci-lint" "Required for pre-commit hooks and CI."
  if $HAS_BREW && confirm "Install golangci-lint via brew?"; then
    brew install golangci-lint && ok "golangci-lint installed" && CRITICAL=$((CRITICAL - 1))
  elif command -v go &>/dev/null && confirm "Install golangci-lint via go install?"; then
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && ok "golangci-lint installed" && CRITICAL=$((CRITICAL - 1))
  else
    hint "brew install golangci-lint"
    hint "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
  fi
fi

# ── gotestsum ────────────────────────────────────────────────────────

mkdir -p "$TOOLS_BIN"

GOTESTSUM=""
if command -v gotestsum &>/dev/null; then
  GOTESTSUM="gotestsum"
elif [ -x "$TOOLS_BIN/gotestsum" ]; then
  GOTESTSUM="$TOOLS_BIN/gotestsum"
elif [ -x "${GOPATH:-$HOME/go}/bin/gotestsum" ]; then
  GOTESTSUM="${GOPATH:-$HOME/go}/bin/gotestsum"
fi

if [ -n "$GOTESTSUM" ]; then
  GOTESTSUM_VERSION=$($GOTESTSUM --version 2>/dev/null | head -1)
  if [ -n "$GOTESTSUM_VERSION" ]; then
    ok "$GOTESTSUM_VERSION"
  else
    ok "gotestsum"
  fi
else
  warn "gotestsum not found" "Recommended — used by ./dev test unit for cleaner package-oriented output."
  if command -v go &>/dev/null && confirm "Install gotestsum via go install?"; then
    GOBIN="$TOOLS_BIN" go install gotest.tools/gotestsum@latest && ok "gotestsum installed in .tools/bin" && WARNINGS=$((WARNINGS - 1))
  else
    hint "GOBIN=\"$TOOLS_BIN\" go install gotest.tools/gotestsum@latest"
    hint "go install gotest.tools/gotestsum@latest"
  fi
fi

# ── Git hooks ────────────────────────────────────────────────────────

if [ -f ".git/hooks/pre-commit" ]; then
  ok "Git hooks"
else
  warn "Git hooks not installed"
  if confirm "Install git hooks now?"; then
    if ./scripts/install-hooks.sh 2>/dev/null; then
      ok "Git hooks installed"
      WARNINGS=$((WARNINGS - 1))
    else
      cp scripts/pre-commit .git/hooks/pre-commit
      chmod +x .git/hooks/pre-commit
      ok "Git hooks installed"
      WARNINGS=$((WARNINGS - 1))
    fi
  fi
fi

# ── Go dependencies ─────────────────────────────────────────────────

if [ -f "go.mod" ]; then
  if go list -m all &>/dev/null 2>&1; then
    ok "Go dependencies"
  else
    warn "Go dependencies not downloaded"
    if confirm "Download Go dependencies?"; then
      go mod download && ok "Go dependencies downloaded" && WARNINGS=$((WARNINGS - 1))
    fi
  fi
fi

section "Dashboard (React/TypeScript)"

if [ -d "dashboard" ]; then

  # ── Node.js ──────────────────────────────────────────────────────

  if command -v node &>/dev/null; then
    NODE_VERSION=$(node -v | sed 's/v//')
    NODE_MAJOR=$(echo "$NODE_VERSION" | cut -d. -f1)

    if [ "$NODE_MAJOR" -ge 18 ]; then
      ok "Node.js $NODE_VERSION"
    else
      warn "Node.js $NODE_VERSION — 18+ recommended"
      if $HAS_BREW && confirm "Install latest Node.js via brew?"; then
        brew install node && ok "Node.js installed" && WARNINGS=$((WARNINGS - 1))
      else
        hint "Install from https://nodejs.org"
      fi
    fi
  else
    warn "Node.js not found" "Optional — needed for dashboard."
    if $HAS_BREW && confirm "Install Node.js via brew?"; then
      brew install node && ok "Node.js installed" && WARNINGS=$((WARNINGS - 1))
    else
      hint "Install from https://nodejs.org"
    fi
  fi

  # ── Bun ────────────────────────────────────────────────────────

  HAS_SUPPORTED_BUN=false
  if command -v bun &>/dev/null; then
    BUN_VERSION=$(bun -v)
    if version_ge "$BUN_VERSION" "$BUN_MIN_VERSION"; then
      ok "Bun $BUN_VERSION"
      HAS_SUPPORTED_BUN=true
    else
      warn "Bun $BUN_VERSION — requires $BUN_MIN_VERSION+ for dashboard installs" "Older Bun versions try to rewrite dashboard/bun.lock and fail with --frozen-lockfile on clean clones."
      if confirm "Upgrade Bun now?"; then
        curl -fsSL https://bun.sh/install | bash && ok "Bun updated (restart shell to use)" && WARNINGS=$((WARNINGS - 1)) && HAS_SUPPORTED_BUN=true
      else
        hint "curl -fsSL https://bun.sh/install | bash"
      fi
    fi
  else
    warn "Bun not found" "Optional — used for fast dashboard builds."
    if confirm "Install Bun?"; then
      curl -fsSL https://bun.sh/install | bash && ok "Bun installed (restart shell to use)" && WARNINGS=$((WARNINGS - 1)) && HAS_SUPPORTED_BUN=true
    else
      hint "curl -fsSL https://bun.sh/install | bash"
    fi
  fi

  # ── tygo ───────────────────────────────────────────────────────

  TYGO="${GOPATH:-$HOME/go}/bin/tygo"
  if [ -x "$TYGO" ] || command -v tygo &>/dev/null; then
    ok "tygo"
  else
    warn "tygo not found" "Types in the dashboard might fall out of sync with Go structs."
    if command -v go &>/dev/null && confirm "Install tygo via go install?"; then
      go install github.com/gzuidhof/tygo@latest && ok "tygo installed" && WARNINGS=$((WARNINGS - 1))
    else
      hint "go install github.com/gzuidhof/tygo@latest"
    fi
  fi

  # ── gum ────────────────────────────────────────────────────────

  if command -v gum &>/dev/null; then
    ok "gum"
  else
    warn "gum not found" "Optional — used by ./dev for the interactive picker UI."
    if $HAS_BREW && confirm "Install gum via brew?"; then
      brew install gum && ok "gum installed" && WARNINGS=$((WARNINGS - 1))
    elif command -v go &>/dev/null && confirm "Install gum via go install?"; then
      go install github.com/charmbracelet/gum@latest && ok "gum installed" && WARNINGS=$((WARNINGS - 1))
    else
      hint "brew install gum"
      hint "go install github.com/charmbracelet/gum@latest"
    fi
  fi

  # ── Dashboard deps ─────────────────────────────────────────────

  if [ -d "dashboard/node_modules" ]; then
    ok "Dashboard dependencies"
  else
    warn "Dashboard dependencies not installed"
    if $HAS_SUPPORTED_BUN; then
      if confirm "Install dashboard dependencies via bun?"; then
        (cd dashboard && bun install) && ok "Dashboard dependencies installed" && WARNINGS=$((WARNINGS - 1))
      fi
    elif command -v bun &>/dev/null; then
      hint "Upgrade Bun to $BUN_MIN_VERSION+ before installing dashboard dependencies"
    elif command -v npm &>/dev/null; then
      if confirm "Install dashboard dependencies via npm?"; then
        (cd dashboard && npm install) && ok "Dashboard dependencies installed" && WARNINGS=$((WARNINGS - 1))
      fi
    else
      hint "cd dashboard && bun install"
    fi
  fi

else
  echo -e "  ${MUTED}Dashboard directory not found (optional)${NC}"
fi

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
echo ""

if [ $CRITICAL -eq 0 ] && [ $WARNINGS -eq 0 ]; then
  echo -e "  ${SUCCESS}${BOLD}All checks passed!${NC} You're ready to develop."
  echo ""
  echo -e "  ${MUTED}Next steps:${NC}"
  echo -e "    ${MUTED}go build ./cmd/pinchtab${NC}"
  echo -e "    ${MUTED}go test ./...${NC}"
  exit 0
fi

[ $CRITICAL -gt 0 ] && echo -e "  ${ERROR}✗${NC} $CRITICAL critical issue(s) remaining"
[ $WARNINGS -gt 0 ] && echo -e "  ${ACCENT}·${NC} $WARNINGS warning(s)"

if [ $CRITICAL -gt 0 ]; then
  echo ""
  echo -e "  ${MUTED}After installing, run ./doctor.sh again.${NC}"
  exit 1
fi

exit 0
