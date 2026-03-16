#!/bin/sh
set -eu

home_dir="${HOME:-/data}"
xdg_config_home="${XDG_CONFIG_HOME:-$home_dir/.config}"
default_config_path="$xdg_config_home/pinchtab/config.json"

mkdir -p "$home_dir" "$xdg_config_home" "$(dirname "$default_config_path")"

# Generate a persisted config on first boot.
# The PINCHTAB_TOKEN env var can be used to set an auth token via Docker secrets
# or environment variables. Prefer Docker secrets for sensitive data:
#   docker run -e PINCHTAB_TOKEN_FILE=/run/secrets/pinchtab_token
if [ -z "${PINCHTAB_CONFIG:-}" ] && [ ! -f "$default_config_path" ]; then
  /usr/local/bin/pinchtab config init >/dev/null
  # Docker containers need to bind to 0.0.0.0 for port publishing to work
  /usr/local/bin/pinchtab config set server.bind "0.0.0.0" >/dev/null
  if [ -n "${PINCHTAB_TOKEN:-}" ]; then
    /usr/local/bin/pinchtab config set server.token "$PINCHTAB_TOKEN" >/dev/null
  fi
fi

# CHROME SANDBOX DISABLED IN CONTAINERS
#
# Chrome requires --no-sandbox inside containers because:
# - Containers don't have user namespaces (sandboxing requires this)
# - Container security (cgroups, capabilities, seccomp) provides isolation
# - The Dockerfile already drops capabilities and uses read-only filesystem
#
# This is standard for headless Chrome in containerized environments.
# Backfill the flag into managed config if not already set.
if [ -z "${PINCHTAB_CONFIG:-}" ] && [ -f "$default_config_path" ]; then
  current_flags="$(/usr/local/bin/pinchtab config get browser.extraFlags 2>/dev/null || true)"
  if [ -z "$current_flags" ]; then
    /usr/local/bin/pinchtab config set browser.extraFlags "--no-sandbox --disable-gpu" >/dev/null
  fi
fi

exec "$@"
