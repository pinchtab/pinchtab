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

run_recent() {
  echo "  ${ACCENT}${BOLD}🐳 E2E Recent tests (Docker)${NC}"
  echo ""
  set +e
  docker compose -f tests/e2e/docker-compose.yml run --build --rm runner /scenarios-recent/run.sh
  local recent_exit=$?
  set -e
  if [ "${recent_exit}" -ne 0 ]; then
    echo -e "${ERROR}  Recent tests failed. Showing pinchtab logs:${NC}"
    docker compose -f tests/e2e/docker-compose.yml logs pinchtab | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose.yml
  return "${recent_exit}"
}

run_api_fast() {
  echo "  ${ACCENT}${BOLD}🐳 E2E API Fast tests (Docker)${NC}"
  echo ""
  set +e
  docker compose -f tests/e2e/docker-compose.yml run --build --rm runner /scenarios/run-fast.sh
  local api_fast_exit=$?
  set -e
  if [ "${api_fast_exit}" -ne 0 ]; then
    docker compose -f tests/e2e/docker-compose.yml logs pinchtab | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose.yml
  return "${api_fast_exit}"
}

run_full_api() {
  echo "  ${ACCENT}${BOLD}🐳 E2E Full API tests (Docker)${NC}"
  echo ""
  set +e
  docker compose -f tests/e2e/docker-compose.yml up --build --abort-on-container-exit
  local api_exit=$?
  set -e
  if [ "${api_exit}" -ne 0 ]; then
    docker compose -f tests/e2e/docker-compose.yml logs pinchtab | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose.yml
  return "${api_exit}"
}

run_cli_fast() {
  echo "  ${ACCENT}${BOLD}🐳 E2E CLI Fast tests (Docker)${NC}"
  echo ""
  build_e2e_cli_binary
  set +e
  docker compose -f tests/e2e/docker-compose-cli.yml run --build --rm runner /bin/bash /scenarios/run-fast.sh
  local cli_fast_exit=$?
  set -e
  if [ "${cli_fast_exit}" -ne 0 ]; then
    docker compose -f tests/e2e/docker-compose-cli.yml logs pinchtab | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose-cli.yml
  return "${cli_fast_exit}"
}

run_full_cli() {
  echo "  ${ACCENT}${BOLD}🐳 E2E Full CLI tests (Docker)${NC}"
  echo ""
  build_e2e_cli_binary
  set +e
  docker compose -f tests/e2e/docker-compose-cli.yml up --build --abort-on-container-exit
  local cli_exit=$?
  set -e
  if [ "${cli_exit}" -ne 0 ]; then
    docker compose -f tests/e2e/docker-compose-cli.yml logs pinchtab | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose-cli.yml
  return "${cli_exit}"
}

run_orchestrator() {
  echo "  ${ACCENT}${BOLD}🐳 E2E Orchestrator tests (Docker)${NC}"
  echo ""
  set +e
  docker compose -f tests/e2e/docker-compose-orchestrator.yml run --build --rm runner
  local orch_exit=$?
  set -e
  if [ "${orch_exit}" -ne 0 ]; then
    docker compose -f tests/e2e/docker-compose-orchestrator.yml logs pinchtab | tail -n 50 || true
    docker compose -f tests/e2e/docker-compose-orchestrator.yml logs pinchtab-bridge | tail -n 50 || true
  fi
  compose_down tests/e2e/docker-compose-orchestrator.yml
  return "${orch_exit}"
}

run_full_extended() {
  local recent_exit=0
  local orch_exit=0

  run_recent || recent_exit=$?

  echo ""

  run_orchestrator || orch_exit=$?

  echo ""
  if [ "${recent_exit}" -ne 0 ] || [ "${orch_exit}" -ne 0 ]; then
    echo "  ${ERROR}Extended E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: recent=${recent_exit}, orchestrator=${orch_exit}${NC}"
    return 1
  fi
  echo "  ${SUCCESS}Extended E2E suites passed${NC}"
  return 0
}

run_pr() {
  local recent_exit=0
  local api_fast_exit=0
  local cli_fast_exit=0

  run_recent || recent_exit=$?

  echo ""

  run_api_fast || api_fast_exit=$?

  echo ""

  run_cli_fast || cli_fast_exit=$?

  echo ""
  if [ "${recent_exit}" -ne 0 ] || [ "${api_fast_exit}" -ne 0 ] || [ "${cli_fast_exit}" -ne 0 ]; then
    echo "  ${ERROR}PR E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: recent=${recent_exit}, api-fast=${api_fast_exit}, cli-fast=${cli_fast_exit}${NC}"
    return 1
  fi
  echo "  ${SUCCESS}PR E2E suites passed${NC}"
  return 0
}

run_release() {
  local api_exit=0
  local cli_exit=0
  local extended_exit=0

  run_full_api || api_exit=$?

  echo ""

  run_full_cli || cli_exit=$?

  echo ""

  run_full_extended || extended_exit=$?

  echo ""
  if [ "${api_exit}" -ne 0 ] || [ "${cli_exit}" -ne 0 ] || [ "${extended_exit}" -ne 0 ]; then
    echo "  ${ERROR}Some E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: full-api=${api_exit}, full-cli=${cli_exit}, full-extended=${extended_exit}${NC}"
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
  recent)
    run_recent
    ;;
  api-fast)
    run_api_fast
    ;;
  cli-fast)
    run_cli_fast
    ;;
  full-api|curl)
    run_full_api
    ;;
  full-cli|cli)
    run_full_cli
    ;;
  full-extended)
    run_full_extended
    ;;
  orchestrator)
    run_orchestrator
    ;;
  release|all)
    run_release
    ;;
  *)
    echo "Unknown E2E suite: ${suite}" >&2
    echo "Available suites: pr, recent, api-fast, cli-fast, full-api, full-cli, full-extended, orchestrator, release" >&2
    exit 1
    ;;
esac
