#!/bin/bash
# run.sh - Run grouped E2E scenarios for a suite directory.

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SUITE="${1:-api}"
shift || true

case "$SUITE" in
  api|scenarios-api|scenarios)
    source "${ROOT_DIR}/helpers/api.sh"
    GROUP_DIR="${ROOT_DIR}/scenarios-api"
    SUITE_KIND="api"
    RUN_ALL=false
    SUITE_TITLE_BASIC="PinchTab E2E API Fast Suite"
    SUITE_TITLE_ALL="PinchTab E2E Test Suite"
    SUMMARY_FILE_BASIC="summary-api-fast.txt"
    SUMMARY_FILE_ALL="summary-api-full.txt"
    REPORT_FILE_BASIC="report-api-fast.md"
    REPORT_FILE_ALL="report-api-full.md"
    PROGRESS_FILE_BASIC="progress-api-fast.log"
    PROGRESS_FILE_ALL="progress-api-full.log"
    ;;
  cli|scenarios-cli)
    source "${ROOT_DIR}/helpers/cli.sh"
    GROUP_DIR="${ROOT_DIR}/scenarios-cli"
    SUITE_KIND="cli"
    RUN_ALL=false
    SUITE_TITLE_BASIC="  PinchTab CLI Fast E2E Suite"
    SUITE_TITLE_ALL="  PinchTab CLI E2E Tests"
    SUMMARY_FILE_BASIC="summary-cli-fast.txt"
    SUMMARY_FILE_ALL="summary-cli-full.txt"
    REPORT_FILE_BASIC="report-cli-fast.md"
    REPORT_FILE_ALL="report-cli-full.md"
    PROGRESS_FILE_BASIC="progress-cli-fast.log"
    PROGRESS_FILE_ALL="progress-cli-full.log"
    ;;
  *)
    echo "unknown suite: $SUITE" >&2
    echo "usage: /bin/bash tests/e2e/run.sh api|cli [all=true|all=false] [filter=<substring>]" >&2
    exit 1
    ;;
esac

SCENARIO_FILTER="${E2E_SCENARIO_FILTER:-}"

for arg in "$@"; do
  case "$arg" in
    all=true)
      RUN_ALL=true
      ;;
    all=false)
      RUN_ALL=false
      ;;
    filter=*)
      SCENARIO_FILTER="${arg#filter=}"
      ;;
    *)
      echo "unknown argument: $arg" >&2
      echo "usage: /bin/bash tests/e2e/run.sh api|cli [all=true|all=false] [filter=<substring>]" >&2
      exit 1
      ;;
  esac
done

SCENARIO_GROUPS=()
for basic_path in "${GROUP_DIR}"/*-basic.sh; do
  if [ ! -f "${basic_path}" ]; then
    echo "no basic group entries found in: ${GROUP_DIR}" >&2
    exit 1
  fi

  feature=$(basename "${basic_path}" -basic.sh)
  basic_script="${feature}-basic.sh"
  SCENARIO_GROUPS+=("${basic_script}")

  if [ "$RUN_ALL" = "true" ]; then
    full_script="${feature}-full.sh"
    full_path="${GROUP_DIR}/${full_script}"
    if [ -f "${full_path}" ]; then
      SCENARIO_GROUPS+=("${full_script}")
    fi
  fi
done

if [ "$RUN_ALL" = "true" ]; then
  for full_path in "${GROUP_DIR}"/*-full.sh; do
    if [ ! -f "${full_path}" ]; then
      echo "no full group entries found in: ${GROUP_DIR}" >&2
      exit 1
    fi

    full_script=$(basename "${full_path}")
    case " ${SCENARIO_GROUPS[*]} " in
      *" ${full_script} "*) ;;
      *) SCENARIO_GROUPS+=("${full_script}") ;;
    esac
  done
fi

if [ -n "$SCENARIO_FILTER" ]; then
  FILTERED_GROUPS=()
  for script_name in "${SCENARIO_GROUPS[@]}"; do
    if [[ "${script_name}" == *"${SCENARIO_FILTER}"* ]]; then
      FILTERED_GROUPS+=("${script_name}")
    fi
  done
  SCENARIO_GROUPS=("${FILTERED_GROUPS[@]}")
  if [ "${#SCENARIO_GROUPS[@]}" -eq 0 ]; then
    echo "no scenario files matched filter: ${SCENARIO_FILTER}" >&2
    exit 1
  fi
fi

if [ "$RUN_ALL" = "true" ]; then
  SUITE_TITLE="$SUITE_TITLE_ALL"
  SUMMARY_FILE="$SUMMARY_FILE_ALL"
  REPORT_FILE="$REPORT_FILE_ALL"
  PROGRESS_FILE="$PROGRESS_FILE_ALL"
else
  SUITE_TITLE="$SUITE_TITLE_BASIC"
  SUMMARY_FILE="$SUMMARY_FILE_BASIC"
  REPORT_FILE="$REPORT_FILE_BASIC"
  PROGRESS_FILE="$PROGRESS_FILE_BASIC"
fi

export E2E_SUMMARY_TITLE="$SUITE_TITLE"
export E2E_SUMMARY_FILE="$SUMMARY_FILE"
export E2E_REPORT_FILE="$REPORT_FILE"
export E2E_PROGRESS_FILE="$PROGRESS_FILE"
export E2E_GENERATE_MARKDOWN_REPORT=1

if [ "$SUITE_KIND" = "api" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo -e "${BLUE}${SUITE_TITLE}${NC}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "E2E_SERVER: ${E2E_SERVER}"
  echo "FIXTURES_URL: ${FIXTURES_URL}"
  if [ -n "$SCENARIO_FILTER" ]; then
    echo "FILTER: ${SCENARIO_FILTER}"
  fi
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""

  echo "Waiting for instances to become ready..."
  wait_for_instance_ready "${E2E_SERVER}"
  if [ "$RUN_ALL" = "true" ]; then
    wait_for_instance_ready "${E2E_SECURE_SERVER}"
    if [ -n "${E2E_MEDIUM_SERVER:-}" ]; then
      wait_for_instance_ready "${E2E_MEDIUM_SERVER}"
    fi
    if [ -n "${E2E_FULL_SERVER:-}" ]; then
      wait_for_instance_ready "${E2E_FULL_SERVER}"
    fi
    if [ -n "${E2E_LITE_SERVER:-}" ]; then
      wait_for_instance_ready "${E2E_LITE_SERVER}"
    fi
    if [ -n "${E2E_BRIDGE_URL:-}" ]; then
      wait_for_instance_ready "${E2E_BRIDGE_URL}" 60 "${E2E_BRIDGE_TOKEN:-}"
    fi
  fi
  echo ""
else
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "${SUITE_TITLE}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
  echo "  Server: $E2E_SERVER"
  echo "  Fixtures: $FIXTURES_URL"
  if [ -n "$SCENARIO_FILTER" ]; then
    echo "  Filter: $SCENARIO_FILTER"
  fi
  echo ""

  wait_for_instance_ready "$E2E_SERVER"

  if ! command -v pinchtab &> /dev/null; then
    echo "ERROR: pinchtab CLI not found in PATH"
    exit 1
  fi

  echo ""
  if [ "$RUN_ALL" = "true" ]; then
    echo "Running CLI tests..."
  else
    echo "Running CLI fast tests..."
  fi
  echo ""
fi

for script_name in "${SCENARIO_GROUPS[@]}"; do
  script_path="${GROUP_DIR}/${script_name}"
  if [ ! -f "${script_path}" ]; then
    echo "group entry not found: ${script_path}" >&2
    exit 1
  fi

  if [ -d "${RESULTS_DIR:-}" ] && [ -n "${E2E_PROGRESS_FILE:-}" ]; then
    printf '%s RUNNING %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${script_name}" >> "${RESULTS_DIR}/${E2E_PROGRESS_FILE}"
  fi

  echo -e "${YELLOW}Running: ${script_name}${NC}"
  echo ""
  CURRENT_SCENARIO_FILE="${script_name%.sh}"
  source "${script_path}"
  echo ""

  if [ -d "${RESULTS_DIR:-}" ] && [ -n "${E2E_PROGRESS_FILE:-}" ]; then
    printf '%s DONE %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${script_name}" >> "${RESULTS_DIR}/${E2E_PROGRESS_FILE}"
  fi
done

print_summary
