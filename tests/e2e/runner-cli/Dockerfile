# CLI E2E test runner — multi-stage build so the pinchtab binary is always
# present, correctly-architected, and executable inside the container.

ARG TARGETARCH=amd64

# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src

# Install git for modules that need VCS metadata
RUN apk add --no-cache git

# Dependency layer (cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy full source and build the CLI binary
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pinchtab ./cmd/pinchtab

# Library-mode example program, exercised by the library-mode smoke scenario
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/enrich-example ./docs/examples/enrich

# ── Runner image ──────────────────────────────────────────────────────────────
FROM alpine:3.23

RUN apk add --no-cache \
    bash \
    curl \
    jq \
    grep \
    sed \
    gawk \
    findutils \
    util-linux \
    coreutils \
    ca-certificates

# Copy the freshly-built binaries from the build stage
COPY --from=build /out/pinchtab /usr/local/bin/pinchtab
COPY --from=build /out/enrich-example /usr/local/bin/enrich-example
RUN chmod +x /usr/local/bin/pinchtab /usr/local/bin/enrich-example

# Sanity-check: bail early with a clear message if the binary is broken/missing
RUN pinchtab --version 2>/dev/null || { echo "ERROR: pinchtab binary not runnable after build"; exit 1; }

# Create results directory
RUN mkdir -p /results

WORKDIR /e2e

CMD ["/bin/bash", "-lc", "echo 'runner-cli is executor-only; use go run ./tests/tools/runner e2e --suite ...' >&2; exit 2"]
