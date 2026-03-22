# Release Process

**⚠️ CRITICAL:** The npm package depends on Go binaries being in the GitHub release. The release pipeline will fail hard if goreleaser doesn't upload binaries — npm users won't get a working binary.

Pinchtab uses an automated CI/CD pipeline triggered by Git tags. When you push a tag like `v0.7.0`, GitHub Actions:

1. **Builds Go binaries** — darwin-arm64, darwin-x64, linux-x64, linux-arm64, windows-x64
2. **Creates GitHub release** — with checksums.txt for integrity verification
3. **Publishes to npm** — TypeScript SDK with auto-download postinstall script
4. **Builds Docker images** — linux/amd64, linux/arm64
5. **Publishes the ClawHub skill** — after npm succeeds

## Prerequisites

### Secrets (configure once in GitHub)

Go to **Settings → Secrets and variables → Actions** and add:

- **NPM_TOKEN** — npm authentication token
  - Create at https://npmjs.com/settings/~/tokens
  - Scope: `automation` (publish + read)

- **DOCKERHUB_USER** — Docker Hub username (if using Docker Hub)
- **DOCKERHUB_TOKEN** — Docker Hub personal access token

### Local setup

```bash
# 1. Ensure main branch is up to date
git checkout main && git pull origin main

# 2. Merge feature branches
# (all features should be on main before tagging)

# 3. Verify version consistency
cat npm/package.json | jq .version # npm package
cat go.mod | grep "module"         # Go module
git describe --tags                # latest tag
```

## Pre-Release Checklist

Goreleaser is already configured correctly. Verify before tagging:

```bash
# 1. Check builds section (compiles for all platforms)
grep -A 8 "^builds:" .goreleaser.yml
# Should output: linux/darwin/windows + amd64/arm64

# 2. Check archives use binary format (required for npm)
grep -A 5 "^archives:" .goreleaser.yml
# Should show: format: binary (not tar/zip)

# 3. Verify checksums enabled
grep "checksum:" .goreleaser.yml
# Should output: name_template: checksums.txt

# 4. Check release config
grep -A 2 "^release:" .goreleaser.yml
# Should output: GitHub owner/repo
```

**Configuration status:**
- ✅ `builds:` section compiles all platforms (darwin-arm64, darwin-amd64, linux-arm64, linux-amd64, windows-amd64, windows-arm64)
- ✅ `archives:` outputs binaries directly: `pinchtab-darwin-arm64`, `pinchtab-linux-x64`, etc.
- ✅ `checksum:` generates `checksums.txt` (used by npm postinstall verification)
- ✅ `release:` points to GitHub (uploads everything automatically)

**Ready to release.** The recommended path is now the manual `Release` workflow, which runs the full verification suite, pauses for approval, creates the tag, and then finishes publishing in the same run.

## Releasing

## Manual Release Flow

1. Choose the branch, tag, or SHA you want to release.
   `Release` now validates the requested version format and tag availability, then rewrites the npm package version inside the CI workspace for the dry-run steps.
2. Open **Actions → Release**.
3. Run it with:
   - `version`: the release version, for example `0.8.0`
   - `ref`: the branch, tag, or SHA you want to release
4. The workflow runs:
   - Go checks
   - dashboard checks
   - npm package verification
   - docs verification
   - full release E2E
   - Docker bootstrap smoke test
   - GoReleaser snapshot
   - `npm publish --dry-run`
5. After those pass, GitHub pauses at the `release-approval` environment gate.
6. Approve the run. The workflow creates and pushes `v0.8.0` from the exact tested commit SHA, then continues publishing from that tag in the same run.
7. Monitor the remaining publish jobs in the same workflow run:
   - GitHub release binaries
   - npm publish
   - Docker image publish
   - ClawHub skill publish

### Required GitHub setup

Configure a protected environment named `release-approval` with the required reviewers for the manual gate to be effective.

### Recovery publish

If a tag already exists and you need to retry publishing, use **Actions → Release / Manual Publish** with the existing tag, for example `v0.8.0`.

## Pipeline details

### 1. Goreleaser (Go binary) — CRITICAL for npm

Triggered on `v*` tag push. Builds binaries and creates GitHub release.

**What it does:**
- ✅ Compiles for all platforms (darwin-arm64, darwin-x64, linux-x64, linux-arm64, windows-x64)
- ✅ Generates `checksums.txt` (SHA256)
- ✅ **Uploads binaries to GitHub Releases** ← Required by npm postinstall!
- Also: Docker images, changelog, etc.

**⚠️ CRITICAL:** npm postinstall script downloads binaries from GitHub Releases. If the release doesn't have the binaries (e.g., only Docker images), `npm install pinchtab` will fail silently and the binary won't be available.

**Configured in:** `.goreleaser.yml`

**Verify release has binaries:**
```bash
curl -s https://api.github.com/repos/pinchtab/pinchtab/releases/v0.7.0 | jq '.assets[].name'
# Should output: pinchtab-darwin-arm64, pinchtab-darwin-x64, etc.
```

### 2. npm publish

Depends on: `release` job (waits for goreleaser to finish)

**What it does:**
- Syncs version from tag (v0.7.0 → 0.7.0)
- Builds TypeScript (`npm run build`)
- Publishes to npm registry
- Users get postinstall script that downloads binaries from GitHub Releases

**User flow on `npm install pinchtab`:**
```
1. npm downloads @pinchtab/cli package
2. Runs postinstall script
3. Script detects OS/arch (darwin-arm64, linux-x64, etc.)
4. Downloads binary from GitHub release
5. Verifies SHA256 checksum
6. Stores in ~/.pinchtab/bin/0.7.0/pinchtab-<os>-<arch>
7. Makes executable
8. If ANY STEP fails → npm install fails (exit 1)
```

**⚠️ REQUIRES:** Goreleaser must have successfully uploaded binaries to GitHub release. The npm postinstall will verify this and fail hard if binaries are missing.

### 3. Docker

Depends on: `release` and `npm`

GitHub Actions builds the release image directly from the tagged source with `docker buildx`.
The workflow pushes the same multi-arch build to both GHCR and Docker Hub, but only after npm has already published successfully.

### 4. ClawHub skill

Depends on: `release` and `npm`

The main `Release` workflow publishes the Pinchtab skill to ClawHub automatically after npm succeeds.
It does not consume npm artifacts or GitHub release binaries directly; the ordering is a policy choice so npm stays the first irreversible publish step.
The standalone `Publish Skill` workflow remains available for retries or one-off recovery publishes.

## Troubleshooting

### npm publish fails (403)

- Check **NPM_TOKEN** is set in secrets
- Verify token has `automation` scope
- Check you're not already published (can't overwrite existing version)

### Binary checksum mismatch

- goreleaser must generate `checksums.txt`
- Verify `.goreleaser.yml` has `checksum:` section
- Check GitHub release includes `checksums.txt`

### Docker push fails

- Verify DOCKERHUB_USER and DOCKERHUB_TOKEN
- Check token has permission to push

## Rolling back

If something goes wrong:

```bash
# Delete the tag locally and on GitHub
git tag -d v0.7.1
git push origin :refs/tags/v0.7.1

# Delete npm version (requires owner permission)
npm unpublish pinchtab@0.7.1

# Revert any commits
git revert <commit>
git push origin main

# Retag when ready
git tag v0.7.1
git push origin v0.7.1
```

## Version strategy

- Use **semantic versioning**: v0.7.0 (major.minor.patch)
- Tag on main branch only
- One tag = one release (all artifacts)
- npm version must match Go binary tag

## See also

- `.github/workflows/release.yml` — GitHub Actions workflow
- `.goreleaser.yml` — Go binary release config
- `npm/package.json` — npm package metadata
