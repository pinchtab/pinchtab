#!/bin/bash
#
# Record an agent benchmark step with commands executed
#
# Usage:
#   ./record-agent-step.sh <group> <step> <pass|fail> <in_tokens> <out_tokens> "commands" "notes"
#
# Example:
#   ./record-agent-step.sh 1 1 pass 150 45 "curl -X POST .../navigate" "Page loaded successfully"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"

# Find most recent agent report file
REPORT_FILE=$(ls -t "${RESULTS_DIR}"/agent_benchmark_*.json 2>/dev/null | head -1)

if [[ -z "${REPORT_FILE}" ]]; then
    echo "ERROR: No agent benchmark report found."
    exit 1
fi

if [[ $# -lt 7 ]]; then
    echo "Usage: $0 <group> <step> <pass|fail> <in_tokens> <out_tokens> \"commands\" \"notes\""
    exit 1
fi

GROUP="$1"
STEP="$2"
STATUS="$3"
INPUT_TOKENS="$4"
OUTPUT_TOKENS="$5"
COMMANDS="$6"
NOTES="$7"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
TOTAL_TOKENS=$((INPUT_TOKENS + OUTPUT_TOKENS))

# Cost per 1M tokens (Haiku default)
INPUT_RATE=0.25
OUTPUT_RATE=1.25

COST=$(echo "scale=6; (${INPUT_TOKENS} / 1000000 * ${INPUT_RATE}) + (${OUTPUT_TOKENS} / 1000000 * ${OUTPUT_RATE})" | bc)

# Escape quotes in commands and notes for JSON
COMMANDS_ESCAPED=$(echo "$COMMANDS" | sed 's/"/\\"/g' | tr '\n' ' ')
NOTES_ESCAPED=$(echo "$NOTES" | sed 's/"/\\"/g')

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
  "commands": "${COMMANDS_ESCAPED}",
  "notes": "${NOTES_ESCAPED}",
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

# Log commands to separate file for analysis
COMMANDS_LOG="${RESULTS_DIR}/agent_commands.log"
echo "[${TIMESTAMP}] Step ${GROUP}.${STEP}: ${COMMANDS_ESCAPED}" >> "${COMMANDS_LOG}"

echo "Recorded: Step ${GROUP}.${STEP} = ${STATUS} (${TOTAL_TOKENS} tokens)"
echo "  Commands: ${COMMANDS_ESCAPED:0:80}..."
