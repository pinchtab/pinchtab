# Local-only CloakBrowser smoke image.
#
# This Dockerfile intentionally lives outside the default Dockerfile so the main
# PinchTab image stays small and does not include the CloakBrowser binary.
# Do not publish images built from this Dockerfile unless the CloakBrowser
# binary license explicitly permits redistribution for your use case.

# Stage 1: Build the React dashboard with Bun.
FROM oven/bun:1 AS dashboard
WORKDIR /build
COPY dashboard/package.json dashboard/bun.lock ./
RUN bun install --frozen-lockfile
COPY dashboard/ .
RUN bun run build

# Stage 2: Compile the Go binary.
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=dashboard /build/dist/ internal/dashboard/dashboard/
RUN mv internal/dashboard/dashboard/index.html internal/dashboard/dashboard/dashboard.html
RUN go build -ldflags="-s -w" -o pinchtab ./cmd/pinchtab

# Stage 3: Download CloakBrowser through its official Python package.
FROM python:3.12-slim AS cloakbrowser-binary
# Pinned so smoke/parity lanes are reproducible and upstream releases can't
# silently change what they test; bump deliberately. The package's
# ensure_binary() below still downloads a Chromium build at image build time
# (local-only image, never published — see header).
ARG CLOAKBROWSER_VERSION=0.3.31
ENV CLOAKBROWSER_CACHE_DIR=/cloak-cache \
    CLOAKBROWSER_AUTO_UPDATE=false \
    PYTHONUNBUFFERED=1
RUN python -m pip install --disable-pip-version-check --root-user-action=ignore --no-cache-dir "cloakbrowser==${CLOAKBROWSER_VERSION}" && \
    python - <<'PY'
import logging
from pathlib import Path
from shutil import copytree, rmtree
from cloakbrowser.download import ensure_binary

logging.basicConfig(level=logging.INFO, format="cloakbrowser: %(message)s")
print("Installing CloakBrowser Chromium binary into the local smoke image...", flush=True)
src = Path(ensure_binary()).parent
dst = Path("/opt/cloakbrowser")
if dst.exists():
    rmtree(dst)
copytree(src, dst, symlinks=True)
print(f"CloakBrowser Chromium binary ready at {dst / 'chrome'}", flush=True)
PY

# Stage 4: Runtime image with GNU/Linux userspace for CloakBrowser.
FROM debian:bookworm-slim

LABEL org.opencontainers.image.source="https://github.com/pinchtab/pinchtab"
LABEL org.opencontainers.image.description="PinchTab with local-only CloakBrowser runtime for smoke testing"

# Debian's glibc userspace lets the CloakBrowser Linux binary run; Alpine reports
# that binary as "not found" because the GNU dynamic loader is absent.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    chromium \
    curl \
    dumb-init \
    fontconfig \
    fonts-liberation \
    fonts-noto-color-emoji \
    procps \
    socat \
  && rm -rf /var/lib/apt/lists/*

# Non-root user; /data is the persistent volume mount point.
RUN groupadd --gid 1000 pinchtab && \
    useradd --uid 1000 --gid 1000 --home-dir /data --create-home --shell /usr/sbin/nologin pinchtab && \
    mkdir -p /data && \
    chown pinchtab:pinchtab /data

COPY --from=builder /build/pinchtab /usr/local/bin/pinchtab
COPY --from=cloakbrowser-binary /opt/cloakbrowser /opt/cloakbrowser
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

USER pinchtab
WORKDIR /data

# HOME and XDG_CONFIG_HOME point into the persistent volume so config
# and Chrome profiles survive container restarts.
ENV HOME=/data \
    XDG_CONFIG_HOME=/data/.config

EXPOSE 9867

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD /bin/sh -lc 'pinchtab health >/dev/null' || exit 1

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/local/bin/docker-entrypoint.sh", "pinchtab", "server"]
