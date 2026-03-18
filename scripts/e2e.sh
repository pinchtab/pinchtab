#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
NC=$'\033[0m'

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
  local services=("$@")

  for service in "${services[@]}"; do
    echo ""
    echo "  ${MUTED}Recent ${service} logs:${NC}"
    docker compose -f "${compose_file}" logs "${service}" | tail -n 80 || true
  done
}

show_suite_summary() {
  local summary_file="$1"
  local report_file="$2"

  if [ -f "${summary_file}" ]; then
    echo ""
    echo "  ${MUTED}Summary: ${summary_file}${NC}"
    cat "${summary_file}" || true
  fi

  if [ -f "${report_file}" ]; then
    echo ""
    echo "  ${MUTED}Report: ${report_file}${NC}"
    sed -n '1,120p' "${report_file}" || true
  fi
}

prepare_suite_results() {
  local summary_file="$1"
  local report_file="$2"

  rm -f \
    "${summary_file}" \
    "${report_file}" \
    tests/e2e/results/summary.txt \
    tests/e2e/results/report.md
}

run_api_fast() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-api-fast.txt"
  local report_file="tests/e2e/results/report-api-fast.md"
  echo "  ${ACCENT}${BOLD}🐳 E2E API Fast tests (Docker)${NC}"
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}"
  set +e
  docker compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh api
  local api_fast_exit=$?
  set -e
  if [ "${api_fast_exit}" -ne 0 ]; then
    show_suite_summary "${summary_file}" "${report_file}"
    dump_compose_failure "${compose_file}" runner-api pinchtab
  fi
  compose_down "${compose_file}"
  return "${api_fast_exit}"
}

run_full_api() {
  local compose_file="tests/e2e/docker-compose-multi.yml"
  local summary_file="tests/e2e/results/summary-api-full.txt"
  local report_file="tests/e2e/results/report-api-full.md"
  echo "  ${ACCENT}${BOLD}🐳 E2E Full API tests (Docker)${NC}"
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}"
  set +e
  docker compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-api runner-api
  local api_exit=$?
  set -e
  if [ "${api_exit}" -ne 0 ]; then
    show_suite_summary "${summary_file}" "${report_file}"
    dump_compose_failure "${compose_file}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
  fi
  compose_down "${compose_file}"
  return "${api_exit}"
}

run_cli_fast() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli-fast.txt"
  local report_file="tests/e2e/results/report-cli-fast.md"
  echo "  ${ACCENT}${BOLD}🐳 E2E CLI Fast tests (Docker)${NC}"
  echo ""
  build_e2e_cli_binary
  prepare_suite_results "${summary_file}" "${report_file}"
  set +e
  docker compose -f "${compose_file}" run --build --rm runner-cli /bin/bash /e2e/run.sh cli
  local cli_fast_exit=$?
  set -e
  if [ "${cli_fast_exit}" -ne 0 ]; then
    show_suite_summary "${summary_file}" "${report_file}"
    dump_compose_failure "${compose_file}" runner-cli pinchtab
  fi
  compose_down "${compose_file}"
  return "${cli_fast_exit}"
}

run_full_cli() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli-full.txt"
  local report_file="tests/e2e/results/report-cli-full.md"
  echo "  ${ACCENT}${BOLD}🐳 E2E Full CLI tests (Docker)${NC}"
  echo ""
  build_e2e_cli_binary
  prepare_suite_results "${summary_file}" "${report_file}"
  set +e
  docker compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-cli runner-cli
  local cli_exit=$?
  set -e
  if [ "${cli_exit}" -ne 0 ]; then
    show_suite_summary "${summary_file}" "${report_file}"
    dump_compose_failure "${compose_file}" runner-cli pinchtab
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
