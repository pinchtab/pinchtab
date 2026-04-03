#!/bin/bash
#
# Compare baseline vs agent benchmark results
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <timestamp>"
    echo "Example: $0 20260403_010000"
    exit 1
fi

TIMESTAMP="$1"
BASELINE="${RESULTS_DIR}/baseline_${TIMESTAMP}.json"
AGENT="${RESULTS_DIR}/agent_benchmark_${TIMESTAMP}.json"

if [[ ! -f "${BASELINE}" ]]; then
    echo "ERROR: Baseline report not found: ${BASELINE}"
    exit 1
fi

if [[ ! -f "${AGENT}" ]]; then
    echo "ERROR: Agent report not found: ${AGENT}"
    exit 1
fi

echo "# Benchmark Comparison: ${TIMESTAMP}"
echo ""
echo "## Results"
echo ""
echo "| Metric | Baseline | Agent Mode | Diff |"
echo "|--------|----------|------------|------|"

# Extract metrics
B_PASS=$(jq '.totals.steps_passed' "${BASELINE}")
B_FAIL=$(jq '.totals.steps_failed' "${BASELINE}")
B_TOTAL=$((B_PASS + B_FAIL))
B_RATE=$(echo "scale=1; ${B_PASS} * 100 / ${B_TOTAL}" | bc)
B_TOKENS=$(jq '.totals.total_tokens' "${BASELINE}")
B_COST=$(jq '.totals.estimated_cost_usd' "${BASELINE}")

A_PASS=$(jq '.totals.steps_passed' "${AGENT}")
A_FAIL=$(jq '.totals.steps_failed' "${AGENT}")
A_TOTAL=$((A_PASS + A_FAIL))
A_RATE=$(echo "scale=1; ${A_PASS} * 100 / ${A_TOTAL}" | bc)
A_TOKENS=$(jq '.totals.total_tokens' "${AGENT}")
A_COST=$(jq '.totals.estimated_cost_usd' "${AGENT}")

# Calculate diffs
RATE_DIFF=$(echo "scale=1; ${A_RATE} - ${B_RATE}" | bc)
TOKEN_DIFF=$(echo "scale=0; ${A_TOKENS} - ${B_TOKENS}" | bc)
COST_DIFF=$(echo "scale=4; ${A_COST} - ${B_COST}" | bc)

echo "| Pass Rate | ${B_RATE}% | ${A_RATE}% | ${RATE_DIFF}% |"
echo "| Steps Passed | ${B_PASS}/${B_TOTAL} | ${A_PASS}/${A_TOTAL} | |"
echo "| Total Tokens | ${B_TOKENS} | ${A_TOKENS} | ${TOKEN_DIFF} |"
echo "| Cost (USD) | \$${B_COST} | \$${A_COST} | \$${COST_DIFF} |"

if [[ ${B_TOTAL} -gt 0 ]]; then
    B_AVG=$(echo "scale=0; ${B_TOKENS} / ${B_TOTAL}" | bc)
else
    B_AVG=0
fi

if [[ ${A_TOTAL} -gt 0 ]]; then
    A_AVG=$(echo "scale=0; ${A_TOKENS} / ${A_TOTAL}" | bc)
else
    A_AVG=0
fi

echo "| Avg Tokens/Step | ${B_AVG} | ${A_AVG} | |"

echo ""
echo "## Agent Commands Used"
echo ""
echo "Commands the agent chose (from agent_commands.log):"
echo '```'
cat "${RESULTS_DIR}/agent_commands.log" 2>/dev/null || echo "(no commands logged)"
echo '```'

echo ""
echo "## Analysis"
echo ""
if (( $(echo "${A_RATE} >= ${B_RATE}" | bc -l) )); then
    echo "✅ Agent mode matched or exceeded baseline pass rate"
else
    echo "⚠️ Agent mode had lower pass rate than baseline"
fi

if (( $(echo "${A_TOKENS} <= ${B_TOKENS}" | bc -l) )); then
    echo "✅ Agent mode used same or fewer tokens"
else
    echo "⚠️ Agent mode used more tokens (natural language overhead)"
fi

# Save comparison
COMPARE_FILE="${RESULTS_DIR}/comparison_${TIMESTAMP}.md"
$0 "$TIMESTAMP" > "${COMPARE_FILE}" 2>/dev/null || true
echo ""
echo "Comparison saved to: ${COMPARE_FILE}"
