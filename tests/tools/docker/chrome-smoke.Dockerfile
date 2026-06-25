# Local-only Chrome smoke image for the browser parity harness.
#
# Extends the standard PinchTab image with the helper utilities required by
# the in-container fixture server (perl) and HTTP probes (curl). These are
# intentionally NOT in the default runtime image because they are only needed
# by the opt-in parity smoke.
#
# Usage:
#   docker build -f tests/tools/docker/chrome-smoke.Dockerfile \
#                -t pinchtab-chrome-smoke:test .
#
# Built automatically by scripts/docker-browser-parity-smoke.sh.

ARG BASE_IMAGE=pinchtab-local:test
FROM ${BASE_IMAGE}

USER root
RUN apk add --no-cache perl curl
USER pinchtab
