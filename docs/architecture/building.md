# Contributing

Guide to build PinchTab from source and contribute to the project.

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

### Install Chrome/Chromium

**macOS (Homebrew):**
```bash
brew install chromium
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt install -y chromium-browser
```

**Linux (RHEL/CentOS):**
```bash
sudo yum install -y chromium
```

### Verify Installations

```bash
go version       # go version go1.25.0 darwin/arm64
git --version    # git version 2.39.0
chromium --version  # Chromium 120.0.6099.xx
```

### Install Dependencies

Clone the repository and download Go modules:

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
go mod download
```

Verify:
```bash
go mod verify    # Verifies all checksums
go list -m all   # List all dependencies
```

---

## Part 2: Build the Project

### Simple Build

```bash
go build -o pinchtab ./cmd/pinchtab
```

**What it does:**
- Compiles Go source code
- Produces binary: `./pinchtab`
- Takes ~30-60 seconds

**Verify:**
```bash
ls -la pinchtab
./pinchtab --version
```

---

## Part 3: Run the Server

### Start (Headless)

```bash
./pinchtab
```

**Expected output:**
```
🦀 PINCH! PINCH! port=9867
auth disabled (set BRIDGE_TOKEN to enable)
```

### Start (Headed Mode)

```bash
BRIDGE_HEADLESS=false ./pinchtab
```

Opens Chrome in the foreground.

### Background

```bash
nohup ./pinchtab > pinchtab.log 2>&1 &
tail -f pinchtab.log  # Watch logs
```

---

## Part 4: Quick Test

### Health Check

```bash
curl http://localhost:9867/health
```

### Try CLI

```bash
./pinchtab quick https://example.com
./pinchtab nav https://github.com
./pinchtab snap
```

---

## Development

### Run Tests

```bash
go test ./... -v
go test ./... -v -coverprofile=coverage.out
go tool cover -html=coverage.out  # View coverage
```

### Code Quality

```bash
gofmt -w .              # Format code
golangci-lint run ./... # Lint (install if needed)
./scripts/check.sh      # Run all checks
```

### Pre-Commit Hook

```bash
./scripts/setup-hooks.sh
```

Automatically runs checks before committing.

### Development Workflow

```bash
# 1. Create feature branch
git checkout -b feat/my-feature

# 2. Make changes
# ... edit files ...

# 3. Test
go test ./... -v

# 4. Format & lint
gofmt -w .
golangci-lint run ./...

# 5. Commit
git add .
git commit -m "feat: description"

# 6. Push
git push origin feat/my-feature

# 7. Create PR on GitHub
```

---

## Continuous Integration

GitHub Actions automatically runs on push:
- Format checks (gofmt)
- Vet checks (go vet)
- Build verification
- Full test suite with coverage
- Linting (golangci-lint)

See `.github/workflows/` for details.

---

## Installation as CLI

### From Source

```bash
go build -o ~/go/bin/pinchtab ./cmd/pinchtab
```

Then use anywhere:
```bash
pinchtab help
pinchtab --version
```

### Via npm (released builds)

```bash
npm install -g pinchtab
pinchtab --version
```

---

## Resources

- **GitHub Repository:** https://github.com/pinchtab/pinchtab
- **Go Documentation:** https://golang.org/doc/
- **Chrome DevTools Protocol:** https://chromedevtools.github.io/devtools-protocol/
- **Chromedp Library:** https://github.com/chromedp/chromedp

---

## Support

Issues? Check:
1. All dependencies installed?
2. Go and Chrome versions correct?
3. Port 9867 available?
4. Check logs: `tail -f pinchtab.log`

See `docs/` for guides and examples.
