# shellcheck shell=bash
# Assertions for the multi-target parity leg: request-level target selection,
# default-target resolution, and primary-failure fallback.

# Writes a stub binary that exits 1. Bind-mounted from host so the non-root
# container user can exec it.
setup_broken_binary() {
  local path="$1"
  cat >"$path" <<'SH'
#!/bin/sh
echo "broken-binary stub: refusing to start" >&2
exit 1
SH
  chmod +x "$path"
  echo "$path"
}

_wait_for_instance_status() {
  local host_port="$1"
  local token="$2"
  local id="$3"
  local target="$4"
  local tries="${5:-120}"
  local body=""
  for _ in $(seq 1 "$tries"); do
    body="$(curl -fsS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/instances/${id}" 2>/dev/null || echo "{}")"
    local status
    status="$(echo "$body" | jq -r '.status // empty')"
    if [ "$status" = "$target" ]; then
      echo "$body"
      return 0
    fi
    if [ "$status" = "error" ]; then
      fail "instance $id entered status=error while waiting for $target: $body"
    fi
    sleep 1
  done
  fail "instance $id did not reach status=$target within ${tries}s: $body"
}

# Best-effort stop; never fails the leg.
_stop_instance() {
  local host_port="$1"
  local token="$2"
  local id="$3"
  [ -n "$id" ] || return 0
  curl -fsS -X POST -H "Authorization: Bearer ${token}" \
    "http://127.0.0.1:${host_port}/instances/${id}/stop" >/dev/null 2>&1 || true
}

# Uses the Instance response (not /stealth/status) as the authoritative
# provider signal — /stealth/status is process-scoped and unreliable in
# multi-instance setups.
assert_target_selection() {
  local host_port="$1"
  local token="$2"
  local target="$3"
  local expected_provider="$4"

  echo "  → target selection: browserTarget=${target} (expecting provider=${expected_provider})"

  local body
  body="$(curl -fsS -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"browserTarget\":\"${target}\"}" \
    "http://127.0.0.1:${host_port}/instances/start" 2>&1)" \
    || fail "POST /instances/start (target=${target}) failed: $body"

  local instance_id resolved_target launch_provider
  instance_id="$(echo "$body" | jq -r '.id // empty')"
  resolved_target="$(echo "$body" | jq -r '.browserTarget // empty')"
  launch_provider="$(echo "$body" | jq -r '.browserProvider // empty')"
  [ -n "$instance_id" ] || fail "instances/start (target=${target}) returned no id: $body"
  if [ "$resolved_target" != "$target" ]; then
    fail "instances/start (target=${target}) returned browserTarget=${resolved_target}: $body"
  fi
  if [ "$launch_provider" != "$expected_provider" ]; then
    fail "instances/start (target=${target}) returned browserProvider=${launch_provider}, expected ${expected_provider}: $body"
  fi

  local running_body
  running_body="$(_wait_for_instance_status "$host_port" "$token" "$instance_id" "running")"

  local got_target got_provider
  got_target="$(echo "$running_body" | jq -r '.browserTarget // empty')"
  got_provider="$(echo "$running_body" | jq -r '.browserProvider // empty')"
  echo "    running instance: browserTarget=${got_target} browserProvider=${got_provider}"
  if [ "$got_provider" != "$expected_provider" ]; then
    fail "running instance for target=${target} reports browserProvider=${got_provider}, expected ${expected_provider}: $running_body"
  fi

  _stop_instance "$host_port" "$token" "$instance_id"
}

assert_default_target() {
  local host_port="$1"
  local token="$2"
  local expected="$3"

  echo "  → default target (expecting browserTarget=${expected})"

  local body
  body="$(curl -fsS -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d '{}' \
    "http://127.0.0.1:${host_port}/instances/start" 2>&1)" \
    || fail "POST /instances/start (default) failed: $body"

  local instance_id resolved
  instance_id="$(echo "$body" | jq -r '.id // empty')"
  resolved="$(echo "$body" | jq -r '.browserTarget // empty')"
  [ -n "$instance_id" ] || fail "instances/start (default) returned no id: $body"
  echo "    response browserTarget: ${resolved}"
  if [ "$resolved" != "$expected" ]; then
    _stop_instance "$host_port" "$token" "$instance_id"
    fail "expected default browserTarget=${expected}, got ${resolved}: $body"
  fi

  _wait_for_instance_status "$host_port" "$token" "$instance_id" "running" >/dev/null
  _stop_instance "$host_port" "$token" "$instance_id"
}

assert_fallback() {
  local host_port="$1"
  local token="$2"
  local broken="$3"
  local healthy="$4"
  local expected_provider="$5"

  echo "  → fallback: primary=${broken} (broken) → ${healthy} (healthy)"

  local body
  body="$(curl -fsS -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"browserTarget\":\"${broken}\",\"fallbackTargets\":[\"${healthy}\"]}" \
    "http://127.0.0.1:${host_port}/instances/start" 2>&1)" \
    || fail "POST /instances/start (fallback) failed: $body"

  local instance_id resolved fallback_from fallback_reason
  instance_id="$(echo "$body" | jq -r '.id // empty')"
  resolved="$(echo "$body" | jq -r '.browserTarget // empty')"
  fallback_from="$(echo "$body" | jq -r '.fallbackFrom // empty')"
  fallback_reason="$(echo "$body" | jq -r '.fallbackReason // empty')"

  [ -n "$instance_id" ] || fail "fallback launch returned no id: $body"
  echo "    response: browserTarget=${resolved} fallbackFrom=${fallback_from} fallbackReason=${fallback_reason}"

  if [ -z "$fallback_from" ] || [ -z "$fallback_reason" ]; then
    _stop_instance "$host_port" "$token" "$instance_id"
    fail "expected non-empty fallbackFrom + fallbackReason on response: $body"
  fi
  if [ "$fallback_from" != "$broken" ]; then
    _stop_instance "$host_port" "$token" "$instance_id"
    fail "expected fallbackFrom=${broken}, got ${fallback_from}: $body"
  fi
  if [ "$resolved" != "$healthy" ]; then
    _stop_instance "$host_port" "$token" "$instance_id"
    fail "expected resolved browserTarget=${healthy}, got ${resolved}: $body"
  fi

  local running_body
  running_body="$(_wait_for_instance_status "$host_port" "$token" "$instance_id" "running")"
  local got_provider
  got_provider="$(echo "$running_body" | jq -r '.browserProvider // empty')"
  echo "    running instance: browserProvider=${got_provider}"
  if [ "$got_provider" != "$expected_provider" ]; then
    _stop_instance "$host_port" "$token" "$instance_id"
    fail "fallback instance reports browserProvider=${got_provider}, expected ${expected_provider}: $running_body"
  fi

  _stop_instance "$host_port" "$token" "$instance_id"
}
