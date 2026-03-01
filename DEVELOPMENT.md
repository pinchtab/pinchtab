# Development Setup

## Prerequisites

- **Go 1.25+**
- **Python 3.8+** (for pre-commit)
- **Git**

## Setup

### 1. Clone the repository

```bash
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab
```

### 2. Install pre-commit hooks

This ensures gofmt and linting checks run **before** you commit.

```bash
# Install pre-commit framework (one-time)
pip install pre-commit
# or: brew install pre-commit

# Setup git hooks in this repo
pre-commit install

# (Optional) Run hooks on all files to verify setup
pre-commit run --all-files
```

### 3. Verify Go environment

```bash
go version  # Should be 1.25+
go mod download
```

## Before Committing

The pre-commit hooks will automatically run on `git commit`. If you need to manually format:

```bash
# Format all Go code
./scripts/format.sh

# Or directly
gofmt -w .

# Run linters manually
pre-commit run --all-files
```

## Common Issues

### "pre-commit: command not found"

Install pre-commit:
```bash
pip install pre-commit
# or
brew install pre-commit
```

Then setup hooks:
```bash
pre-commit install
```

### gofmt fails in CI even though pre-commit passed locally

- Ensure you ran `pre-commit install` (not just installed the tool)
- Verify hooks are installed: `cat .git/hooks/pre-commit`
- Try updating: `pre-commit autoupdate`

### Tests failing locally

```bash
# Run full test suite
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestName ./...
```

## Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Code Style

- **Format:** `gofmt` (automatic via pre-commit)
- **Lint:** `golangci-lint` (automatic via pre-commit)
- **Docs:** Files in `docs/` should follow Markdown standards (checked via pre-commit)

## Git Workflow

```bash
# 1. Create branch
git checkout -b feature/your-feature

# 2. Make changes
# ... edit files ...

# 3. Format and test
./scripts/format.sh
go test ./...

# 4. Commit (pre-commit hooks run automatically)
git commit -m "feat: description"

# 5. Push
git push origin feature/your-feature

# 6. Create Pull Request on GitHub
```

## Documentation

Update docs when adding features:

```bash
# Docs location
docs/
├── core-concepts.md
├── get-started.md
├── references/
├── architecture/
└── guides/
```

Validate docs: `./scripts/check-docs-json.sh`

## Useful Commands

```bash
# Format without committing
gofmt -w .

# Check what would be formatted
gofmt -l .

# Run specific linter
golangci-lint run

# Clean build artifacts
go clean

# Update dependencies
go get -u ./...

# List all test functions
go test -list .
```

## Getting Help

- Read the [Overview](docs/overview.md)
- Check [Architecture](docs/architecture/pinchtab-architecture.md)
- See [API Reference](docs/references/instance-api.md)
- Browse [Guides](docs/guides/)
