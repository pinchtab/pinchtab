#!/bin/bash
#
# Record a benchmark step result
#
# Usage:
#   ./record-step.sh [--type baseline|agent|agent-browser] <group> <step> <pass|fail|skip|answer> [answer] [notes]
#
# Options:
#   --type baseline|agent|agent-browser   Report type (default: auto-detect most recent)
#   --tokens <in> <out>     Deprecated and ignored (kept for compatibility)
#   --bytes <n>             HTTP response size in bytes (baseline runs)
#   --tool-calls <n>        Override auto-counted tool calls
#   --observed <text>       Deprecated alias for answer text
#   --answer <text>         Optional explicit answer text
#
# Examples:
#   ./record-step.sh 1 1 pass "Navigation completed"
#   ./record-step.sh --type agent 2 3 fail "Element not found"
#   ./record-step.sh --type agent 2 4 answer "Robert Griesemer, 2009" "read infobox"
#   ./record-step.sh --type baseline 1 2 pass --bytes 4520 "Snapshot returned"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/../results"
mkdir -p "${RESULTS_DIR}"

# Parse flags
REPORT_TYPE=""
REPORT_FILE=""
INPUT_TOKENS=0
OUTPUT_TOKENS=0
RESPONSE_BYTES=0
TOOL_CALLS=""
ANSWER=""
AGENT_BROWSER_LOG="${RESULTS_DIR}/agent_browser_commands.ndjson"
CURRENT_BASELINE_PTR="${RESULTS_DIR}/current_baseline_report.txt"
CURRENT_AGENT_PTR="${RESULTS_DIR}/current_agent_report.txt"
CURRENT_AGENT_BROWSER_PTR="${RESULTS_DIR}/current_agent_browser_report.txt"

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
        --tokens)
            INPUT_TOKENS="$2"
            OUTPUT_TOKENS="$3"
            shift 3
            ;;
        --bytes)
            RESPONSE_BYTES="$2"
            shift 2
            ;;
        --tool-calls)
            TOOL_CALLS="$2"
            shift 2
            ;;
        --observed)
            ANSWER="$2"
            shift 2
            ;;
        --answer)
            ANSWER="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [[ $# -lt 3 ]]; then
    echo "Usage: $0 [--type baseline|agent|agent-browser] [--report-file <path>] <group> <step> <pass|fail|skip|answer> [answer] [notes]"
    exit 1
fi

GROUP="$1"
STEP="$2"
STATUS="$3"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
TOTAL_TOKENS=$((INPUT_TOKENS + OUTPUT_TOKENS))

case "${STATUS}" in
    pass|fail|skip|answer|observed) ;;
    *)
        echo "ERROR: status must be one of pass, fail, skip, answer"
        exit 1
        ;;
esac

if [[ "${STATUS}" == "observed" ]]; then
    STATUS="answer"
fi

shift 3

if [[ "${STATUS}" == "answer" ]]; then
    if [[ -z "${ANSWER}" && $# -ge 1 ]]; then
        ANSWER="$1"
        shift
    fi
    NOTES="${1:-}"
else
    NOTES="${1:-}"
fi

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
            baseline)
                REPORT_FILE="$(resolve_current_report "${CURRENT_BASELINE_PTR}" || true)"
                [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/baseline_*.json 2>/dev/null | head -1)
                ;;
            agent)
                REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_PTR}" || true)"
                [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_benchmark_*.json 2>/dev/null | head -1)
                ;;
            agent-browser|agent_browser)
                REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_BROWSER_PTR}" || true)"
                [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_browser_benchmark_*.json 2>/dev/null | head -1)
                ;;
            *)
                echo "ERROR: --type must be 'baseline', 'agent', or 'agent-browser'"
                exit 1
                ;;
        esac
    else
        REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_BROWSER_PTR}" || true)"
        [[ -n "${REPORT_FILE}" ]] || REPORT_FILE="$(resolve_current_report "${CURRENT_AGENT_PTR}" || true)"
        [[ -n "${REPORT_FILE}" ]] || REPORT_FILE="$(resolve_current_report "${CURRENT_BASELINE_PTR}" || true)"
        [[ -n "${REPORT_FILE}" ]] || REPORT_FILE=$(ls -t "${RESULTS_DIR}"/baseline_*.json "${RESULTS_DIR}"/agent_benchmark_*.json "${RESULTS_DIR}"/agent_browser_benchmark_*.json 2>/dev/null | head -1)
    fi
fi

if [[ -z "${REPORT_FILE}" || ! -f "${REPORT_FILE}" ]]; then
    echo "ERROR: No benchmark report found. Run ./run-optimization.sh first."
    exit 1
fi

BENCHMARK_TYPE=$(jq -r '.benchmark.type' "${REPORT_FILE}")

case "${BENCHMARK_TYPE}" in
    baseline)
        case "${STATUS}" in
            pass|fail|skip) ;;
            *)
                echo "ERROR: baseline steps must use pass, fail, or skip"
                exit 1
                ;;
        esac
        ;;
    agent|agent-browser)
        case "${STATUS}" in
            answer|fail|skip) ;;
            *)
                echo "ERROR: agent steps must use answer, fail, or skip"
                exit 1
                ;;
        esac
        ;;
esac

# Optional token metadata for runners that can provide trustworthy numbers.
COST=0
if [[ ${TOTAL_TOKENS} -gt 0 ]]; then
    MODEL=$(jq -r '.benchmark.model // "unknown"' "${REPORT_FILE}")

    case "${MODEL}" in
        *haiku*) INPUT_RATE=0.25; OUTPUT_RATE=1.25 ;;
        *sonnet*) INPUT_RATE=3.0; OUTPUT_RATE=15.0 ;;
        *opus*) INPUT_RATE=15.0; OUTPUT_RATE=75.0 ;;
        *gpt-4o-mini*) INPUT_RATE=0.15; OUTPUT_RATE=0.60 ;;
        *gpt-4o*) INPUT_RATE=2.50; OUTPUT_RATE=10.0 ;;
        *gpt-4*) INPUT_RATE=10.0; OUTPUT_RATE=30.0 ;;
        *gemini*flash*) INPUT_RATE=0.075; OUTPUT_RATE=0.30 ;;
        *gemini*pro*) INPUT_RATE=1.25; OUTPUT_RATE=5.0 ;;
        *) INPUT_RATE=1.0; OUTPUT_RATE=3.0 ;;
    esac

    COST=$(echo "scale=6; (${INPUT_TOKENS} / 1000000 * ${INPUT_RATE}) + (${OUTPUT_TOKENS} / 1000000 * ${OUTPUT_RATE})" | bc)
fi

STEP_TOOL_CALLS=0
if [[ -n "${TOOL_CALLS}" ]]; then
    STEP_TOOL_CALLS="${TOOL_CALLS}"
elif [[ "${BENCHMARK_TYPE}" == "agent-browser" ]]; then
    CURRENT_TOOL_CALLS=0
    PREV_TOOL_CALLS=$(jq -r '.totals.tool_calls // 0' "${REPORT_FILE}")
    if [[ -f "${AGENT_BROWSER_LOG}" ]]; then
        CURRENT_TOOL_CALLS=$(jq -Rsc 'split("\n") | map(select(length > 0) | try fromjson catch empty) | length' "${AGENT_BROWSER_LOG}")
    fi
    if [[ "${CURRENT_TOOL_CALLS}" -ge "${PREV_TOOL_CALLS}" ]]; then
        STEP_TOOL_CALLS=$((CURRENT_TOOL_CALLS - PREV_TOOL_CALLS))
    fi
fi

# Create step entry — only include metric fields when they have real values.
# Zero-value tokens/cost/bytes/tool_calls are omitted to reduce noise.
STEP_JSON=$(jq -n \
    --argjson group "${GROUP}" \
    --argjson step "${STEP}" \
    --arg id "${GROUP}.${STEP}" \
    --arg status "${STATUS}" \
    --argjson in_tokens "${INPUT_TOKENS}" \
    --argjson out_tokens "${OUTPUT_TOKENS}" \
    --argjson total_tokens "${TOTAL_TOKENS}" \
    --argjson tool_calls "${STEP_TOOL_CALLS}" \
    --argjson cost "${COST}" \
    --argjson bytes "${RESPONSE_BYTES}" \
    --arg answer "${ANSWER}" \
    --arg notes "${NOTES}" \
    --arg ts "${TIMESTAMP}" \
    '{group: $group, step: $step, id: $id, status: $status, timestamp: $ts} |
     if ($answer | length) > 0 then .answer = $answer else . end |
     if ($notes | length) > 0 then .notes = $notes else . end |
     if $in_tokens > 0 then .input_tokens = $in_tokens else . end |
     if $out_tokens > 0 then .output_tokens = $out_tokens else . end |
     if $total_tokens > 0 then .total_tokens = $total_tokens else . end |
     if $tool_calls > 0 then .tool_calls = $tool_calls else . end |
     if $cost > 0 then .cost_usd = $cost else . end |
     if $bytes > 0 then .response_bytes = $bytes else . end |
     if $status == "answer" then .verification = {status: "pending"} else . end')

# Append to report and update totals
TMP_FILE=$(mktemp)
jq --argjson step "${STEP_JSON}" \
   --argjson in "${INPUT_TOKENS}" \
   --argjson out "${OUTPUT_TOKENS}" \
   --argjson tool_calls "${STEP_TOOL_CALLS}" \
   --argjson cost "${COST}" \
   '.steps += [$step] |
    .totals.input_tokens = ((.totals.input_tokens // 0) + $in) |
    .totals.output_tokens = ((.totals.output_tokens // 0) + $out) |
    .totals.total_tokens = ((.totals.total_tokens // 0) + $in + $out) |
    .totals.tool_calls = ((.totals.tool_calls // 0) + $tool_calls) |
    .totals.estimated_cost_usd = ((.totals.estimated_cost_usd // 0) + $cost) |
    .totals.steps_passed = ([.steps[] | select(.status == "pass")] | length) |
    .totals.steps_failed = ([.steps[] | select(.status == "fail")] | length) |
    .totals.steps_skipped = ([.steps[] | select(.status == "skip")] | length) |
    .totals.steps_answered = ([.steps[] | select(.status == "answer")] | length) |
    .totals.steps_verified_passed = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "pass"))] | length) |
    .totals.steps_verified_failed = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "fail"))] | length) |
    .totals.steps_verified_skipped = ([.steps[] | select(.status == "answer" and ((.verification.status // "") == "skip"))] | length) |
    .totals.steps_pending_verification = ([.steps[] | select(.status == "answer" and ((.verification.status // "pending") == "pending"))] | length)' \
   "${REPORT_FILE}" > "${TMP_FILE}"

mv "${TMP_FILE}" "${REPORT_FILE}"

# Log failures
if [[ "${STATUS}" == "fail" ]]; then
    echo "[${TIMESTAMP}] Step ${GROUP}.${STEP} FAILED: ${NOTES}" >> "${RESULTS_DIR}/errors.log"
fi

echo "Recorded: Step ${GROUP}.${STEP} = ${STATUS} (bytes: ${RESPONSE_BYTES}, tool_calls: ${STEP_TOOL_CALLS})"
