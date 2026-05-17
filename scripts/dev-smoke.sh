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
  ./dev smoke [--provider=chrome|cloak|all] [filter...]
  ./dev smoke ci [--provider=chrome|cloak] [--filter TEXT]
  ./dev smoke cdp-attach [chrome|cloak|all]
  ./dev smoke live-detection [--provider=chrome|cloak|all]
  ./dev smoke cloakbrowser [--provider=chrome|cloak|all] [special flags]

Filters:
  ci, e2e              Run the CI-backed E2E smoke subset
  cloakbrowser         Run browser parity / CloakBrowser smoke
  browser-parity       Alias for cloakbrowser
  cdp-attach           Run CDP attach smoke
  live-detection       Run advisory live detection smoke
  docker, mcp, api...  Passed to the E2E smoke runner as --filter

Defaults:
  ./dev smoke          Runs all local smoke categories for all supported providers
  ./dev smoke ci       Runs only the CI smoke subset, default provider chrome

Special cloakbrowser flags:
  --multi-target
  --profile-persistence
  --profile-lock-recovery
EOF
}

provider=""
ci_only=0
dry_run=0
logs="${E2E_LOGS:-hide}"
e2e_filter=""
declare -a filters=()
declare -a parity_args=()

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
    ci|ci-smoke|e2e|e2e-smoke|cloakbrowser|browser-parity|cdp-attach|live-detection)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

append_smoke_filter() {
  case "$1" in
    ci|ci-smoke)
      ci_only=1
      append_unique "e2e"
      ;;
    e2e|e2e-smoke)
      append_unique "e2e"
      ;;
    cloakbrowser|browser-parity)
      append_unique "browser-parity"
      ;;
    cdp-attach|live-detection)
      append_unique "$1"
      ;;
    *)
      append_unique "$1"
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
    *) echo "invalid --provider: $1 (expected chrome|cloak|all)" >&2; return 1 ;;
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
      ci_only=1
      append_unique "e2e"
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
    --provider=*)
      set_provider "${1#--provider=}"
      ;;
    --provider)
      if [ "$#" -lt 2 ]; then
        echo "${ERROR}--provider requires a value${NC}" >&2
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
    ci|ci-smoke)
      append_smoke_filter "$1"
      ;;
    e2e|e2e-smoke)
      append_smoke_filter "$1"
      ;;
    cloakbrowser|browser-parity)
      append_smoke_filter "$1"
      ;;
    cdp-attach|live-detection)
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
  if [ "$ci_only" -eq 1 ]; then
    provider="chrome"
  else
    provider="all"
  fi
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

run_e2e_smoke() {
  local selected_provider="$1"
  local filter="${2:-}"
  local -a args=(go run ./tests/tools/runner e2e --suite smoke --provider "$selected_provider" --logs "$logs")
  [ "$dry_run" -eq 1 ] && args+=(--dry-run)
  [ -n "$filter" ] && args+=(--filter "$filter")
  run_step 0 "E2E smoke subset (${selected_provider}${filter:+, filter=${filter}})" "${args[@]}"
}

run_cdp_attach() {
  local selected_provider="$1"
  run_step "$2" "CDP attach smoke (${selected_provider})" bash scripts/docker-cdp-attach-smoke.sh "$selected_provider"
}

run_live_detection() {
  local selected_provider="$1"
  run_step "$2" "Live detection smoke (${selected_provider})" bash scripts/docker-live-detection-smoke.sh --provider="$selected_provider"
}

provider_list() {
  case "$1" in
    all) printf '%s\n' chrome cloak ;;
    *) printf '%s\n' "$1" ;;
  esac
}

failures=0

run_or_record() {
  "$@" || failures=1
}

if [ "$ci_only" -eq 1 ]; then
  if [ "$provider" = "all" ]; then
    echo "${ERROR}./dev smoke ci accepts --provider=chrome|cloak, not all${NC}" >&2
    exit 1
  fi
  if [ "${#filters[@]}" -gt 0 ]; then
    for filter in "${filters[@]}"; do
      [ "$filter" = "e2e" ] && continue
      e2e_filter="$filter"
    done
  fi
  run_e2e_smoke "$provider" "$e2e_filter"
  exit $?
fi

if [ "${#filters[@]}" -eq 0 ]; then
  # Full local smoke: run all categories. Browser parity runs before Cloak E2E
  # because it builds the local CloakBrowser smoke image when needed.
  case "$provider" in
    chrome)
      run_or_record run_e2e_smoke chrome ""
      run_or_record run_step 1 "Browser parity smoke (chrome)" bash scripts/docker-browser-parity-smoke.sh --provider=chrome "${parity_args[@]}"
      run_or_record run_cdp_attach chrome 1
      run_or_record run_live_detection chrome 1
      ;;
    cloak)
      run_or_record run_step 1 "Browser parity smoke (cloak)" bash scripts/docker-browser-parity-smoke.sh --provider=cloak "${parity_args[@]}"
      run_or_record run_e2e_smoke cloak ""
      run_or_record run_cdp_attach cloak 1
      run_or_record run_live_detection cloak 1
      ;;
    all)
      run_or_record run_e2e_smoke chrome ""
      run_or_record run_step 1 "Browser parity smoke (all)" bash scripts/docker-browser-parity-smoke.sh --provider=all "${parity_args[@]}"
      run_or_record run_e2e_smoke cloak ""
      run_or_record run_cdp_attach all 1
      run_or_record run_live_detection chrome 1
      run_or_record run_live_detection cloak 1
      ;;
  esac
else
  for filter in "${filters[@]}"; do
    case "$filter" in
      e2e)
        while IFS= read -r selected_provider; do
          run_or_record run_e2e_smoke "$selected_provider" ""
        done < <(provider_list "$provider")
        ;;
      browser-parity)
        run_or_record run_step 0 "Browser parity smoke (${provider})" bash scripts/docker-browser-parity-smoke.sh --provider="$provider" "${parity_args[@]}"
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
      *)
        while IFS= read -r selected_provider; do
          run_or_record run_e2e_smoke "$selected_provider" "$filter"
        done < <(provider_list "$provider")
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
