# Release Process

Pinchtab uses an automated CI/CD pipeline triggered by Git tags. When you push a tag like `v0.7.0`, GitHub Actions:

1. **Builds Go binaries** — darwin-arm64, darwin-x64, linux-x64, linux-arm64, windows-x64
2. **Creates GitHub release** — with checksums.txt for integrity verification
3. **Publishes to npm** — TypeScript SDK with auto-download postinstall script
4. **Builds Docker images** — linux/amd64, linux/arm64

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
cat package.json | jq .version     # npm package
cat go.mod | grep "module"         # Go module
git describe --tags                # latest tag
```

## Releasing

### For patch/minor versions (recommended)

```bash
# 1. Bump version in all places
npm version patch   # or minor, major
git push origin main

# 2. Create tag
git tag v0.7.1
git push origin v0.7.1
```

### For manual releases

```bash
# 1. Tag directly
git tag -a v0.7.1 -m "Release v0.7.1"
git push origin v0.7.1

# 2. Or via GitHub UI: Releases → Create from tag
#    (workflow will auto-create if not present)
```

### Using workflow_dispatch (manual trigger)

If you need to re-release an existing tag:

1. Go to **Actions → Release**
2. **Run workflow**
3. Enter tag (e.g. `v0.7.1`)

## Pipeline details

### 1. Goreleaser (Go binary)

Triggered on `v*` tag push. Builds binaries and creates GitHub release.

**What it does:**
- Compiles for all platforms
- Generates `checksums.txt` (SHA256)
- Uploads to GitHub Releases

**Configured in:** `.goreleaser.yml`

### 2. npm publish

Depends on: `release` job (waits for goreleaser to finish)

**What it does:**
- Syncs version from tag (v0.7.0 → 0.7.0)
- Builds TypeScript (`npm run build`)
- Publishes to npm registry
- Postinstall script will download binaries from GitHub Releases

**Key point:** Users who `npm install pinchtab` will:
```bash
1. Download npm package
2. Run postinstall script
3. Postinstall fetches binary from GitHub releases
4. Verifies checksum (SHA256)
5. Makes executable
```

### 3. Docker

Independent job. Pushes to Docker Hub if configured.

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
