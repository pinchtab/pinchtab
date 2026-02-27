# Scripts

Development and CI helper scripts.

## Security Scanning

### `run-gosec.sh`

Run the gosec security scanner locally with the same configuration as CI.

**Usage:**
```bash
./scripts/run-gosec.sh
```

**What it checks:**
- G112 (Slowloris vulnerability)
- G204 (Command injection)

**Notes:**
- First run will install gosec to `~/go/bin/gosec`
- Scan takes 1-2 minutes (same as CI)
- Results saved to `gosec-results.json`
- Exits with code 1 if critical issues found (same as CI)

**CI behavior:**
- Only runs when Go files (`.go`, `go.mod`, `go.sum`) change
- Skipped when only docs/workflows change (path filtering)
- Same exclusions as local scan

**Tip:** Run this before pushing to catch security issues early!
