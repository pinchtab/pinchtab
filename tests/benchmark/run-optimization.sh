#!/bin/bash
#
# PinchTab Benchmark Optimization Loop
# Runs both benchmarks, analyzes differences, proposes improvements
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
LOG_FILE="${RESULTS_DIR}/optimization_log.md"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RUN_NUMBER=$(grep -c "^## Run #" "${LOG_FILE}" 2>/dev/null || echo 0)
RUN_NUMBER=$((RUN_NUMBER + 1))

echo "=== PinchTab Optimization Run #${RUN_NUMBER} ==="
echo "Timestamp: ${TIMESTAMP}"

# Ensure we're on feat/benchmark branch
cd ~/dev/pinchtab
CURRENT_BRANCH=$(git branch --show-current)
if [[ "${CURRENT_BRANCH}" != "feat/benchmark" ]]; then
    echo "Switching to feat/benchmark branch..."
    git checkout feat/benchmark
fi
git pull --rebase origin feat/benchmark 2>/dev/null || true

# Ensure Docker is running
cd "${SCRIPT_DIR}"
if ! docker compose ps 2>/dev/null | grep -q "running"; then
    echo "Starting Docker..."
    docker compose up -d --build
    sleep 15
fi

# Verify PinchTab is healthy
if ! curl -sf -H "Authorization: Bearer benchmark-token" http://localhost:9867/health > /dev/null; then
    echo "ERROR: PinchTab not responding, restarting..."
    docker compose down
    docker compose up -d --build
    sleep 15
fi

# Initialize reports
BASELINE_REPORT="${RESULTS_DIR}/baseline_${TIMESTAMP}.json"
AGENT_REPORT="${RESULTS_DIR}/agent_benchmark_${TIMESTAMP}.json"

cat > "${BASELINE_REPORT}" << EOF
{
  "benchmark": {
    "type": "baseline",
    "run_number": ${RUN_NUMBER},
    "timestamp": "${TIMESTAMP}",
    "model": "claude-haiku-4-5"
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0
  },
  "steps": []
}
EOF

cat > "${AGENT_REPORT}" << EOF
{
  "benchmark": {
    "type": "agent",
    "run_number": ${RUN_NUMBER},
    "timestamp": "${TIMESTAMP}",
    "model": "claude-haiku-4-5"
  },
  "totals": {
    "input_tokens": 0,
    "output_tokens": 0,
    "total_tokens": 0,
    "estimated_cost_usd": 0,
    "steps_passed": 0,
    "steps_failed": 0,
    "steps_skipped": 0
  },
  "steps": []
}
EOF

# Clear previous agent commands log
rm -f "${RESULTS_DIR}/agent_commands.log"

echo "Reports initialized:"
echo "  Baseline: ${BASELINE_REPORT}"
echo "  Agent: ${AGENT_REPORT}"
echo ""
echo "Ready for benchmark execution."
echo "Timestamp for this run: ${TIMESTAMP}"
echo "Run number: ${RUN_NUMBER}"
