#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
NC=$'\033[0m'

usage() {
  cat <<'EOF'
Usage:
  ./dev smoke [--browser=chrome|cloak|all] [filter...]
  ./dev smoke cdp-attach [chrome|cloak|all]
  ./dev smoke live-detection [--browser=chrome|cloak|all]
  ./dev smoke cloakbrowser [--browser=chrome|cloak|all] [special flags]

Filters:
  cloakbrowser         Run browser parity / CloakBrowser smoke
  browser-parity       Alias for cloakbrowser
  cdp-attach           Run CDP attach smoke
  live-detection       Run advisory live detection smoke
  npm-install          Package + install the npm wrapper in a clean container

Defaults:
  ./dev smoke          Runs Docker smoke categories for supported providers:
                       browser parity, CDP attach, and live detection, plus the
                       provider-independent npm install e2e

Special cloakbrowser flags:
  --multi-target
  --profile-persistence
  --profile-lock-recovery
EOF
}

provider=""
dry_run=0
logs="${E2E_LOGS:-hide}"
declare -a filters=()
declare -a parity_args=()

e2e_smoke_error() {
  echo "${ERROR}E2E smoke now lives under './dev e2e smoke'.${NC}" >&2
  echo "${MUTED}Examples:${NC} ./dev e2e smoke --browser=chrome" >&2
  echo "${MUTED}          ${NC} ./dev e2e smoke --browser=cloak --filter recording" >&2
  exit 1
}

append_unique() {
  local value="$1"
  local existing
  for existing in "${filters[@]:-}"; do
    [ "$existing" = "$value" ] && return
  done
  filters+=("$value")
}

is_named_smoke_filter() {
  case "$1" in
    cloakbrowser|browser-parity|cdp-attach|live-detection|npm-install)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

append_smoke_filter() {
  case "$1" in
    cloakbrowser|browser-parity)
      append_unique "browser-parity"
      ;;
    cdp-attach|live-detection|npm-install)
      append_unique "$1"
      ;;
    *)
      echo "${ERROR}unknown smoke filter: $1${NC}" >&2
      echo "${MUTED}Use './dev e2e smoke --filter $1' for E2E smoke filters.${NC}" >&2
      usage >&2
      exit 1
      ;;
  esac
}

append_requested_filter() {
  local value="$1"
  if [[ "$value" == */* ]]; then
    local old_ifs="$IFS"
    local -a parts=()
    IFS='/'
    read -r -a parts <<<"$value"
    IFS="$old_ifs"

    local part
    for part in "${parts[@]}"; do
      if ! is_named_smoke_filter "$part"; then
        append_unique "$value"
        return
      fi
    done
    for part in "${parts[@]}"; do
      append_smoke_filter "$part"
    done
    return
  fi
  append_smoke_filter "$value"
}

normalize_provider() {
  case "$1" in
    chrome|cloak|all) printf '%s\n' "$1" ;;
    *) echo "invalid --browser: $1 (expected chrome|cloak|all)" >&2; return 1 ;;
  esac
}

set_provider() {
  local normalized
  normalized="$(normalize_provider "$1")" || exit 1
  provider="$normalized"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --help|-h)
      usage
      exit 0
      ;;
    --ci)
      e2e_smoke_error
      ;;
    --dry-run)
      dry_run=1
      ;;
    --logs=*)
      logs="${1#--logs=}"
      ;;
    --logs)
      if [ "$#" -lt 2 ]; then
        echo "${ERROR}--logs requires a value${NC}" >&2
        exit 1
      fi
      shift
      logs="$1"
      ;;
    --browser=*)
      set_provider "${1#--browser=}"
      ;;
    --browser)
      if [ "$#" -lt 2 ]; then
        echo "${ERROR}--browser requires a value${NC}" >&2
        exit 1
      fi
      shift
      set_provider "$1"
      ;;
    --filter=*)
      append_requested_filter "${1#--filter=}"
      ;;
    --filter)
      if [ "$#" -lt 2 ]; then
        echo "${ERROR}--filter requires a value${NC}" >&2
        exit 1
      fi
      shift
      append_requested_filter "$1"
      ;;
    --multi-target|--profile-persistence|--profile-lock-recovery)
      parity_args+=("$1")
      ;;
    --*)
      echo "${ERROR}unknown smoke flag: $1${NC}" >&2
      usage >&2
      exit 1
      ;;
    ci|ci-smoke|e2e|e2e-smoke)
      e2e_smoke_error
      ;;
    cloakbrowser|browser-parity)
      append_smoke_filter "$1"
      ;;
    cdp-attach|live-detection|npm-install)
      append_smoke_filter "$1"
      ;;
    chrome|cloak|all)
      set_provider "$1"
      ;;
    *)
      append_requested_filter "$1"
      ;;
  esac
  shift
done

if [ -z "$provider" ]; then
  provider="all"
fi

case "$logs" in
  show|hide) ;;
  *) echo "${ERROR}--logs must be show or hide${NC}" >&2; exit 1 ;;
esac

run_step() {
  local allow_skip="$1"
  local title="$2"
  shift 2
  local -a cmd=("$@")

  echo ""
  echo "  ${ACCENT}${BOLD}${title}${NC}"
  printf "  ${MUTED}"
  printf "%q " "${cmd[@]}"
  printf "${NC}\n"

  if [ "$dry_run" -eq 1 ]; then
    return 0
  fi

  set +e
  "${cmd[@]}"
  local code=$?
  set -e
  if [ "$code" -eq 0 ]; then
    echo "  ${SUCCESS}${BOLD}passed:${NC} ${title}"
    return 0
  fi
  if [ "$code" -eq 77 ] && [ "$allow_skip" -eq 1 ]; then
    echo "  ${MUTED}skipped:${NC} ${title}"
    return 0
  fi
  echo "  ${ERROR}${BOLD}failed:${NC} ${title} (exit ${code})" >&2
  return "$code"
}

run_browser_parity() {
  local allow_skip="$1"
  local selected_provider="$2"
  run_step "$allow_skip" "Browser parity smoke (${selected_provider})" \
    bash scripts/docker-browser-parity-smoke.sh "--browser=${selected_provider}" ${parity_args[@]+"${parity_args[@]}"}
}

run_cdp_attach() {
  local selected_provider="$1"
  run_step "$2" "CDP attach smoke (${selected_provider})" bash scripts/docker-cdp-attach-smoke.sh "$selected_provider"
}

run_live_detection() {
  local selected_provider="$1"
  run_step "$2" "Live detection smoke (${selected_provider})" bash scripts/docker-live-detection-smoke.sh --browser="$selected_provider"
}

run_npm_install() {
  # Provider-agnostic: packages + installs the npm wrapper in a clean container.
  run_step "$1" "npm install e2e (clean container)" bash scripts/docker-npm-install-smoke.sh
}

failures=0

run_or_record() {
  "$@" || failures=1
}

if [ "${#filters[@]}" -eq 0 ]; then
  # Full Docker smoke excludes the Go E2E smoke subset. Run that explicitly via
  # `./dev e2e smoke`, which keeps the provider smoke harnesses and E2E lanes
  # separate.
  case "$provider" in
    chrome)
      run_or_record run_browser_parity 1 chrome
      run_or_record run_cdp_attach chrome 1
      run_or_record run_live_detection chrome 1
      ;;
    cloak)
      run_or_record run_browser_parity 1 cloak
      run_or_record run_cdp_attach cloak 1
      run_or_record run_live_detection cloak 1
      ;;
    all)
      run_or_record run_browser_parity 1 all
      run_or_record run_cdp_attach all 1
      run_or_record run_live_detection chrome 1
      run_or_record run_live_detection cloak 1
      ;;
  esac
  # Provider-independent; runs once regardless of the selected browser. Allowed
  # to skip (exit 77) when docker/network/release prerequisites are missing.
  run_or_record run_npm_install 1
else
  for filter in "${filters[@]}"; do
    case "$filter" in
      browser-parity)
        run_or_record run_browser_parity 0 "$provider"
        ;;
      cdp-attach)
        run_or_record run_cdp_attach "$provider" 0
        ;;
      live-detection)
        if [ "$provider" = "all" ]; then
          run_or_record run_live_detection chrome 0
          run_or_record run_live_detection cloak 0
        else
          run_or_record run_live_detection "$provider" 0
        fi
        ;;
      npm-install)
        run_or_record run_npm_install 0
        ;;
      *)
        echo "${ERROR}unknown smoke filter: $filter${NC}" >&2
        echo "${MUTED}Use './dev e2e smoke --filter $filter' for E2E smoke filters.${NC}" >&2
        failures=1
        ;;
    esac
  done
fi

if [ "$failures" -ne 0 ]; then
  echo ""
  echo "  ${ERROR}${BOLD}Smoke failed${NC}" >&2
  exit 1
fi

echo ""
echo "  ${SUCCESS}${BOLD}Smoke complete${NC}"
