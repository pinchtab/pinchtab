#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;230;57;70m'
NC=$'\033[0m'

# Parse arguments after suite
E2E_FILTER=""
E2E_EXTRA=""
E2E_LOGS="${E2E_LOGS:-show}"
for arg in "${@:2}"; do
  case "$arg" in
    filter=*)
      E2E_FILTER="${arg#filter=}"
      ;;
    extra=*)
      E2E_EXTRA="${arg#extra=}"
      ;;
    logs=hide|logs=quiet|quiet=true)
      E2E_LOGS="hide"
      ;;
    logs=show|quiet=false)
      E2E_LOGS="show"
      ;;
    logs=*)
      echo "Unknown logs mode: ${arg#logs=}. Use logs=show or logs=hide." >&2
      exit 1
      ;;
    quiet=*)
      echo "Unknown quiet mode: ${arg#quiet=}. Use quiet=true or quiet=false." >&2
      exit 1
      ;;
    *)
      # Backwards compatibility: treat bare argument as filter
      if [ -z "$E2E_FILTER" ]; then
        E2E_FILTER="$arg"
      fi
      ;;
  esac
done


show_filter_status() {
  if [ -n "${E2E_FILTER}" ]; then
    echo "  ${MUTED}filter: ${E2E_FILTER}${NC}"
    return
  fi

  echo "  ${MUTED}filter: none (running all scenarios in this suite)${NC}"
}

show_logs_status() {
  if [ "${E2E_LOGS}" = "hide" ]; then
    echo "  ${MUTED}logs: hidden (showing only failure summary)${NC}"
    return
  fi

  echo "  ${MUTED}logs: streaming${NC}"
}

# Detect available docker compose command
COMPOSE="docker compose"
if [ -n "${PINCHTAB_COMPOSE:-}" ]; then
  COMPOSE="${PINCHTAB_COMPOSE}"
elif docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  echo "Neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 127
fi

compose() {
  $COMPOSE "$@"
}

now_ms() {
  python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
}

format_duration_ms() {
  local ms="$1"
  if [ "$ms" -lt 1000 ]; then
    printf '%sms' "$ms"
    return
  fi

  local seconds=$((ms / 1000))
  local rem_ms=$((ms % 1000))
  if [ "$seconds" -lt 60 ]; then
    printf '%s.%03ss' "$seconds" "$rem_ms"
    return
  fi

  local minutes=$((seconds / 60))
  local rem_sec=$((seconds % 60))
  printf '%sm%02d.%03ds' "$minutes" "$rem_sec" "$rem_ms"
}

compose_down() {
  local compose_file="$1"
  if [ "${E2E_LOGS}" = "hide" ]; then
    compose -f "${compose_file}" down -v >/dev/null 2>&1 || true
    return
  fi

  compose -f "${compose_file}" down -v 2>/dev/null || true
}

# Always rebuild the fixtures nginx image up front so changes to
# tests/e2e/nginx/{Dockerfile,default.conf} are guaranteed to land in the
# container, regardless of whether `compose run --build` would otherwise
# rebuild this transitive dependency. This eliminates an entire class of
# "the new fixture config didn't get picked up" failure modes.
build_support_images() {
  local compose_file="$1"
  compose -f "${compose_file}" build fixtures
}

run_logged_command() {
  local output_file="$1"
  local progress_file="$2"
  local phase_label="$3"
  shift 3

  if [ "${E2E_LOGS}" = "hide" ]; then
    mkdir -p "$(dirname "${output_file}")"
    echo "  ${MUTED}${phase_label}...${NC}"
    "$@" >> "${output_file}" 2>&1 &
    local command_pid=$!
    local last_progress=""
    local heartbeat=0

    while kill -0 "${command_pid}" 2>/dev/null; do
      if [ -n "${progress_file}" ] && [ -f "${progress_file}" ]; then
        local progress_line
        progress_line=$(tail -n 1 "${progress_file}" 2>/dev/null || true)
        if [ -n "${progress_line}" ] && [ "${progress_line}" != "${last_progress}" ]; then
          last_progress="${progress_line}"
          local progress_state progress_name
          progress_state=$(printf '%s\n' "${progress_line}" | awk '{print $2}')
          progress_name=$(printf '%s\n' "${progress_line}" | cut -d' ' -f3-)
          case "${progress_state}" in
            RUNNING)
              echo "  ${MUTED}running: ${progress_name%.sh}${NC}"
              ;;
          esac
          heartbeat=0
        fi
      fi

      heartbeat=$((heartbeat + 1))
      if [ "${heartbeat}" -ge 5 ]; then
        echo "  ${MUTED}${phase_label}...${NC}"
        heartbeat=0
      fi
      sleep 2
    done

    wait "${command_pid}"
    local command_exit=$?
    if [ "${command_exit}" -eq 0 ]; then
      echo "  ${MUTED}${phase_label}: done${NC}"
    fi
    return "${command_exit}"
  fi

  "$@"
}

dump_compose_failure() {
  local compose_file="$1"
  shift
  local log_prefix="$1"
  shift
  local services=("$@")

  mkdir -p tests/e2e/results
  for service in "${services[@]}"; do
    compose -f "${compose_file}" logs "${service}" > "tests/e2e/results/${log_prefix}-${service}.log" 2>&1 || true
  done
}

show_suite_artifacts() {
  local summary_file="$1"
  local report_file="$2"
  local progress_file="$3"
  local log_prefix="$4"
  local output_file="$5"
  local suite_duration_ms="$6"
  shift 6
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

  if [ -f "${output_file}" ]; then
    echo "  ${MUTED}Captured runner output: ${output_file}${NC}"
    printed=1
  fi

  if [ -n "${suite_duration_ms}" ]; then
    echo "  ${MUTED}Suite wall time: $(format_duration_ms "${suite_duration_ms}")${NC}"
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

show_failure_summary() {
  local report_file="$1"
  local output_file="$2"
  local failed_tests

  [ "${E2E_LOGS}" = "hide" ] || return 0

  if [ -f "${report_file}" ]; then
    failed_tests=$(awk -F'|' '/\| ❌ \|/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' "${report_file}")
    if [ -n "${failed_tests}" ]; then
      echo ""
      echo "  ${ERROR}${BOLD}Failed tests${NC}"
      while IFS= read -r test_name; do
        [ -n "${test_name}" ] || continue
        echo "  ${ERROR}- ${test_name}${NC}"
      done <<< "${failed_tests}"
      echo ""
    fi
  fi

  if [ -f "${output_file}" ] && [ ! -f "${report_file}" ]; then
    echo ""
    echo "  ${ERROR}${BOLD}Suite failed before report generation; recent output:${NC}"
    tail -n 40 "${output_file}"
    echo ""
  fi
}

prepare_suite_results() {
  local summary_file="$1"
  local report_file="$2"
  local progress_file="$3"
  local log_prefix="$4"
  local output_file="$5"

  rm -f \
    "${summary_file}" \
    "${report_file}" \
    "${progress_file}" \
    "${output_file}" \
    tests/e2e/results/${log_prefix}-*.log \
    tests/e2e/results/summary.txt \
    tests/e2e/results/report.md
}

record_suite_duration() {
  local summary_file="$1"
  local report_file="$2"
  local suite_duration_ms="$3"

  [ -n "${suite_duration_ms}" ] || return 0

  if [ -f "${summary_file}" ]; then
    append_text_file "${summary_file}" "suite_wall_time=${suite_duration_ms}ms"$'\n'
  fi

  if [ -f "${report_file}" ]; then
    append_text_file "${report_file}" "$(printf '\n**Suite Wall Time:** %s\n' "$(format_duration_ms "${suite_duration_ms}")")"
  fi
}

append_text_file() {
  local target_file="$1"
  local content="$2"
  local target_dir tmp_file

  if [ -w "${target_file}" ]; then
    printf '%s' "${content}" >> "${target_file}"
    return
  fi

  target_dir=$(dirname "${target_file}")
  tmp_file=$(mktemp "${target_dir}/.append.XXXXXX")
  cat "${target_file}" > "${tmp_file}"
  printf '%s' "${content}" >> "${tmp_file}"
  mv "${tmp_file}" "${target_file}"
}

suite_filter_matches() {
  local group_dir="$1"
  local include_extended="$2"
  local script_name
  local extra
  local name

  if [ -z "${E2E_FILTER}" ]; then
    return 0
  fi

  for basic_path in "${group_dir}"/*-basic.sh; do
    [ -f "${basic_path}" ] || continue
    script_name=$(basename "${basic_path}")
    [[ "${script_name}" == *"${E2E_FILTER}"* ]] && return 0
  done

  if [ "${include_extended}" = "true" ]; then
    for extended_path in "${group_dir}"/*-extended.sh; do
      [ -f "${extended_path}" ] || continue
      script_name=$(basename "${extended_path}")
      [[ "${script_name}" == *"${E2E_FILTER}"* ]] && return 0
    done

    for standalone in "${group_dir}"/*.sh; do
      [ -f "${standalone}" ] || continue
      name=$(basename "${standalone}")
      if [[ "${name}" != *-basic.sh && "${name}" != *-extended.sh && "${name}" == *"${E2E_FILTER}"* ]]; then
        return 0
      fi
    done
  fi

  if [ -n "${E2E_EXTRA}" ]; then
    for extra in ${E2E_EXTRA}; do
      name=$(basename "${extra}")
      if [ -f "${group_dir}/${name}" ] && [[ "${name}" == *"${E2E_FILTER}"* ]]; then
        return 0
      fi
    done
  fi

  return 1
}

show_suite_skip() {
  local suite_label="$1"
  echo "  ${MUTED}Skipping ${suite_label}: filter '${E2E_FILTER}' has no matching scenarios${NC}"
}

run_api() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-api.txt"
  local report_file="tests/e2e/results/report-api.md"
  local progress_file="tests/e2e/results/progress-api.log"
  local log_prefix="logs-api"
  local output_file="tests/e2e/results/output-api.log"
  echo "  ${ACCENT}${BOLD}E2E API tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local api_exit=$?
  local -a args=()
  [ -n "${E2E_FILTER}" ] && args+=("filter=${E2E_FILTER}")
  [ -n "${E2E_EXTRA}" ] && args+=("extra=${E2E_EXTRA}")
  if [ "${api_exit}" -eq 0 ]; then
    run_logged_command "${output_file}" "${progress_file}" "running api suite" compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh api "${args[@]}"
    api_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${api_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-api pinchtab
  fi
  compose_down "${compose_file}"
  return "${api_exit}"
}

run_api_extended() {
  local compose_file="tests/e2e/docker-compose-multi.yml"
  local summary_file="tests/e2e/results/summary-api-extended.txt"
  local report_file="tests/e2e/results/report-api-extended.md"
  local progress_file="tests/e2e/results/progress-api-extended.log"
  local log_prefix="logs-api-extended"
  local output_file="tests/e2e/results/output-api-extended.log"
  echo "  ${ACCENT}${BOLD}E2E API Extended tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local api_exit=$?
  if [ "${api_exit}" -eq 0 ]; then
    E2E_SUITE=api E2E_EXTENDED=true E2E_SCENARIO_FILTER="${E2E_FILTER}" run_logged_command "${output_file}" "${progress_file}" "running api extended suite" compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-api runner-api
    api_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${api_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
  fi
  compose_down "${compose_file}"
  return "${api_exit}"
}

run_cli() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli.txt"
  local report_file="tests/e2e/results/report-cli.md"
  local progress_file="tests/e2e/results/progress-cli.log"
  local log_prefix="logs-cli"
  local output_file="tests/e2e/results/output-cli.log"
  echo "  ${ACCENT}${BOLD}E2E CLI tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local cli_exit=$?
  local -a args=()
  [ -n "${E2E_FILTER}" ] && args+=("filter=${E2E_FILTER}")
  [ -n "${E2E_EXTRA}" ] && args+=("extra=${E2E_EXTRA}")
  if [ "${cli_exit}" -eq 0 ]; then
    run_logged_command "${output_file}" "${progress_file}" "running cli suite" compose -f "${compose_file}" run --build --rm runner-cli /bin/bash /e2e/run.sh cli "${args[@]}"
    cli_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${cli_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-cli pinchtab
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-cli pinchtab
  fi
  compose_down "${compose_file}"
  return "${cli_exit}"
}

run_cli_extended() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-cli-extended.txt"
  local report_file="tests/e2e/results/report-cli-extended.md"
  local progress_file="tests/e2e/results/progress-cli-extended.log"
  local log_prefix="logs-cli-extended"
  local output_file="tests/e2e/results/output-cli-extended.log"
  echo "  ${ACCENT}${BOLD}E2E CLI Extended tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local cli_exit=$?
  if [ "${cli_exit}" -eq 0 ]; then
    E2E_SUITE=cli E2E_EXTENDED=true E2E_SCENARIO_FILTER="${E2E_FILTER}" run_logged_command "${output_file}" "${progress_file}" "running cli extended suite" compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-cli runner-cli
    cli_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${cli_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-cli pinchtab
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-cli pinchtab
  fi
  compose_down "${compose_file}"
  return "${cli_exit}"
}

run_infra() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-infra.txt"
  local report_file="tests/e2e/results/report-infra.md"
  local progress_file="tests/e2e/results/progress-infra.log"
  local log_prefix="logs-infra"
  local output_file="tests/e2e/results/output-infra.log"
  echo "  ${ACCENT}${BOLD}E2E Infra tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local infra_exit=$?
  local -a args=()
  [ -n "${E2E_FILTER}" ] && args+=("filter=${E2E_FILTER}")
  [ -n "${E2E_EXTRA}" ] && args+=("extra=${E2E_EXTRA}")
  if [ "${infra_exit}" -eq 0 ]; then
    run_logged_command "${output_file}" "${progress_file}" "running infra suite" compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh infra "${args[@]}"
    infra_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${infra_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-api pinchtab
  fi
  compose_down "${compose_file}"
  return "${infra_exit}"
}

run_infra_extended() {
  local compose_file="tests/e2e/docker-compose-multi.yml"
  local summary_file="tests/e2e/results/summary-infra-extended.txt"
  local report_file="tests/e2e/results/report-infra-extended.md"
  local progress_file="tests/e2e/results/progress-infra-extended.log"
  local log_prefix="logs-infra-extended"
  local output_file="tests/e2e/results/output-infra-extended.log"
  echo "  ${ACCENT}${BOLD}E2E Infra Extended tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local infra_exit=$?
  if [ "${infra_exit}" -eq 0 ]; then
    E2E_SUITE=infra E2E_EXTENDED=true E2E_SCENARIO_FILTER="${E2E_FILTER}" run_logged_command "${output_file}" "${progress_file}" "running infra extended suite" compose -f "${compose_file}" up --build --abort-on-container-exit --exit-code-from runner-api runner-api
    infra_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${infra_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-api pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-lite pinchtab-bridge
  fi
  compose_down "${compose_file}"
  return "${infra_exit}"
}

run_pr() {
  local api_exit=0
  local cli_exit=0
  local infra_exit=0
  local ran_any=0

  if suite_filter_matches "tests/e2e/scenarios/api" false; then
    ran_any=1
    run_api || api_exit=$?
  else
    show_suite_skip "api"
  fi

  echo ""

  if suite_filter_matches "tests/e2e/scenarios/cli" false; then
    ran_any=1
    run_cli || cli_exit=$?
  else
    show_suite_skip "cli"
  fi

  echo ""

  if suite_filter_matches "tests/e2e/scenarios/infra" false; then
    ran_any=1
    run_infra || infra_exit=$?
  else
    show_suite_skip "infra"
  fi

  echo ""
  if [ "${ran_any}" -eq 0 ]; then
    echo "  ${ERROR}No PR E2E suites matched filter '${E2E_FILTER}'${NC}"
    return 1
  fi
  if [ "${api_exit}" -ne 0 ] || [ "${cli_exit}" -ne 0 ] || [ "${infra_exit}" -ne 0 ]; then
    echo "  ${ERROR}PR E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: api=${api_exit}, cli=${cli_exit}, infra=${infra_exit}${NC}"
    return 1
  fi
  echo "  ${SUCCESS}PR E2E suites passed${NC}"
  return 0
}

run_plugin() {
  local compose_file="tests/e2e/docker-compose.yml"
  local summary_file="tests/e2e/results/summary-plugin.txt"
  local report_file="tests/e2e/results/report-plugin.md"
  local progress_file="tests/e2e/results/progress-plugin.log"
  local log_prefix="logs-plugin"
  local output_file="tests/e2e/results/output-plugin.log"
  echo "  ${ACCENT}${BOLD}E2E Plugin tests (Docker)${NC}"
  show_filter_status
  show_logs_status
  echo ""
  prepare_suite_results "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}"
  local suite_started_at
  suite_started_at=$(now_ms)
  set +e
  run_logged_command "${output_file}" "" "building support images" build_support_images "${compose_file}"
  local plugin_exit=$?
  local -a args=()
  [ -n "${E2E_FILTER}" ] && args+=("filter=${E2E_FILTER}")
  if [ "${plugin_exit}" -eq 0 ]; then
    run_logged_command "${output_file}" "${progress_file}" "running plugin suite" compose -f "${compose_file}" run --build --rm runner-api /bin/bash /e2e/run.sh plugin "${args[@]}"
    plugin_exit=$?
  fi
  set -e
  local suite_duration_ms=$(( $(now_ms) - suite_started_at ))
  record_suite_duration "${summary_file}" "${report_file}" "${suite_duration_ms}"
  if [ "${plugin_exit}" -ne 0 ]; then
    dump_compose_failure "${compose_file}" "${log_prefix}" runner-api pinchtab
    show_failure_summary "${report_file}" "${output_file}"
    show_suite_artifacts "${summary_file}" "${report_file}" "${progress_file}" "${log_prefix}" "${output_file}" "${suite_duration_ms}" runner-api pinchtab
  fi
  compose_down "${compose_file}"
  return "${plugin_exit}"
}

run_release() {
  local api_exit=0
  local cli_exit=0
  local infra_exit=0
  local plugin_exit=0
  local ran_any=0

  if suite_filter_matches "tests/e2e/scenarios/api" true; then
    ran_any=1
    run_api_extended || api_exit=$?
  else
    show_suite_skip "api-extended"
  fi

  echo ""

  if suite_filter_matches "tests/e2e/scenarios/cli" true; then
    ran_any=1
    run_cli_extended || cli_exit=$?
  else
    show_suite_skip "cli-extended"
  fi

  echo ""

  if suite_filter_matches "tests/e2e/scenarios/infra" true; then
    ran_any=1
    run_infra_extended || infra_exit=$?
  else
    show_suite_skip "infra-extended"
  fi

  echo ""

  # Plugin smoke tests (always run in release suite)
  ran_any=1
  run_plugin || plugin_exit=$?

  echo ""
  if [ "${ran_any}" -eq 0 ]; then
    echo "  ${ERROR}No E2E suites matched filter '${E2E_FILTER}'${NC}"
    return 1
  fi
  if [ "${api_exit}" -ne 0 ] || [ "${cli_exit}" -ne 0 ] || [ "${infra_exit}" -ne 0 ] || [ "${plugin_exit}" -ne 0 ]; then
    echo "  ${ERROR}Some E2E suites failed${NC}"
    echo "  ${MUTED}exit codes: api-extended=${api_exit}, cli-extended=${cli_exit}, infra-extended=${infra_exit}, plugin=${plugin_exit}${NC}"
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
  api)
    run_api
    ;;
  api-extended)
    run_api_extended
    ;;
  cli)
    run_cli
    ;;
  cli-extended)
    run_cli_extended
    ;;
  infra)
    run_infra
    ;;
  infra-extended)
    run_infra_extended
    ;;
  plugin)
    run_plugin
    ;;
  release|all)
    run_release
    ;;
  # Backwards compatibility aliases
  api-fast)
    run_api
    ;;
  cli-fast)
    run_cli
    ;;
  api-full|full-api|curl)
    run_api_extended
    ;;
  cli-full|full-cli)
    run_cli_extended
    ;;
  *)
    echo "Unknown E2E suite: ${suite}" >&2
    echo "Available suites: pr, api, cli, infra, plugin, api-extended, cli-extended, infra-extended, release" >&2
    exit 1
    ;;
esac
