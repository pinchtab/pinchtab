# Build stage
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o pinchtab ./cmd/pinchtab

# Runtime stage
FROM alpine:latest

# Install Chromium and dependencies
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    dumb-init

# Create non-root user
RUN adduser -D -g '' pinchtab

# Copy binary from builder
COPY --from=builder /build/pinchtab /usr/local/bin/pinchtab

# Create state directory
RUN mkdir -p /data && chown pinchtab:pinchtab /data

# Switch to non-root user
USER pinchtab

# Environment variables
ENV BRIDGE_PORT=9867 \
    BRIDGE_HEADLESS=true \
    BRIDGE_STATE_DIR=/data \
    BRIDGE_PROFILE=/data/chrome-profile \
    CHROME_BINARY=/usr/bin/chromium-browser \
    CHROME_FLAGS="--no-sandbox --disable-gpu"

# Expose port
EXPOSE 9867

# Use dumb-init to properly handle signals
ENTRYPOINT ["/usr/bin/dumb-init", "--"]

# Run pinchtab
CMD ["pinchtab"]