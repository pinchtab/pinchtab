FROM node:20-bookworm

ENV DEBIAN_FRONTEND=noninteractive \
    npm_config_update_notifier=false \
    AGENT_BROWSER_EXECUTABLE_PATH=/usr/bin/chromium

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    chromium \
    curl \
    git \
    jq \
  && rm -rf /var/lib/apt/lists/*

# Linux ARM64 does not have a Chrome-for-Testing build, so use the distro
# Chromium binary and point agent-browser at it explicitly.
RUN npm install -g agent-browser

WORKDIR /workspace/tests/benchmark
