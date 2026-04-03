#!/bin/bash
#
# Run both baseline and agent benchmarks for comparison
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== PinchTab Benchmark Comparison ==="
echo "Timestamp: ${TIMESTAMP}"
echo ""

# Check Docker
if ! docker compose ps | grep -q "running"; then
    echo "Starting Docker..."
    docker compose up -d --build
    sleep 10
fi

# Verify PinchTab is healthy
if ! curl -sf -H "Authorization: Bearer benchmark-token" http://localhost:9867/health > /dev/null; then
    echo "ERROR: PinchTab not responding"
    exit 1
fi

echo "✅ PinchTab is healthy"

# Initialize baseline report
BASELINE_REPORT="${RESULTS_DIR}/baseline_${TIMESTAMP}.json"
cat > "${BASELINE_REPORT}" << EOF
{
  "benchmark": {
    "type": "baseline",
    "timestamp": "${TIMESTAMP}",
    "model": "claude-haiku-4-5",
    "pinchtab_version": "dev"
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
echo "📊 Baseline report: ${BASELINE_REPORT}"

# Initialize agent report
AGENT_REPORT="${RESULTS_DIR}/agent_benchmark_${TIMESTAMP}.json"
cat > "${AGENT_REPORT}" << EOF
{
  "benchmark": {
    "type": "agent",
    "timestamp": "${TIMESTAMP}",
    "model": "claude-haiku-4-5",
    "pinchtab_version": "dev"
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
echo "🤖 Agent report: ${AGENT_REPORT}"

echo ""
echo "Reports initialized. Run benchmarks:"
echo "  1. Baseline: Follow BENCHMARK_TASKS.md (explicit commands)"
echo "  2. Agent:    Follow AGENT_TASKS.md (natural language)"
echo ""
echo "After both complete, run: ./compare-results.sh ${TIMESTAMP}"
