# shellcheck shell=bash
# On-failure inline dump: container logs, process list, fixture log, HTTP
# diagnostics, rendered config, plus a copy of /data into TMP_DIR.
# Idempotent and tolerates missing state.

dump_diagnostics() {
  local provider="$1"
  local name="$2"
  local host_port="$3"
  local token="$4"
  local config_path="$5"
  local fixtures_started="${6:-0}"

  echo ""
  echo "===== Diagnostics (provider=${provider}, container=${name}) ====="

  echo ""
  echo "--- Provider config (${config_path}) ---"
  if [ -f "$config_path" ]; then
    cat "$config_path" || true
  else
    echo "(config file missing)"
  fi

  if [ -n "$name" ] && docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    echo ""
    echo "--- Container logs (last 200) ---"
    docker logs --tail 200 "$name" 2>&1 || true

    echo ""
    echo "--- Process list ---"
    docker exec "$name" ps -eo pid,user,args 2>&1 || true

    if [ "$fixtures_started" = "1" ]; then
      echo ""
      echo "--- Fixture server log ---"
      docker exec "$name" sh -lc 'cat /tmp/pinchtab-fixtures.log 2>/dev/null || true' || true
    fi

    mkdir -p "$TMP_DIR/${provider}/data"
    docker cp "${name}:/data/." "$TMP_DIR/${provider}/data/" >/dev/null 2>&1 || true
  else
    echo "(container ${name} not found; skipping container-level diagnostics)"
  fi

  if [ -n "${host_port:-}" ]; then
    echo ""
    echo "--- HTTP /stealth/status ---"
    curl -sS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/stealth/status" 2>&1 || true
    echo ""

    echo ""
    echo "--- HTTP /autorestart/status ---"
    curl -sS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/autorestart/status" 2>&1 || true
    echo ""

    echo ""
    echo "--- HTTP /instances ---"
    local instances_diag
    instances_diag="$(curl -sS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/instances" 2>/dev/null || true)"
    echo "$instances_diag"
    echo "$instances_diag" | jq -r '.[].id? // empty' 2>/dev/null | while read -r inst_id; do
      [ -n "$inst_id" ] || continue
      echo ""
      echo "--- HTTP /instances/${inst_id}/logs ---"
      curl -sS -H "Authorization: Bearer ${token}" "http://127.0.0.1:${host_port}/instances/${inst_id}/logs" 2>&1 || true
      echo ""
    done
  fi

  echo ""
  echo "===== End diagnostics (${provider}) ====="
}
