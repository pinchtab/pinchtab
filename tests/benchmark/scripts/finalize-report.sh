#!/usr/bin/env bash
#
# Renders a short markdown summary for the latest benchmark JSON report.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="${SCRIPT_DIR}/.."
RESULTS_DIR="${BENCH_DIR}/results"
CURRENT_BASELINE_PTR="${RESULTS_DIR}/current_baseline_report.txt"
CURRENT_AGENT_PTR="${RESULTS_DIR}/current_agent_report.txt"
CURRENT_AGENT_BROWSER_PTR="${RESULTS_DIR}/current_agent_browser_report.txt"

resolve_current_report() {
  local ptr="$1"
  if [[ -f "${ptr}" ]]; then
    tr -d '[:space:]' < "${ptr}"
    return 0
  fi
  return 1
}

REPORT_FILE="${1:-}"
if [[ -z "${REPORT_FILE}" ]]; then
  REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_BROWSER_PTR}" || true)"
  [[ -n "${REPORT_FILE}" ]] || REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_PTR}" || true)"
  [[ -n "${REPORT_FILE}" ]] || REPORT_FILE="$(resolve_current_report "${CURRENT_BASELINE_PTR}" || true)"
  if [[ -z "${REPORT_FILE}" ]]; then
    shopt -s nullglob
    candidates=(
      "${RESULTS_DIR}"/agent_browser_benchmark_*.json
      "${RESULTS_DIR}"/agent_benchmark_*.json
      "${RESULTS_DIR}"/baseline_*.json
    )
    shopt -u nullglob

    if [[ ${#candidates[@]} -gt 0 ]]; then
      REPORT_FILE=$(ls -t "${candidates[@]}" | head -1)
    fi
  fi
fi

if [[ -z "${REPORT_FILE}" || ! -f "${REPORT_FILE}" ]]; then
  echo "ERROR: no benchmark report found"
  exit 1
fi

# Auto-verify pending agent answers before rendering the summary.
# This ensures the "Verification Pass Rate" in the summary reflects actual
# grading, not just the answer count. For baseline reports (which use
# pass/fail directly) this is a no-op.
PENDING=$(jq -r '.totals.steps_pending_verification // 0' "${REPORT_FILE}")
if [[ "${PENDING}" -gt 0 ]]; then
  VERIFY_SCRIPT="${SCRIPT_DIR}/verify-answers.sh"
  if [[ -x "${VERIFY_SCRIPT}" ]]; then
    echo "Auto-verifying ${PENDING} pending answers..."
    "${VERIFY_SCRIPT}" "${REPORT_FILE}"
  else
    echo "WARNING: ${PENDING} answers pending verification but verify-answers.sh not found"
  fi
fi

SUMMARY_FILE="${REPORT_FILE%.json}_summary.md"

jq -r '
  def pct($a; $b):
    if $b == 0 then "0.0%" else (((1000 * $a) / $b | round) / 10 | tostring) + "%" end;
  . as $root |
  ($root.totals.steps_passed + $root.totals.steps_failed + $root.totals.steps_skipped) as $baseline_total |
  (($root.totals.steps_answered // 0) + $root.totals.steps_failed + $root.totals.steps_skipped) as $agent_total |
  ($root.totals.steps_verified_passed // 0) as $verified_passed |
  ($root.totals.steps_verified_failed // 0) as $verified_failed |
  ($root.totals.steps_verified_skipped // 0) as $verified_skipped |
  ($root.totals.steps_pending_verification // 0) as $pending |
  (if $root.benchmark.type == "baseline" then
    [
      "# Benchmark Summary",
      "",
      "| Metric | Value |",
      "|--------|-------|",
      "| Type | \($root.benchmark.type) |",
      "| Model | \($root.benchmark.model) |",
      "| Steps Passed | \($root.totals.steps_passed) |",
      "| Steps Failed | \($root.totals.steps_failed) |",
      "| Steps Skipped | \($root.totals.steps_skipped) |",
      "| Pass Rate | \(pct($root.totals.steps_passed; $baseline_total)) |",
      "| Tool Calls | \($root.totals.tool_calls // 0) |"
    ]
  else
    [
      "# Benchmark Summary",
      "",
      "| Metric | Value |",
      "|--------|-------|",
      "| Type | \($root.benchmark.type) |",
      "| Model | \($root.benchmark.model) |",
      "| Steps Answered | \($root.totals.steps_answered // 0) |",
      "| Steps Failed | \($root.totals.steps_failed) |",
      "| Steps Skipped | \($root.totals.steps_skipped) |",
      "| Answer Rate | \(pct($root.totals.steps_answered // 0; $agent_total)) |",
      "| Verified Passed | \($verified_passed) |",
      "| Verified Failed | \($verified_failed) |",
      "| Verified Skipped | \($verified_skipped) |",
      "| Pending Verification | \($pending) |",
      "| Verification Pass Rate | \(pct($verified_passed; $agent_total)) |",
      "| Tool Calls | \($root.totals.tool_calls // 0) |"
    ]
  end)[],
  "",
  "## Pending Verification",
  "",
  (
    [ $root.steps[] | select(.status == "answer" and ((.verification.status // "pending") == "pending")) | "- \(.id): \(.answer // .notes // "")" ] |
    if length == 0 then ["- none"] else . end
  )[],
  "",
  "## Failed Steps",
  "",
  (
    [ $root.steps[] | select(.status == "fail") | "- \(.id): \(.notes)" ] |
    if length == 0 then ["- none"] else . end
  )[],
  "",
  "## Verification Failures",
  "",
  (
    [ $root.steps[] | select(.status == "answer" and ((.verification.status // "") == "fail")) | "- \(.id): \(.verification.notes // "")" ] |
    if length == 0 then ["- none"] else . end
  )[]
' "${REPORT_FILE}" > "${SUMMARY_FILE}"

echo "Wrote ${SUMMARY_FILE}"
