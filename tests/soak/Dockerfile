# Soak/stability test image: Chromium + Python (to serve fixtures) + curl.
# The pinchtab binary and fixtures are injected at provision time by setup.sh, so
# this image rarely needs rebuilding.
FROM debian:stable-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium ca-certificates curl python3 \
    && rm -rf /var/lib/apt/lists/*
