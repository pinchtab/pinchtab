#!/bin/bash
set -euo pipefail

# check.sh — Local pre-push checks matching GitHub Actions CI
# Runs: format → vet → build → lint

cd "$(dirname "$0")/.."

source scripts/lib/term.sh

trap 'rm -f pinchtab coverage.out 2>/dev/null' EXIT

echo ""
echo -e "  ${ACCENT}${BOLD}🦀 Pinchtab Check${NC}"
echo -e "  ${INFO}Running pre-push checks (matches GitHub Actions CI)...${NC}"

# ── Format ───────────────────────────────────────────────────────────

section "Format"

go_files=()
while IFS= read -r file; do
  [ -f "$file" ] && go_files+=("$file")
done < <(git ls-files -- '*.go'; git ls-files --others --exclude-standard -- '*.go')
unformatted=""
if [ ${#go_files[@]} -gt 0 ]; then
  unformatted=$(gofmt -l "${go_files[@]}")
fi
if [ -n "$unformatted" ]; then
  fail "gofmt" "Files not formatted:"
  echo "$unformatted" | while read f; do hint "  $f"; done
  echo ""
  printf "  Fix formatting now? (Y/n) "
  read -r answer
  if [ "$answer" != "n" ] && [ "$answer" != "N" ]; then
    gofmt -w "${go_files[@]}"
    ok "gofmt (fixed)"
  else
    hint "Run: gofmt -w \$( { git ls-files -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } | while read -r file; do [ -f \"\$file\" ] && printf '%s\\n' \"\$file\"; done)"
    exit 1
  fi
else
  ok "gofmt"
fi

# ── Vet ──────────────────────────────────────────────────────────────

section "Vet"

if ! go vet ./... 2>&1; then
  fail "go vet"
  exit 1
fi
ok "go vet"

# ── Build ────────────────────────────────────────────────────────────

section "Build"

if ! go build -o pinchtab ./cmd/pinchtab 2>&1; then
  fail "go build"
  exit 1
fi
ok "go build"

# ── Lint ─────────────────────────────────────────────────────────────

section "Lint"

LINT_CMD=""
if command -v golangci-lint >/dev/null 2>&1; then
  LINT_CMD="golangci-lint"
elif [ -x "${GOPATH:-$HOME/go}/bin/golangci-lint" ]; then
  LINT_CMD="${GOPATH:-$HOME/go}/bin/golangci-lint"
elif [ -x "$HOME/bin/golangci-lint" ]; then
  LINT_CMD="$HOME/bin/golangci-lint"
elif [ -x "/usr/local/bin/golangci-lint" ]; then
  LINT_CMD="/usr/local/bin/golangci-lint"
fi

if [ -z "$LINT_CMD" ]; then
  fail "golangci-lint" "Required locally because CI runs golangci-lint."
  hint "Install: brew install golangci-lint"
  hint "Or: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
  exit 1
fi

if ! $LINT_CMD run; then
  fail "golangci-lint"
  exit 1
fi
ok "golangci-lint"

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
echo ""
echo -e "  ${SUCCESS}${BOLD}All checks passed!${NC} Ready to push."
echo ""
