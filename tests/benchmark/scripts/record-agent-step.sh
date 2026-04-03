#!/bin/bash
#
# Record a benchmark step result
#
# Usage:
#   ./record-step.sh <group> <step> <pass|fail|skip> <input_tokens> <output_tokens> [notes]
#
# Example:
#   ./record-step.sh 1 1 pass 150 45 "Navigation completed"
#   ./record-step.sh 2 3 fail 200 80 "Element not found"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/../results"

# Find most recent report file
REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_benchmark_*.json 2>/dev/null | head -1)

if [[ -z "${REPORT_FILE}" ]]; then
    echo "ERROR: No benchmark report found. Run ./run-benchmark.sh first."
    exit 1
fi

if [[ $# -lt 5 ]]; then
    echo "Usage: $0 <group> <step> <pass|fail|skip> <input_tokens> <output_tokens> [notes]"
    exit 1
fi

GROUP="$1"
STEP="$2"
STATUS="$3"
INPUT_TOKENS="$4"
OUTPUT_TOKENS="$5"
NOTES="${6:-}"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
TOTAL_TOKENS=$((INPUT_TOKENS + OUTPUT_TOKENS))

# Calculate cost based on model (from report)
MODEL=$(jq -r '.benchmark.model' "${REPORT_FILE}")
PROVIDER=$(jq -r '.benchmark.provider' "${REPORT_FILE}")

# Cost per 1M tokens
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

# Calculate cost (using bc for floating point)
COST=$(echo "scale=6; (${INPUT_TOKENS} / 1000000 * ${INPUT_RATE}) + (${OUTPUT_TOKENS} / 1000000 * ${OUTPUT_RATE})" | bc)

# Create step entry
STEP_JSON=$(cat << EOF
{
  "group": ${GROUP},
  "step": ${STEP},
  "id": "${GROUP}.${STEP}",
  "status": "${STATUS}",
  "input_tokens": ${INPUT_TOKENS},
  "output_tokens": ${OUTPUT_TOKENS},
  "total_tokens": ${TOTAL_TOKENS},
  "cost_usd": ${COST},
  "notes": "${NOTES}",
  "timestamp": "${TIMESTAMP}"
}
EOF
)

# Append to report
TMP_FILE=$(mktemp)
jq --argjson step "${STEP_JSON}" '.steps += [$step]' "${REPORT_FILE}" > "${TMP_FILE}"

# Update totals
jq --argjson in "${INPUT_TOKENS}" --argjson out "${OUTPUT_TOKENS}" --argjson cost "${COST}" \
   --arg status "${STATUS}" \
   '.totals.input_tokens += $in |
    .totals.output_tokens += $out |
    .totals.total_tokens += ($in + $out) |
    .totals.estimated_cost_usd += $cost |
    if $status == "pass" then .totals.steps_passed += 1
    elif $status == "fail" then .totals.steps_failed += 1
    else .totals.steps_skipped += 1 end' \
   "${TMP_FILE}" > "${REPORT_FILE}"

rm -f "${TMP_FILE}"

# If failed, also log to error log with details
if [[ "${STATUS}" == "fail" ]]; then
    ERROR_LOG="${RESULTS_DIR}/errors.log"
    echo "[${TIMESTAMP}] Step ${GROUP}.${STEP} FAILED: ${NOTES}" >> "${ERROR_LOG}"
fi

echo "Recorded: Step ${GROUP}.${STEP} = ${STATUS} (${TOTAL_TOKENS} tokens, \$${COST})"
