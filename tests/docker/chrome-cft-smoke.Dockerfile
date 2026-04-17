FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/pinchtab ./cmd/pinchtab

FROM ubuntu:22.04

ARG CHROME_FOR_TESTING_VERSION=145.0.7632.6

ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/data \
    XDG_CONFIG_HOME=/data/.config \
    PINCHTAB_CONFIG=/etc/pinchtab/smoke-config.json

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    dumb-init \
    fonts-liberation \
    libasound2 \
    libatk-bridge2.0-0 \
    libatk1.0-0 \
    libc6 \
    libcairo2 \
    libcups2 \
    libdbus-1-3 \
    libexpat1 \
    libfontconfig1 \
    libgbm1 \
    libgcc-s1 \
    libglib2.0-0 \
    libgtk-3-0 \
    libnspr4 \
    libnss3 \
    libpango-1.0-0 \
    libpangocairo-1.0-0 \
    libstdc++6 \
    libx11-6 \
    libx11-xcb1 \
    libxcb1 \
    libxcomposite1 \
    libxcursor1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxi6 \
    libxrandr2 \
    libxrender1 \
    libxshmfence1 \
    libxss1 \
    libxtst6 \
    netcat-openbsd \
    procps \
    unzip \
 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://storage.googleapis.com/chrome-for-testing-public/${CHROME_FOR_TESTING_VERSION}/linux64/chrome-linux64.zip" -o /tmp/chrome-linux64.zip \
 && mkdir -p /opt/chrome \
 && unzip -q /tmp/chrome-linux64.zip -d /opt/chrome \
 && ln -s /opt/chrome/chrome-linux64/chrome /usr/bin/google-chrome \
 && rm -f /tmp/chrome-linux64.zip

RUN useradd -m -d /data -s /bin/bash pinchtab \
 && mkdir -p /data /etc/pinchtab \
 && chown -R pinchtab:pinchtab /data

COPY --from=builder /out/pinchtab /usr/local/bin/pinchtab
RUN printf '%s\n' \
  '{' \
  '  "server": {' \
  '    "bind": "0.0.0.0",' \
  '    "port": "9867",' \
  '    "token": "chrome-cft-smoke-token"' \
  '  },' \
  '  "browser": {' \
    '    "binary": "/opt/chrome/chrome-linux64/chrome"' \
  '  },' \
  '  "instanceDefaults": {' \
  '    "mode": "headless"' \
  '  }' \
  '}' > /etc/pinchtab/smoke-config.json

USER pinchtab
WORKDIR /data

EXPOSE 9867

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["pinchtab", "server"]
