# shellcheck shell=bash
# Generates runtime config JSON for the smoke harnesses.
#   write_provider_config <provider> <path> <token> <fixtures_host> [chrome_bin] [cloak_bin]
#   write_multi_target_config <path> <token> <fixtures_host> <chrome_bin> <cloak_bin> [primary_bin]
# When primary_bin is set, the chrome target points there instead of chrome_bin
# — used by the multi-target fallback assertion.

write_provider_config() {
  local provider="$1"
  local config_path="$2"
  local token="$3"
  local fixtures_host="$4"
  local chrome_binary="${5:-}"
  local cloak_binary="${6:-}"

  python3 - "$config_path" "$provider" "$token" "$fixtures_host" "$chrome_binary" "$cloak_binary" <<'PY'
import json
import sys

path, provider, token, fixtures_host, chrome_binary, cloak_binary = sys.argv[1:7]

browser = {
    "extensionPaths": [],
}
if provider == "cloak":
    if not cloak_binary:
        raise SystemExit("cloak provider requires cloak_binary path")
    browser["binary"] = cloak_binary
    browser["cloak"] = {
        "fingerprintSeed": "42069",
        "platform": "linux",
        "locale": "en-US",
        "timezone": "UTC",
        "disableDefaultStealthArgs": True,
    }
elif provider == "chrome":
    if chrome_binary:
        browser["binary"] = chrome_binary
else:
    raise SystemExit(f"unknown provider: {provider}")

cfg = {
    "server": {
        "bind": "0.0.0.0",
        "port": "9867",
        "token": token,
        "stateDir": "/data",
    },
    # browsers.default is the authoritative provider selector. The legacy
    # browser.provider field is rejected at validation time (it breaks
    # `config set`/`config validate`); browser.binary/cloak below remain
    # accepted and migrate into the default target on load.
    "browsers": {
        "default": provider,
        "available": [provider],
    },
    "browser": browser,
    "instanceDefaults": {
        "mode": "headless",
        "humanize": True,
        "maxTabs": 10,
    },
    "security": {
        "allowEvaluate": True,
        "allowDownload": True,
        "allowCookies": True,
        "allowUpload": True,
        "allowClipboard": True,
        "allowStateExport": True,
        "allowedDomains": [fixtures_host, "127.0.0.1", "localhost", "::1"],
        "downloadAllowedDomains": [fixtures_host],
        # 127.0.0.1/32 alone blocks fixtures that resolve to the Docker bridge
        # network even when the host is in allowedDomains. Docker can assign
        # any RFC1918 range to a user-defined network (172.16/12 is the legacy
        # bridge default, but compose networks routinely land on 192.168.0.0/20
        # on macOS Docker Desktop, and custom networks use 10.0.0.0/8). Trust
        # all three to keep tests stable across Docker hosts.
        "trustedResolveCIDRs": [
            "127.0.0.1/32",
            "10.0.0.0/8",
            "172.16.0.0/12",
            "192.168.0.0/16",
        ],
    },
    "profiles": {
        "baseDir": "/data/profiles",
        "defaultProfile": f"{provider}-docker-smoke",
    },
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(cfg, fh, indent=2)
PY
}

write_multi_target_config() {
  local config_path="$1"
  local token="$2"
  local fixtures_host="$3"
  local chrome_binary="$4"
  local cloak_binary="$5"
  local primary_binary="${6:-}"

  if [ -z "$chrome_binary" ] || [ -z "$cloak_binary" ]; then
    fail "write_multi_target_config: chrome_binary and cloak_binary are required"
  fi

  python3 - "$config_path" "$token" "$fixtures_host" "$chrome_binary" "$cloak_binary" "$primary_binary" <<'PY'
import json
import sys

path, token, fixtures_host, chrome_binary, cloak_binary, primary_binary = sys.argv[1:7]

chrome_target_binary = primary_binary if primary_binary else chrome_binary

cfg = {
    "server": {
        "bind": "0.0.0.0",
        "port": "9867",
        "token": token,
        "stateDir": "/data",
    },
    "browser": {
        "extensionPaths": [],
        "targets": {
            "chrome": {
                "provider": "chrome",
                "binary": chrome_target_binary,
            },
            "cloak": {
                "provider": "cloak",
                "binary": cloak_binary,
                "cloak": {
                    "fingerprintSeed": "42069",
                    "platform": "linux",
                    "locale": "en-US",
                    "timezone": "UTC",
                    "disableDefaultStealthArgs": True,
                },
            },
        },
        "defaultTarget": "chrome",
        "fallbackOrder": ["chrome", "cloak"],
    },
    "instanceDefaults": {
        "mode": "headless",
        "humanize": True,
        "maxTabs": 10,
    },
    "security": {
        "allowEvaluate": True,
        "allowDownload": True,
        "allowCookies": True,
        "allowUpload": True,
        "allowClipboard": True,
        "allowStateExport": True,
        "allowedDomains": [fixtures_host, "127.0.0.1", "localhost", "::1"],
        "downloadAllowedDomains": [fixtures_host],
        # See write_provider_config above: Docker can assign any RFC1918 range
        # to a user-defined network; trust all three so fixtures aren't blocked
        # by the private-IP guard.
        "trustedResolveCIDRs": [
            "127.0.0.1/32",
            "10.0.0.0/8",
            "172.16.0.0/12",
            "192.168.0.0/16",
        ],
    },
    "profiles": {
        "baseDir": "/data/profiles",
        "defaultProfile": "multi-target-docker-smoke",
    },
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(cfg, fh, indent=2)
PY
}
