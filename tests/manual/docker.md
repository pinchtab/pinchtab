# Pinchtab Docker Test Plan

**Goal:** Validate the Docker image builds, runs, and behaves identically to the native binary.

**Prerequisites:** Docker installed, `pinchtab/pinchtab:latest` image available (pull or local build).

---

## 1. Image Build & Structure

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D1 | Build succeeds | `docker build -t pinchtab/pinchtab .` | Exit 0, image created |
| D2 | Image size reasonable | `docker images pinchtab/pinchtab --format '{{.Size}}'` | < 1GB |
| D3 | Non-root user | `docker run --rm pinchtab/pinchtab whoami` | `pinchtab` |
| D4 | Binary present | `docker run --rm pinchtab/pinchtab which pinchtab` | `/usr/local/bin/pinchtab` |
| D5 | Chromium present | `docker run --rm pinchtab/pinchtab which chromium-browser` | Path returned |
| D6 | No build artifacts | `docker run --rm pinchtab/pinchtab ls /build 2>&1` | Directory not found |

## 2. Container Startup & Health

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D7 | Default startup | `docker run -d -p 9867:9867 --security-opt seccomp=unconfined pinchtab/pinchtab` | Container starts, stays running |
| D8 | Health check | `curl http://localhost:9867/health` | 200, `{"status":"ok"}` |
| D9 | Custom port | `docker run -d -p 8080:8080 -e BRIDGE_PORT=8080 --security-opt seccomp=unconfined pinchtab/pinchtab` | Health on :8080 |
| D10 | Auth token | `docker run -d -p 9867:9867 -e BRIDGE_TOKEN=secret --security-opt seccomp=unconfined pinchtab/pinchtab` | `/health` without token → 401, with token → 200 |
| D11 | Graceful stop | `docker stop <container>` | Exit 0, no zombie Chrome processes |

## 3. Core Functionality (in container)

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D12 | Navigate | `POST /navigate {"url":"https://example.com"}` | 200, title="Example Domain" |
| D13 | Snapshot | `GET /snapshot?tabId=<id>` | 200, accessibility tree returned |
| D14 | Text extract | `GET /text?tabId=<id>` | 200, page text content |
| D15 | Click action | `POST /action {"kind":"click","ref":"e1"}` | 200, action performed |
| D16 | Multi-tab | Open 3 tabs via `/navigate`, `GET /tabs` | All 3 tabs listed |

## 4. Persistence & Volumes

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D17 | State dir mounted | `docker run -v pinchtab-data:/data ...`, navigate + login, stop, restart | Session/cookies persist |
| D18 | Profile persistence | Same as D17 — Chrome profile in `/data/chrome-profile` | Login state survives restart |
| D19 | Without volume | Run without `-v`, stop, restart | Clean slate, no state |

## 5. Docker Compose

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D20 | Compose up | `docker compose up -d` | Container starts, health OK |
| D21 | Compose with token | `PINCHTAB_TOKEN=secret docker compose up -d` | Auth enforced |
| D22 | Compose down + up | `docker compose down && docker compose up -d` | Volume persists, clean restart |
| D23 | SHM size | Check Chrome stability under load (compose sets `shm_size: 2gb`) | No crashes from shared memory exhaustion |

## 6. Multi-Platform

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D24 | AMD64 image | Pull/run on x86_64 host | Works correctly |
| D25 | ARM64 image | Pull/run on ARM host (e.g. Apple Silicon via Rosetta, Raspberry Pi) | Works correctly |

## 7. Resource & Security

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D26 | Memory limit | `docker run --memory=512m ...` | Runs (may OOM on heavy pages, but starts OK) |
| D27 | CPU limit | `docker run --cpus=1 ...` | Runs, slower but functional |
| D28 | seccomp required | `docker run -d -p 9867:9867 pinchtab/pinchtab` (no seccomp flag) | Chrome may fail to launch — document requirement |
| D29 | Read-only rootfs | `docker run --read-only -v pinchtab-data:/data ...` | Runs (writes only to /data) |
| D30 | No privileged | `docker run ...` (without `--privileged`) | Works with seccomp=unconfined, no need for privileged |

## 8. Edge Cases

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| D31 | Rapid restart | `docker restart <container>` 5 times quickly | No port conflicts, no zombie processes |
| D32 | OOM kill recovery | Kill Chrome inside container | Pinchtab detects and either restarts Chrome or exits cleanly |
| D33 | Container logs | `docker logs <container>` | Startup info visible, no sensitive data leaked |
| D34 | Signal forwarding | `docker kill -s SIGTERM <container>` | dumb-init forwards signal, clean shutdown |
