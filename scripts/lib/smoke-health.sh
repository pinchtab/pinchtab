# shellcheck shell=bash
# HTTP readiness polling helpers.
#   wait_for_health <host_port> <token> [tries=90]
#   wait_for_instance_running <host_port> <token> [tries=120]
#     — aborts early if /autorestart/status reports status=crashed.

wait_for_health() {
  local host_port="$1"
  local token="$2"
  local tries="${3:-90}"
  local health_body=""
  for _ in $(seq 1 "$tries"); do
    if curl -fsS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  health_body="$(curl -sS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/health" 2>&1 || true)"
  fail "health check did not pass: $health_body"
}

wait_for_instance_running() {
  local host_port="$1"
  local token="$2"
  local tries="${3:-120}"
  local instances_body="[]"
  local autorestart_body="{}"
  for _ in $(seq 1 "$tries"); do
    instances_body="$(curl -fsS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/instances" 2>/dev/null || echo "[]")"
    if echo "$instances_body" | jq -e '.[]? | select(.status == "running")' >/dev/null; then
      return 0
    fi
    autorestart_body="$(curl -fsS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/autorestart/status" 2>/dev/null || echo "{}")"
    if echo "$autorestart_body" | jq -e '.status == "crashed"' >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  fail "managed browser instance did not become running: instances=$instances_body autorestart=$autorestart_body"
}
