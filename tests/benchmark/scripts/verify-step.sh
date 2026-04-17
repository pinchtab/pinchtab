#!/usr/bin/env bash
#
# Record a deferred verification decision for a previously observed benchmark
# step.
#
# Usage:
#   ./verify-step.sh [--type agent|agent-browser] [--report-file <path>] <group> <step> <pass|fail|skip> "notes"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/../results"
CURRENT_AGENT_PTR="${RESULTS_DIR}/current_agent_report.txt"
CURRENT_AGENT_BROWSER_PTR="${RESULTS_DIR}/current_agent_browser_report.txt"

REPORT_TYPE=""
REPORT_FILE=""

while [[ $# -gt 0 && "$1" == --* ]]; do
  case "$1" in
    --type)
      REPORT_TYPE="$2"
      shift 2
      ;;
    --report-file)
      REPORT_FILE="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [[ $# -lt 3 ]]; then
  echo "Usage: $0 [--type agent|agent-browser] [--report-file <path>] <group> <step> <pass|fail|skip> [notes]"
  exit 1
fi

GROUP="$1"
STEP="$2"
STATUS="$3"
NOTES="${4:-}"
STEP_ID="${GROUP}.${STEP}"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

case "${STATUS}" in
  pass|fail|skip) ;;
  *)
    echo "ERROR: verification status must be one of pass, fail, skip"
    exit 1
    ;;
esac

resolve_current_report() {
  local ptr="$1"
  if [[ -f "${ptr}" ]]; then
    tr -d '[:space:]' < "${ptr}"
    return 0
  fi
  return 1
}

if [[ -z "${REPORT_FILE}" ]]; then
  if [[ -n "${REPORT_TYPE}" ]]; then
    case "${REPORT_TYPE}" in
      agent)
        REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_PTR}" || true)"
        [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_benchmark_*.json 2>/dev/null | head -1)
        ;;
      agent-browser|agent_browser)
        REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_BROWSER_PTR}" || true)"
        [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_browser_benchmark_*.json 2>/dev/null | head -1)
        ;;
      *)
        echo "ERROR: --type must be 'agent' or 'agent-browser'"
        exit 1
        ;;
    esac
  else
    REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_BROWSER_PTR}" || true)"
    [[ -n "${REPORT_FILE}" ]] || REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_PTR}" || true)"
    [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_browser_benchmark_*.json "${RESULTS_DIR}"/agent_benchmark_*.json 2>/dev/null | head -1)
  fi
fi

if [[ -z "${REPORT_FILE:-}" || ! -f "${REPORT_FILE}" ]]; then
  echo "ERROR: no agent benchmark report found"
  exit 1
fi

TMP_FILE=$(mktemp)
jq --arg id "${STEP_ID}" \
   --arg status "${STATUS}" \
   --arg notes "${NOTES}" \
   --arg ts "${TIMESTAMP}" \
   '
   if any(.steps[]; .id == $id) | not then
     error("step not found: " + $id)
   elif any(.steps[]; .id == $id and .status == "answer") | not then
     error("step is not answer-status and cannot be verified: " + $id)
   else
     .
   end |
   .steps |= map(
     if .id == $id then
       .verification = {
         status: $status,
         notes: $notes,
         timestamp: $ts
       }
     else . end
   ) |
   .totals.steps_verified_passed = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "pass"))] | length) |
   .totals.steps_verified_failed = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "fail"))] | length) |
   .totals.steps_verified_skipped = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "skip"))] | length) |
   .totals.steps_pending_verification = ([.steps[] | select(.status == "answer" and ((.verification.status // "pending") == "pending"))] | length)
   ' "${REPORT_FILE}" > "${TMP_FILE}"

mv "${TMP_FILE}" "${REPORT_FILE}"
echo "Verified: Step ${STEP_ID} = ${STATUS}"
