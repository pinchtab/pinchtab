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

**Performance notes:**
- CI runs in ~1-2 minutes (GitHub runners)
- Local runs may take 3-5+ minutes depending on your machine
- **Tip:** If it's too slow, just push and let CI run it (with conditional execution, it only runs when Go files change)

**CI behavior:**
- ✅ Only runs when Go files (`.go`, `go.mod`, `go.sum`) change
- ⏭️ Skipped when only docs/workflows change (path filtering)
- Same exclusions as local scan
- Faster on GitHub runners (optimized VMs)

**First run:**
- Installs gosec to `~/go/bin/gosec`
- Results saved to `gosec-results.json`
- Exits with code 1 if critical issues found (same as CI)

**Alternative:**
If gosec is too slow locally, you can:
1. Just push your changes - CI will run it (only on Go changes)
2. Use our conditional workflow - it skips when no Go files changed
3. Trust the CI check - it's fast and runs on every PR
