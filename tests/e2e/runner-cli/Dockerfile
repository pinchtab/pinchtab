# CLI E2E test runner
# Includes pinchtab binary for testing CLI commands

FROM alpine:3.21

# Install test dependencies
RUN apk add --no-cache \
    bash \
    curl \
    jq \
    coreutils

# Copy pre-built pinchtab binary (built by CI before docker build)
COPY pinchtab /usr/local/bin/pinchtab
RUN chmod +x /usr/local/bin/pinchtab

# Create results directory
RUN mkdir -p /results

WORKDIR /e2e

# Default command runs all tests
CMD ["/bin/bash", "/e2e/run.sh", "cli", "all=true"]
