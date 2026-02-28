# Building Pinchtab from Source

Complete guide to clone, build, and run Pinchtab from source code.

## System Requirements

### Minimum Requirements

| Requirement | Version | Purpose |
|------------|---------|---------|
| Go | 1.25+ | Build language |
| Chrome/Chromium | Latest | Browser automation |
| macOS, Linux, or WSL2 | Current | OS support |

### Recommended Setup

- **macOS**: Homebrew for package management
- **Linux**: apt (Debian/Ubuntu) or yum (RHEL/CentOS)
- **WSL2**: Full Linux environment (not WSL1)

---

## Part 1: Prerequisites

### Install Go

**macOS (Homebrew):**
```bash
brew install go
go version  # Verify: go version go1.25.0
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install -y golang-go git build-essential
go version
```

**Linux (RHEL/CentOS):**
```bash
sudo yum install -y golang git
go version
```

**Manual (any OS):**
Download from https://golang.org/dl/ and follow instructions.

### Install Chrome/Chromium

**macOS (Homebrew):**
```bash
brew install chromium
# Or use system Chrome
which chromium  # /usr/local/bin/chromium
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt install -y chromium-browser
which chromium-browser
```

**Linux (RHEL/CentOS):**
```bash
sudo yum install -y chromium
which chromium
```

### Verify Installations

```bash
go version       # go version go1.25.0 darwin/arm64
git --version    # git version 2.39.0
chromium --version  # Chromium 120.0.6099.xx
```

---

## Part 2: Clone the Repository

### Clone from GitHub

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
```

### Verify You're on Main Branch

```bash
git branch
# * main
#   feat/...
#   other-branches
```

### Check Project Structure

```bash
ls -la
# cmd/              # Go entry points
# docs/             # Documentation
# internal/         # Core packages
# scripts/          # Build and utility scripts
# go.mod            # Go module file
# go.sum            # Go module checksums
# Dockerfile        # Container build
# README.md         # Project overview
```

---

## Part 3: Install Dependencies

### Download Go Modules

```bash
cd ~/dev/pinchtab  # or your pinchtab directory
go mod download
```

**What it does:**
- Downloads all Go dependencies from `go.mod`
- Verifies checksums against `go.sum`
- Caches modules locally

**Output:**
```
go: downloading github.com/chromedp/chromedp v0.14.2
go: downloading github.com/chromedp/cdproto v0.0.0-...
...
```

### Verify Dependencies

```bash
go mod verify
# go: all module checksums verified (or error if corrupted)

go list -m all  # List all dependencies
```

---

## Part 4: Build the Project

### Simple Build

```bash
go build -o pinchtab ./cmd/pinchtab
```

**What it does:**
- Compiles Go source code
- Produces binary: `./pinchtab`
- Takes ~30-60 seconds

**Output:**
```
# (no output = success)
ls -la pinchtab
# -rwxr-xr-x  pinchtab  12MB  (approximate size)
```

### Optimized Build (for production)

```bash
go build -ldflags="-s -w" -o pinchtab ./cmd/pinchtab
```

**Flags explained:**
- `-ldflags="-s -w"` — Strip debug info, smaller binary (~8MB)
- `-o pinchtab` — Output filename
- `./cmd/pinchtab` — Build source directory

### Build with Version Info

```bash
VERSION=$(git describe --tags --always)
go build \
  -ldflags="-s -w -X main.version=$VERSION" \
  -o pinchtab \
  ./cmd/pinchtab

./pinchtab --version
# pinchtab 0.7.6 (or current version)
```

### Verify Build

```bash
./pinchtab --version
# pinchtab dev

./pinchtab help
# Shows help text
```

---

## Part 5: Run the Server

### Start Pinchtab (Headless Mode)

```bash
./pinchtab
```

**Expected output:**
```
🦀 PINCH! PINCH! port=9867 cdp= stealth=light
auth disabled (set BRIDGE_TOKEN to enable)
startup configuration bind=127.0.0.1 port=9867 headless=true stealth=light ...
tabs restored count=0
```

The server is now running on `http://127.0.0.1:9867`.

### Start with Visible Window (Headed Mode)

```bash
BRIDGE_HEADLESS=false ./pinchtab
```

Opens Chrome in the foreground so you can see what's happening.

### Start in Background

```bash
./pinchtab &
# [1] 12345
```

Or with nohup (survives terminal close):
```bash
nohup ./pinchtab > pinchtab.log 2>&1 &
tail -f pinchtab.log  # Watch logs
```

### Configuration

Common environment variables:

```bash
# Port
BRIDGE_PORT=9868 ./pinchtab

# Custom Chrome binary
CHROME_BINARY=/usr/bin/google-chrome ./pinchtab

# Custom profile directory
BRIDGE_PROFILE=~/.pinchtab/profile1 ./pinchtab

# Stealth mode
BRIDGE_STEALTH=full ./pinchtab

# Block ads
BRIDGE_BLOCK_ADS=true ./pinchtab

# API authentication
BRIDGE_TOKEN=secret-token ./pinchtab

# Multiple settings
BRIDGE_PORT=9868 BRIDGE_STEALTH=full BRIDGE_BLOCK_ADS=true ./pinchtab
```

---

## Part 6: Verify Installation

### Health Check

In another terminal:
```bash
curl http://localhost:9867/health
```

**Response:**
```json
{
  "status": "ok",
  "version": "dev",
  "uptime": 123
}
```

### Try a Command

```bash
# Navigate
curl -X POST http://localhost:9867/tab \
  -H "Content-Type: application/json" \
  -d '{"action":"new","url":"https://example.com"}'

# Extract text
curl http://localhost:9867/text
```

### Test with pinchtab CLI

```bash
./pinchtab help           # Show help
./pinchtab quick https://example.com  # Quick workflow
./pinchtab nav https://github.com     # Navigate
./pinchtab snap                       # Get snapshot
```

---

## Part 7: Development Setup

### Run Tests

```bash
go test ./...
```

**Output:**
```
ok  	github.com/pinchtab/pinchtab/cmd/pinchtab	1.234s
ok  	github.com/pinchtab/pinchtab/internal/bridge	2.567s
... more packages
```

### Run Tests with Coverage

```bash
go test ./... -v -coverprofile=coverage.out
go tool cover -html=coverage.out  # Open in browser
```

### Format Code

```bash
gofmt -w .
```

### Lint Code

```bash
golangci-lint run ./...
```

(Install first if needed):
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ~/bin v2.9.0
```

### Run Pre-Push Checks

```bash
./scripts/check.sh
```

**Runs:**
- gofmt check
- go vet
- Build
- Tests with coverage
- Lint (if installed)

---

## Part 8: Docker Build

### Build Docker Image

```bash
docker build -t pinchtab:latest .
```

**Build stages:**
1. Builder stage — Compiles Go binary
2. Runtime stage — Minimal Alpine with Chrome

### Run Docker Container

```bash
docker run -d -p 9867:9867 pinchtab:latest
```

### Verify Docker

```bash
curl http://localhost:9867/health
```

### Docker with Custom Configuration

```bash
docker run -d \
  -p 9868:9867 \
  -e BRIDGE_PORT=9867 \
  -e BRIDGE_STEALTH=full \
  -e BRIDGE_BLOCK_ADS=true \
  pinchtab:latest
```

---

## Common Build Issues

### Issue 1: "go: command not found"

**Problem:** Go not installed or not in PATH

**Solution:**
```bash
which go        # Should show path
go version      # Should show version

# If not found, install Go (see Prerequisites)
export PATH=$PATH:/usr/local/go/bin  # Add to PATH if needed
```

### Issue 2: "chromium-browser: command not found"

**Problem:** Chrome/Chromium not installed

**Solution:**
```bash
# macOS
brew install chromium

# Linux Ubuntu/Debian
sudo apt install chromium-browser

# Linux RHEL/CentOS
sudo yum install chromium
```

### Issue 3: Build Fails with "undefined reference"

**Problem:** Missing Go modules

**Solution:**
```bash
go mod download
go mod tidy
go build -o pinchtab ./cmd/pinchtab
```

### Issue 4: "permission denied" when running binary

**Problem:** Binary not executable

**Solution:**
```bash
chmod +x pinchtab
./pinchtab --version
```

### Issue 5: Port 9867 already in use

**Problem:** Another service using the port

**Solution:**
```bash
# Use different port
BRIDGE_PORT=9868 ./pinchtab

# Or find and kill the process
lsof -i :9867
kill -9 <PID>
```

### Issue 6: Chrome crashes on startup

**Problem:** Missing libraries or wrong flags

**Solution:**
```bash
# Try with debug flags
CHROME_FLAGS="--no-sandbox --disable-dev-shm-usage --disable-gpu" ./pinchtab

# Or use custom Chrome binary
CHROME_BINARY=/usr/bin/google-chrome ./pinchtab
```

---

## Build Variations

### Build for Different OS

**macOS (from any OS):**
```bash
GOOS=darwin GOARCH=amd64 go build -o pinchtab-darwin-amd64 ./cmd/pinchtab
```

**Linux (from any OS):**
```bash
GOOS=linux GOARCH=amd64 go build -o pinchtab-linux-amd64 ./cmd/pinchtab
```

**Windows (from any OS):**
```bash
GOOS=windows GOARCH=amd64 go build -o pinchtab-windows-amd64.exe ./cmd/pinchtab
```

### Static Build

```bash
CGO_ENABLED=0 GOOS=linux go build -o pinchtab-static ./cmd/pinchtab
```

Creates a fully static binary with no external dependencies.

---

## Development Workflow

### 1. Create Feature Branch

```bash
git checkout -b feat/my-feature
```

### 2. Make Changes

Edit Go files in `cmd/pinchtab/` or `internal/`

### 3. Run Tests

```bash
go test ./... -v
```

### 4. Run Checks

```bash
./scripts/check.sh
```

### 5. Commit Changes

```bash
git add .
git commit -m "feat: description of change"
```

### 6. Push Branch

```bash
git push origin feat/my-feature
```

### 7. Create Pull Request

On GitHub, create PR from your branch to `main`.

---

## Continuous Integration

### Pre-Commit Hook

Automatically runs checks before committing:

```bash
./scripts/setup-hooks.sh
```

Now `git commit` will run checks automatically.

### GitHub Actions

On push, GitHub Actions runs:
- Format check (gofmt)
- Vet check (go vet)
- Build
- Tests with coverage
- Lint check (golangci-lint)

See `.github/workflows/go-verify.yml` for details.

---

## Installation as CLI

### Install Globally (from source)

```bash
go build -o ~/go/bin/pinchtab ./cmd/pinchtab
```

Then use anywhere:
```bash
pinchtab help
pinchtab --version
```

### Install via npm (published releases)

```bash
npm install -g pinchtab
pinchtab --version
```

---

## Next Steps

1. **Run the server:** `./pinchtab`
2. **Try the CLI:** `./pinchtab quick https://example.com`
3. **Read the API:** See `docs/curl-commands.md`
4. **Check workflows:** See `docs/getting-started-api-workflows.md`
5. **Contribute:** Create a feature branch and submit a PR

---

## Troubleshooting

### Check Version

```bash
./pinchtab --version
```

### Check Configuration

```bash
./pinchtab help
```

### View Logs

```bash
./pinchtab &
# Watch output directly
```

Or save to file:
```bash
./pinchtab > pinchtab.log 2>&1 &
tail -f pinchtab.log
```

### Debug Mode

```bash
# Set verbose logging (if supported)
./pinchtab -v
./pinchtab --debug
```

### Test Health

```bash
curl http://localhost:9867/health | jq .
```

---

## Quick Start Script

Save as `build-and-run.sh`:

```bash
#!/bin/bash
set -e

echo "🔨 Building Pinchtab..."
go build -o pinchtab ./cmd/pinchtab

echo "✅ Build complete!"
echo ""
echo "🚀 Starting Pinchtab server..."
./pinchtab
```

Then run:
```bash
chmod +x build-and-run.sh
./build-and-run.sh
```

---

## Resources

- **GitHub Repository:** https://github.com/pinchtab/pinchtab
- **Go Documentation:** https://golang.org/doc/
- **Chrome DevTools Protocol:** https://chromedevtools.github.io/devtools-protocol/
- **Chromedp Library:** https://github.com/chromedp/chromedp

---

## Support

Having issues? Check:
1. System requirements met?
2. All dependencies installed?
3. Port 9867 available?
4. Chrome/Chromium working?

See `docs/` for more guides and examples.
