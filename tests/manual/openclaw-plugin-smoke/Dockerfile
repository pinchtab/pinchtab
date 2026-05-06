FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    npm_config_update_notifier=false

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    python3 \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

ARG OPENCLAW_VERSION=2026.4.27
RUN npm install -g openclaw@$OPENCLAW_VERSION
