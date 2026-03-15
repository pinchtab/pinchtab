#!/bin/bash
set -e

# test.sh — Run Go tests with optional scope
# Usage: test.sh [unit|all]
# Default: all

cd "$(dirname "$0")/.."

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

  go test -json "$@" 2>&1 | tee "$json_file" | while IFS= read -r line; do
    local action test_name package_name elapsed output_text
    action=$(echo "$line" | jq -r '.Action // empty' 2>/dev/null) || continue
    test_name=$(echo "$line" | jq -r '.Test // empty' 2>/dev/null) || continue
    package_name=$(echo "$line" | jq -r '.Package // empty' 2>/dev/null) || continue
    elapsed=$(echo "$line" | jq -r '.Elapsed // empty' 2>/dev/null)
    output_text=$(echo "$line" | jq -r '.Output // empty' 2>/dev/null)

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
          if [ -n "$output_text" ] && [[ "$output_text" =~ ^panic:|^FAIL[[:space:]]|^---[[:space:]]FAIL ]]; then
            if ! $interactive; then
              output_text=${output_text%$'\n'}
              printf "      %s\n" "$output_text"
            fi
          fi
          ;;
        pass)
          if [ -n "$package_name" ] && $interactive; then
            render_progress "${SUCCESS}${package_name}${NC}"
          fi
          ;;
        fail)
          if [ -n "$package_name" ]; then
            if $interactive; then
              render_progress "${ERROR}${package_name}${NC}"
            else
              if [ -n "$elapsed" ]; then
                printf "    ${ERROR}✗${NC} ${MUTED}package${NC} %s ${MUTED}%6ss${NC}\n" "$package_name" "$elapsed"
              else
                printf "    ${ERROR}✗${NC} ${MUTED}package${NC} %s\n" "$package_name"
              fi
            fi
          fi
          ;;
      esac
      continue
    fi

    local display="$test_name"
    local top_level="$test_name"
    if [[ "$top_level" == *"/"* ]]; then
      top_level="${top_level%%/*}"
    fi
    if [ ${#display} -gt $max_len ]; then
      display="${display:0:$((max_len - 1))}…"
    fi

    case "$action" in
      run)
            if $interactive; then
              render_progress "${current_package} ${MUTED}${display}${NC}"
            fi ;;
      pass)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              passed=$((passed + 1))
            fi
            if $interactive; then
              render_progress "${current_package} ${SUCCESS}${display}${NC}"
            fi ;;
      fail)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              failed=$((failed + 1))
            fi
            if $interactive; then
              render_progress "${current_package} ${ERROR}${display}${NC}"
            else
              if [ -n "$elapsed" ]; then
                printf "    ${ERROR}✗${NC} ${MUTED}[%2d]${NC} %-${max_len}s ${MUTED}%6ss${NC}\n" "$completed" "$display" "$elapsed"
              else
                printf "    ${ERROR}✗${NC} ${MUTED}[%2d]${NC} %-${max_len}s\n" "$completed" "$display"
              fi
            fi ;;
      skip)
            if ! grep -Fq "$package_name	$top_level" "$status_file"; then
              printf '%s\t%s\n' "$package_name" "$top_level" >> "$status_file"
              completed=$((completed + 1))
              skipped=$((skipped + 1))
            fi
            if $interactive; then
              render_progress "${current_package} ${ACCENT}${display}${NC}"
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

  if ! run_go_test_json "$UNIT_JSON" "unit" -p 1 -count=1 ./...; then
    fail "test:🔬:go unit"
    test_summary "$UNIT_JSON" "Unit Test Results"
    exit 1
  fi
  ok "test:🔬:go unit"
  test_summary "$UNIT_JSON" "Unit Test Results"
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
