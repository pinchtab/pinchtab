# shellcheck shell=bash
# Container lifecycle helpers.
#   start_provider_container <name> <image> <config_path> <fixtures_host> <fixtures_port> <root>
#   resolve_host_port <name>     — echoes host port mapped to container :9867
#   teardown_container <name>    — idempotent force-remove
#
# Env: PROFILE_VOLUME=<name> swaps /data from tmpfs to a persistent Docker
# named volume (used by the profile-persistence leg).

start_provider_container() {
  local name="$1"
  local image="$2"
  local config_path="$3"
  local fixtures_host="$4"
  local fixtures_port="$5"
  local root="$6"

  local -a data_mount
  if [ -n "${PROFILE_VOLUME:-}" ]; then
    data_mount=(-v "${PROFILE_VOLUME}:/data:rw")
  else
    data_mount=(--tmpfs "/data:rw,size=1024m,uid=1000,gid=1000,mode=0755")
  fi

  docker run -d \
    --name "$name" \
    --user 1000:1000 \
    --shm-size=2g \
    "${data_mount[@]}" \
    --tmpfs /tmp:rw,size=512m,mode=1777 \
    --add-host "${fixtures_host}:127.0.0.1" \
    -e "PINCHTAB_CONFIG=/config/pinchtab.json" \
    -v "${config_path}:/config/pinchtab.json:ro" \
    -v "${root}/tests/e2e/fixtures:/fixtures:ro" \
    -v "${root}/tests/tools/fixtures/static-server.pl:/usr/local/bin/fixture-server.pl:ro" \
    --publish "127.0.0.1::9867" \
    --publish "127.0.0.1:${fixtures_port}:${fixtures_port}" \
    "$image" >/dev/null
  CONTAINER_STARTED=1
}

resolve_host_port() {
  local name="$1"
  local port
  port="$(docker port "$name" 9867/tcp 2>/dev/null | head -1 | awk -F: '{print $NF}' || true)"
  if [ -z "$port" ]; then
    port="$(docker inspect -f '{{with index .NetworkSettings.Ports "9867/tcp"}}{{(index . 0).HostPort}}{{end}}' "$name" 2>/dev/null || true)"
  fi
  if [ -z "$port" ]; then
    fail "failed to determine published host port for $name"
  fi
  echo "$port"
}

teardown_container() {
  local name="$1"
  if docker ps -a --format '{{.Names}}' | grep -Fxq "$name"; then
    docker rm -f "$name" >/dev/null 2>&1 || true
  fi
}
