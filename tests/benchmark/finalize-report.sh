#!/bin/bash
#
# Finalize benchmark report and generate summary
#
# Usage:
#   ./finalize-report.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"

# Find most recent report file
REPORT_FILE=$(ls -t "${RESULTS_DIR}"/benchmark_*.json 2>/dev/null | head -1)

if [[ -z "${REPORT_FILE}" ]]; then
    echo "ERROR: No benchmark report found."
    exit 1
fi

SUMMARY_FILE="${REPORT_FILE%.json}_summary.md"

echo "Finalizing report: ${REPORT_FILE}"

# Extract data
TIMESTAMP=$(jq -r '.benchmark.timestamp' "${REPORT_FILE}")
MODEL=$(jq -r '.benchmark.model' "${REPORT_FILE}")
PROVIDER=$(jq -r '.benchmark.provider' "${REPORT_FILE}")
VERSION=$(jq -r '.benchmark.pinchtab_version.version // "unknown"' "${REPORT_FILE}")

TOTAL_INPUT=$(jq -r '.totals.input_tokens' "${REPORT_FILE}")
TOTAL_OUTPUT=$(jq -r '.totals.output_tokens' "${REPORT_FILE}")
TOTAL_TOKENS=$(jq -r '.totals.total_tokens' "${REPORT_FILE}")
TOTAL_COST=$(jq -r '.totals.estimated_cost_usd' "${REPORT_FILE}")
PASSED=$(jq -r '.totals.steps_passed' "${REPORT_FILE}")
FAILED=$(jq -r '.totals.steps_failed' "${REPORT_FILE}")
SKIPPED=$(jq -r '.totals.steps_skipped' "${REPORT_FILE}")

TOTAL_STEPS=$((PASSED + FAILED + SKIPPED))
if [[ ${TOTAL_STEPS} -gt 0 ]]; then
    PASS_RATE=$(echo "scale=1; ${PASSED} * 100 / ${TOTAL_STEPS}" | bc)
else
    PASS_RATE="0"
fi

# Generate markdown summary
cat > "${SUMMARY_FILE}" << EOF
# PinchTab Benchmark Results

## Summary

| Metric | Value |
|--------|-------|
| **Date** | ${TIMESTAMP} |
| **Model** | ${MODEL} |
| **Provider** | ${PROVIDER} |
| **PinchTab Version** | ${VERSION} |

## Results

| Metric | Value |
|--------|-------|
| **Steps Passed** | ${PASSED} |
| **Steps Failed** | ${FAILED} |
| **Steps Skipped** | ${SKIPPED} |
| **Pass Rate** | ${PASS_RATE}% |

## Token Usage

| Metric | Value |
|--------|-------|
| **Input Tokens** | ${TOTAL_INPUT} |
| **Output Tokens** | ${TOTAL_OUTPUT} |
| **Total Tokens** | ${TOTAL_TOKENS} |
| **Estimated Cost** | \$${TOTAL_COST} |

## Steps by Group

EOF

# Group steps
jq -r '
  .steps | group_by(.group) | .[] |
  "### Group " + (.[0].group | tostring) + "\n\n" +
  "| Step | Status | Tokens | Cost | Notes |\n|------|--------|--------|------|-------|\n" +
  (map("| " + .id + " | " + .status + " | " + (.total_tokens | tostring) + " | $" + (.cost_usd | tostring) + " | " + .notes + " |") | join("\n")) +
  "\n"
' "${REPORT_FILE}" >> "${SUMMARY_FILE}"

# Add failed steps section
FAILED_STEPS=$(jq -r '.steps | map(select(.status == "fail"))' "${REPORT_FILE}")
FAILED_COUNT=$(echo "${FAILED_STEPS}" | jq 'length')

if [[ ${FAILED_COUNT} -gt 0 ]]; then
    cat >> "${SUMMARY_FILE}" << EOF

## Failed Steps

EOF
    echo "${FAILED_STEPS}" | jq -r '.[] | "- **" + .id + "**: " + .notes' >> "${SUMMARY_FILE}"
fi

# Cost breakdown by group
cat >> "${SUMMARY_FILE}" << EOF

## Cost by Group

| Group | Tokens | Cost |
|-------|--------|------|
EOF

jq -r '
  .steps | group_by(.group) | .[] |
  "| " + (.[0].group | tostring) + " | " + 
  (map(.total_tokens) | add | tostring) + " | $" + 
  (map(.cost_usd) | add | tostring) + " |"
' "${REPORT_FILE}" >> "${SUMMARY_FILE}"

echo ""
echo "=== Benchmark Complete ==="
echo ""
echo "Results:"
echo "  - Passed: ${PASSED}"
echo "  - Failed: ${FAILED}"
echo "  - Skipped: ${SKIPPED}"
echo "  - Pass Rate: ${PASS_RATE}%"
echo ""
echo "Tokens: ${TOTAL_TOKENS} (in: ${TOTAL_INPUT}, out: ${TOTAL_OUTPUT})"
echo "Cost: \$${TOTAL_COST}"
echo ""
echo "Report: ${REPORT_FILE}"
echo "Summary: ${SUMMARY_FILE}"
