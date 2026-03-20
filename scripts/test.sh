#!/bin/bash
set -e

# test.sh — Run Go tests with optional scope
# Usage: test.sh [unit|all]
# Default: all

cd "$(dirname "$0")/.."
TOOLS_BIN="$(pwd)/.tools/bin"

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
MUTED=$'\033[38;2;90;100;128m'
NC=$'\033[0m'

SYSTEM_REGEX='^(TestProxy_InstanceIsolation|TestOrchestrator_(HealthCheck|HashBasedIDs|PortAllocation|PortReuse|ListInstances|FirstRequestLazyChrome|AggregateTabsEndpoint|StopNonexistent|InstanceCleanup))$'

ok()   { echo -e "  ${SUCCESS}✓${NC} $1"; }
fail() { echo -e "  ${ERROR}✗${NC} $1"; }

section() {
  echo ""
  echo -e "  ${ACCENT}${BOLD}$1${NC}"
}

format_elapsed() {
  local elapsed="$1"
  if [ -z "$elapsed" ]; then
    return
  fi
  printf "%.3f" "$elapsed"
}

resolve_gotestsum() {
  if command -v gotestsum >/dev/null 2>&1; then
    command -v gotestsum
    return 0
  fi
  if [ -x "$TOOLS_BIN/gotestsum" ]; then
    echo "$TOOLS_BIN/gotestsum"
    return 0
  fi
  if command -v go >/dev/null 2>&1; then
    local gobin
    gobin="$(go env GOBIN 2>/dev/null)"
    if [ -n "$gobin" ] && [ -x "$gobin/gotestsum" ]; then
      echo "$gobin/gotestsum"
      return 0
    fi
    local gopath
    gopath="$(go env GOPATH 2>/dev/null)"
    if [ -n "$gopath" ] && [ -x "$gopath/bin/gotestsum" ]; then
      echo "$gopath/bin/gotestsum"
      return 0
    fi
  fi
  return 1
}

# Parse gotestsum JSON and print summary
test_summary() {
  local json_file="$1"
  local label="$2"

  [ ! -s "$json_file" ] && return

  local total=0 pass=0 fail=0 skip=0
  read total pass fail skip <<<"$(jq -r \
    'select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip"))
     | [.Package, (.Test | split("/")[0]), .Action] | @tsv' "$json_file" \
    | awk -F'\t' 'NF == 3 { key = $1 "\t" $2; status[key] = $3 }
      END {
        for (k in status) {
          t++
          if (status[k] == "pass") p++
          else if (status[k] == "fail") f++
          else if (status[k] == "skip") s++
        }
        printf "%d %d %d %d\n", t+0, p+0, f+0, s+0
      }')"

  echo ""
  echo -e "    ${BOLD}$label${NC}"
  echo -e "    ${MUTED}────────────────────────────${NC}"
  echo -e "    Total:   ${BOLD}$total${NC}"
  [ "$pass" -gt 0 ] && echo -e "    Passed:  ${SUCCESS}$pass${NC}"
  [ "$fail" -gt 0 ] && echo -e "    Failed:  ${ERROR}$fail${NC}"
  [ "$skip" -gt 0 ] && echo -e "    Skipped: ${ACCENT}$skip${NC}"

  local failed_packages
  failed_packages="$(
    jq -r '
      select(.Package != null and (.Test == null or .Test == "") and (.Action == "pass" or .Action == "fail" or .Action == "skip"))
      | [.Package, .Action] | @tsv
    ' "$json_file" \
      | awk -F'\t' 'NF == 2 { status[$1] = $2 }
        END {
          for (pkg in status) {
            if (status[pkg] == "fail") {
              print pkg
            }
          }
        }' \
      | sort
  )"

  if [ -n "$failed_packages" ]; then
    echo ""
    echo -e "    ${ERROR}Failed packages:${NC}"
    while IFS= read -r pkg; do
      [ -n "$pkg" ] && echo "      ✗ $pkg"
    done <<<"$failed_packages"

    echo ""
    echo -e "    ${ERROR}Failure details:${NC}"
    while IFS= read -r pkg; do
      [ -z "$pkg" ] && continue
      echo "      $pkg"
      jq -r --arg pkg "$pkg" '
        select(.Package == $pkg and .Action == "output")
        | .Output
      ' "$json_file" \
        | sed '/^[[:space:]]*$/d' \
        | sed '/^=== RUN/d' \
        | sed '/^--- PASS:/d' \
        | sed '/^PASS$/d' \
        | tail -n 20 \
        | sed 's/^/        /'
      echo ""
    done <<<"$failed_packages"
  fi

  if [ "$fail" -gt 0 ]; then
    echo ""
    echo -e "    ${ERROR}Failed tests:${NC}"
    jq -r 'select(.Test != null and .Action == "fail") | "      ✗ \(.Test)"' "$json_file" | sort -u
  fi

}

# Live progress for go test -json streams
run_go_test_json() {
  local json_file="$1"; shift
  local label="${1:-tests}"
  shift
  local completed=0
  local passed=0
  local failed=0
  local skipped=0
  local max_len=40
  local interactive=false
  local line_open=false
  local current_package=""
  local status_file="${json_file}.status"

  : > "$status_file"

  if [ -t 1 ]; then
    interactive=true
  fi

  render_progress() {
    local display="$1"
    if $interactive; then
      printf "\r\033[2K    ${MUTED}▸${NC} ${BOLD}%-11s${NC} ${MUTED}pass:%d fail:%d skip:%d${NC} %s" \
        "$label" "$passed" "$failed" "$skipped" "$display"
      line_open=true
    fi
  }

  clear_progress_line() {
    if $interactive && $line_open; then
      printf "\r\033[2K"
      line_open=false
    fi
  }

  print_package_line() {
    local pkg_action="$1"
    local package_name="$2"
    local elapsed="$3"
    local status_label="${SUCCESS}PASS${NC}"
    local elapsed_display=""

    if [ "$pkg_action" = "fail" ]; then
      status_label="${ERROR}FAIL${NC}"
    elif [ "$pkg_action" = "skip" ]; then
      status_label="${ACCENT}SKIP${NC}"
    fi
    if [ -n "$elapsed" ]; then
      elapsed_display="$(format_elapsed "$elapsed")"
    fi

    clear_progress_line
    if [ -n "$elapsed_display" ]; then
      printf "    %b ${MUTED}package${NC} %s ${MUTED}%6ss${NC}\n" "$status_label" "$package_name" "$elapsed_display"
    else
      printf "    %b ${MUTED}package${NC} %s\n" "$status_label" "$package_name"
    fi
  }

  go test -json "$@" 2>&1 \
    | tee "$json_file" \
    | jq -r --unbuffered '
        [
          (.Action // ""),
          (.Test // ""),
          (.Package // ""),
          ((.Elapsed // "") | tostring),
          ((.Output // "") | gsub("\r?\n$"; ""))
        ] | @tsv
      ' \
    | while IFS=$'\t' read -r action test_name package_name elapsed output_text; do

    if [ -z "$test_name" ]; then
      case "$action" in
        start)
          if [ -n "$package_name" ]; then
            current_package="$package_name"
            if $interactive; then
              render_progress "${MUTED}${package_name}${NC}"
            else
              printf "    ${MUTED}▸${NC} ${MUTED}package${NC} %s\n" "$package_name"
            fi
          fi
          ;;
        output)
          if [ -n "$output_text" ] && [[ "$output_text" =~ ^panic:|^---[[:space:]]FAIL ]]; then
            if ! $interactive; then
              output_text=${output_text%$'\n'}
              printf "      %s\n" "$output_text"
            fi
          fi
          ;;
        pass)
          if [ -n "$package_name" ]; then
            print_package_line "$action" "$package_name" "$elapsed"
          fi
          ;;
        fail)
          if [ -n "$package_name" ]; then
            print_package_line "$action" "$package_name" "$elapsed"
          fi
          ;;
        skip)
          if [ -n "$package_name" ]; then
            print_package_line "$action" "$package_name" "$elapsed"
          fi
          ;;
      esac
      continue
    fi

    local top_level="$test_name"
    if [[ "$top_level" == *"/"* ]]; then
      top_level="${top_level%%/*}"
    fi

    case "$action" in
      run)
            if $interactive; then
              render_progress "${MUTED}${current_package}${NC}"
            fi ;;
      pass)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              passed=$((passed + 1))
            fi
            if $interactive; then
              render_progress "${MUTED}${current_package}${NC}"
            fi ;;
      fail)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              failed=$((failed + 1))
            fi
            if $interactive; then
              render_progress "${MUTED}${current_package}${NC}"
            fi ;;
      skip)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              skipped=$((skipped + 1))
            fi
            if $interactive; then
              render_progress "${MUTED}${current_package}${NC}"
            fi ;;
    esac
  done
  local test_status=${PIPESTATUS[0]}
  clear_progress_line
  return "$test_status"
}

SCOPE="${1:-all}"
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# ── Unit tests ───────────────────────────────────────────────────────

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "unit" ]; then
  section "test:🔬:go unit"

  UNIT_JSON="$TMPDIR_TEST/unit.json"
  GOTESTSUM_BIN=""
  if GOTESTSUM_BIN="$(resolve_gotestsum)"; then
    :
  else
    GOTESTSUM_BIN=""
  fi

  if [ -n "$GOTESTSUM_BIN" ]; then
    if ! "$GOTESTSUM_BIN" --format=pkgname --hide-summary=output --jsonfile "$UNIT_JSON" -- -p 1 -count=1 ./...; then
      fail "test:🔬:go unit"
      exit 1
    fi
  else
    echo -e "    ${MUTED}gotestsum not found; using built-in formatter${NC}"
    echo -e "    ${MUTED}Install: go install gotest.tools/gotestsum@latest${NC}"
    echo -e "    ${MUTED}Or run: ./dev doctor${NC}"
    echo ""
    if ! run_go_test_json "$UNIT_JSON" "unit" -p 1 -count=1 ./...; then
      fail "test:🔬:go unit"
      test_summary "$UNIT_JSON" "Unit Test Results"
      exit 1
    fi
    test_summary "$UNIT_JSON" "Unit Test Results"
  fi
  ok "test:🔬:go unit"
fi

# ── Dashboard ────────────────────────────────────────────────────────

if [ "$SCOPE" = "all" ] || [ "$SCOPE" = "dashboard" ]; then
  section "test:🔬:dashboard"
  ./scripts/test-dashboard.sh
fi

# ── Summary ──────────────────────────────────────────────────────────

section "Summary"
echo ""
echo -e "  ${SUCCESS}${BOLD}All tests passed!${NC}"
echo ""
