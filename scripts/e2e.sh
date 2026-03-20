#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
NC=$'\033[0m'
E2E_FILTER="${2:-}"

build_e2e_cli_binary() {
  echo "  ${MUTED}Building static binary for E2E CLI tests...${NC}"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tests/e2e/runner-cli/pinchtab ./cmd/pinchtab
  echo "  ${SUCCESS}✓${NC} Binary built"
  echo ""
}

compose_down() {
  local compose_file="$1"
  docker compose -f "${compose_file}" down -v 2>/dev/null || true
}

dump_compose_failure() {
  local compose_file="$1"
  shift
  local log_prefix="$1"
  shift
  local services=("$@")

  mkdir -p tests/e2e/results
  for service in "${services[@]}"; do
    docker compose -f "${compose_file}" logs "${service}" > "tests/e2e/results/${log_prefix}-${service}.log" 2>&1 || true
  done
}

show_suite_artifacts() {
  local summary_file="$1"
  local report_file="$2"
  local progress_file="$3"
  local log_prefix="$4"
  shift 4
  local services=("$@")
  local printed=0

  if [ -f "${summary_file}" ]; then
    echo ""
    echo "  ${MUTED}Summary saved to: ${summary_file}${NC}"
    printed=1
  fi

  if [ -f "${report_file}" ]; then
    echo "  ${MUTED}Report saved to: ${report_file}${NC}"
    printed=1
  fi

  if [ -f "${progress_file}" ]; then
    echo "  ${MUTED}Progress saved to: ${progress_file}${NC}"
    printed=1
  fi

  for service in "${services[@]}"; do
    local service_log="tests/e2e/results/${log_prefix}-${service}.log"
    if [ -f "${service_log}" ]; then
      echo "  ${MUTED}Logs saved to: ${service_log}${NC}"
      printed=1
    fi
  done

  if [ "${printed}" -eq 1 ]; then
    echo ""
  fi
}

show_suite_summary() {
  local compose_file="$1"
  shift
  :
}

prepare_suite_results() {
  local summary_file="$1"
  local report_file="$2"
  local progress_file="$3"
  local log_prefix="$4"

  rm -f \
    "${summary_file}" \
    "${report_file}" \
    "${progress_file}" \
    tests/e2e/results/${log_prefix}-*.log \
    tests/e2e/results/summary.txt \
    tests/e2e/results/report.md
}

run_api_fast() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-api-fast.txt"
  local report_file="tests/e2e/results/report-api-fast.md"
  local progress_file="tests/e2e/results/progress-api-fast.log"
  local log_prefix="logs-api-fast"
  echo "  ${ACCENT}${BOLD}🐳 E2E API Fast tests (Docker)${NC}"
  if [ -n "${E2E_FILTER}" ]; then
    echo "  ${MUTED}filter: ${E2E_FILTER}${NC}"
  fi
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}"
  set +e
  if [ -n "${E2E_FILTER}" ]; then
    docker compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh api "filter=${E2E_FILTER}"
  else
    docker compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh api
  fi
  local api_fast_exit=$?
  set -e
  if [ "${api_fast_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" runner-api pinchtab
  fi
  compose_down "${compose_file}"
  return "${api_fast_exit}"
}

run_full_api() {
  local compose_file="tests/e2e/docker-compose-multi.yml"
  local summary_file="tests/e2e/results/summary-api-full.txt"
  local report_file="tests/e2e/results/report-api-full.md"
  local progress_file="tests/e2e/results/progress-api-full.log"
  local log_prefix="logs-api-full"
  echo "  ${ACCENT}${BOLD}🐳 E2E Full API tests (Docker)${NC}"
  if [ -n "${E2E_FILTER}" ]; then
    echo "  ${MUTED}filter: ${E2E_FILTER}${NC}"
  fi
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}"
  set +e
  E2E_SCENARIO_FILTER="${E2E_FILTER}" docker compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-api runner-api
  local api_exit=$?
  set -e
  if [ "${api_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
  fi
  compose_down "${compose_file}"
  return "${api_exit}"
}

run_cli_fast() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli-fast.txt"
  local report_file="tests/e2e/results/report-cli-fast.md"
  local progress_file="tests/e2e/results/progress-cli-fast.log"
  local log_prefix="logs-cli-fast"
  echo "  ${ACCENT}${BOLD}🐳 E2E CLI Fast tests (Docker)${NC}"
  if [ -n "${E2E_FILTER}" ]; then
    echo "  ${MUTED}filter: ${E2E_FILTER}${NC}"
  fi
  echo ""
  build_e2e_cli_binary
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}"
  set +e
  if [ -n "${E2E_FILTER}" ]; then
    docker compose -f "${compose_file}" run --build --rm runner-cli /bin/bash /e2e/run.sh cli "filter=${E2E_FILTER}"
  else
    docker compose -f "${compose_file}" run --build --rm runner-cli /bin/bash /e2e/run.sh cli
  fi
  local cli_fast_exit=$?
  set -e
  if [ "${cli_fast_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-cli pinchtab
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" runner-cli pinchtab
  fi
  compose_down "${compose_file}"
  return "${cli_fast_exit}"
}

run_full_cli() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli-full.txt"
  local report_file="tests/e2e/results/report-cli-full.md"
  local progress_file="tests/e2e/results/progress-cli-full.log"
  local log_prefix="logs-cli-full"
  echo "  ${ACCENT}${BOLD}🐳 E2E Full CLI tests (Docker)${NC}"
  if [ -n "${E2E_FILTER}" ]; then
    echo "  ${MUTED}filter: ${E2E_FILTER}${NC}"
  fi
  echo ""
  build_e2e_cli_binary
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}"
  set +e
  E2E_SCENARIO_FILTER="${E2E_FILTER}" docker compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-cli runner-cli
  local cli_exit=$?
  set -e
  if [ "${cli_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-cli pinchtab
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" runner-cli pinchtab
  fi
  compose_down "${compose_file}"
  return "${cli_exit}"
}

run_pr() {
  local api_fast_exit=0
  local cli_fast_exit=0

  run_api_fast || api_fast_exit=$?

  echo ""

  run_cli_fast || cli_fast_exit=$?

  echo ""
  if [ "${api_fast_exit}" -ne 0 ] || [ "${cli_fast_exit}" -ne 0 ]; then
    echo "  ${ERROR}PR E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: api-fast=${api_fast_exit}, cli-fast=${cli_fast_exit}${NC}"
    return 1
  fi
  echo "  ${SUCCESS}PR E2E suites passed${NC}"
  return 0
}

run_release() {
  local api_exit=0
  local cli_exit=0

  run_full_api || api_exit=$?

  echo ""

  run_full_cli || cli_exit=$?

  echo ""
  if [ "${api_exit}" -ne 0 ] || [ "${cli_exit}" -ne 0 ]; then
    echo "  ${ERROR}Some E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: api-full=${api_exit}, cli-full=${cli_exit}${NC}"
    return 1
  fi
  echo "  ${SUCCESS}All E2E suites passed${NC}"
  return 0
}

chmod -R 755 tests/e2e/fixtures/test-extension* 2>/dev/null || true

suite="${1:-release}"

case "${suite}" in
  pr)
    run_pr
    ;;
  api-fast)
    run_api_fast
    ;;
  cli-fast)
    run_cli_fast
    ;;
  api-full|full-api|curl)
    run_full_api
    ;;
  cli-full|full-cli|cli)
    run_full_cli
    ;;
  release|all)
    run_release
    ;;
  *)
    echo "Unknown E2E suite: ${suite}" >&2
    echo "Available suites: pr, api-fast, cli-fast, api-full, cli-full, release" >&2
    exit 1
    ;;
esac
