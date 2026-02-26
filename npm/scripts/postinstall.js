#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');

const GITHUB_REPO = 'pinchtab/pinchtab';

function getVersion() {
  const pkgPath = path.join(__dirname, '..', 'package.json');
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
  return pkg.version;
}

function detectPlatform() {
  const platform = process.platform;
  const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';

  const osMap = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const detectedOS = osMap[platform];
  if (!detectedOS) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os: detectedOS, arch };
}

function getBinaryName(platform) {
  const { os, arch } = platform;
  const archName = arch === 'arm64' ? 'arm64' : 'amd64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

function getBinaryPath(binaryName) {
  // Allow override via environment variable (useful for Docker, dev, containers)
  if (process.env.PINCHTAB_BINARY_PATH) {
    return process.env.PINCHTAB_BINARY_PATH;
  }

  return path.join(os.homedir(), '.pinchtab', 'bin', binaryName);
}

function getBinDir() {
  return path.join(os.homedir(), '.pinchtab', 'bin');
}

function fetchUrl(url, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    const attemptFetch = (currentUrl, redirectsRemaining) => {
      const httpsOptions = new URL(currentUrl);

      // Proxy support for corporate environments
      if (process.env.HTTPS_PROXY || process.env.HTTP_PROXY) {
        const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;
        try {
          const proxy = new URL(proxyUrl);
          httpsOptions.agent = new https.Agent({
            host: proxy.hostname,
            port: proxy.port,
            keepAlive: true,
          });
        } catch (err) {
          console.warn(`Warning: Invalid proxy URL ${proxyUrl}, ignoring`);
        }
      }

      https
        .get(currentUrl, httpsOptions, (response) => {
          // Handle redirects (301, 302, 307, 308)
          if ([301, 302, 307, 308].includes(response.statusCode)) {
            if (redirectsRemaining <= 0) {
              reject(new Error(`Too many redirects from ${currentUrl}`));
              return;
            }

            const redirectUrl = response.headers.location;
            if (!redirectUrl) {
              reject(new Error(`Redirect without location header from ${currentUrl}`));
              return;
            }

            // Consume the response stream to avoid memory leaks
            response.resume();
            attemptFetch(redirectUrl, redirectsRemaining - 1);
            return;
          }

          if (response.statusCode === 404) {
            reject(new Error(`Not found: ${currentUrl}`));
            return;
          }

          if (response.statusCode !== 200) {
            reject(new Error(`HTTP ${response.statusCode}: ${currentUrl}`));
            return;
          }

          const chunks = [];
          response.on('data', (chunk) => chunks.push(chunk));
          response.on('end', () => resolve(Buffer.concat(chunks)));
          response.on('error', reject);
        })
        .on('error', reject);
    };

    attemptFetch(url, maxRedirects);
  });
}

async function downloadChecksums(version) {
  const url = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/checksums.txt`;

  try {
    const data = await fetchUrl(url);
    const checksums = new Map();

    data
      .toString('utf-8')
      .trim()
      .split('\n')
      .forEach((line) => {
        const parts = line.split(/\s+/);
        if (parts.length >= 2) {
          const hash = parts[0];
          const filename = parts[1];
          checksums.set(filename.trim(), hash.trim());
        }
      });

    return checksums;
  } catch (err) {
    throw new Error(
      `Failed to download checksums: ${err.message}\n` +
        `Ensure v${version} is released on GitHub with checksums.txt`
    );
  }
}

function verifySHA256(filePath, expectedHash) {
  const hash = crypto.createHash('sha256');
  const data = fs.readFileSync(filePath);
  hash.update(data);
  const actualHash = hash.digest('hex');
  return actualHash.toLowerCase() === expectedHash.toLowerCase();
}

async function downloadBinary(platform, version) {
  const binaryName = getBinaryName(platform);
  const binDir = getBinDir();
  const versionDir = path.join(binDir, version);
  const binaryPath = path.join(versionDir, binaryName);

  // Skip if already exists
  if (fs.existsSync(binaryPath)) {
    console.log(`✓ Pinchtab binary already present: ${binaryPath}`);
    return;
  }

  // Fetch checksums
  console.log(`Downloading Pinchtab ${version} for ${platform.os}-${platform.arch}...`);
  const checksums = await downloadChecksums(version);

  if (!checksums.has(binaryName)) {
    throw new Error(
      `Binary not found in checksums: ${binaryName}\n` +
        `Available: ${Array.from(checksums.keys()).join(', ')}\n` +
        `\nMake sure v${version} release has binaries compiled (not just Docker images).`
    );
  }

  const expectedHash = checksums.get(binaryName);
  const downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/${binaryName}`;

  // Ensure version-specific directory exists
  if (!fs.existsSync(versionDir)) {
    fs.mkdirSync(versionDir, { recursive: true });
  }

  // Download binary
  return new Promise((resolve, reject) => {
    console.log(`Downloading from ${downloadUrl}...`);

    const file = fs.createWriteStream(binaryPath);
    let redirectCount = 0;
    const maxRedirects = 5;

    const performDownload = (url) => {
      https
        .get(url, (response) => {
          // Handle redirects (301, 302, 307, 308)
          if ([301, 302, 307, 308].includes(response.statusCode)) {
            if (redirectCount >= maxRedirects) {
              fs.unlink(binaryPath, () => {});
              reject(new Error(`Too many redirects downloading ${downloadUrl}`));
              return;
            }

            const redirectUrl = response.headers.location;
            if (!redirectUrl) {
              fs.unlink(binaryPath, () => {});
              reject(new Error(`Redirect without location header from ${url}`));
              return;
            }

            redirectCount++;
            response.resume(); // Consume response stream
            performDownload(redirectUrl);
            return;
          }

          if (response.statusCode !== 200) {
            fs.unlink(binaryPath, () => {});
            reject(new Error(`HTTP ${response.statusCode}: ${url}`));
            return;
          }

          response.pipe(file);

          file.on('finish', () => {
            file.close();

            // Verify checksum
            if (!verifySHA256(binaryPath, expectedHash)) {
              fs.unlink(binaryPath, () => {});
              reject(
                new Error(
                  `Checksum verification failed for ${binaryName}\n` +
                    `Downloaded file may be corrupted. Please try installing again.`
                )
              );
              return;
            }

            // Make executable
            fs.chmodSync(binaryPath, 0o755);
            console.log(`✓ Verified and installed: ${binaryPath}`);
            resolve();
          });

          file.on('error', (err) => {
            fs.unlink(binaryPath, () => {});
            reject(err);
          });
        })
        .on('error', reject);
    };

    performDownload(downloadUrl);
  });
}

// Main
(async () => {
  try {
    const platform = detectPlatform();
    const version = getVersion();

    // Ensure binary was successfully downloaded
    // (If PINCHTAB_BINARY_PATH is set, skip download but trust the binary exists)
    if (!process.env.PINCHTAB_BINARY_PATH) {
      const binaryPath = getBinaryPath(getBinaryName(platform));
      const binDir = path.dirname(binaryPath);

      // Create version-specific directory
      const versionDir = path.join(binDir, version);
      if (!fs.existsSync(versionDir)) {
        fs.mkdirSync(versionDir, { recursive: true });
      }

      // Try to download, but don't fail if release doesn't exist yet
      // (useful during CI/release workflows)
      try {
        await downloadBinary(platform, version);
      } catch (downloadErr) {
        const errMsg = downloadErr instanceof Error ? downloadErr.message : String(downloadErr);
        // If it's a 404 (release doesn't exist), warn but don't fail
        if (errMsg.includes('404') || errMsg.includes('Not found')) {
          console.warn('\n⚠ Pinchtab binary not yet available (release in progress).');
          console.warn('  On first use, run: npm rebuild pinchtab');
          process.exit(0);
        }
        // Real errors should fail
        throw downloadErr;
      }

      // Verify binary exists after download
      const finalPath = path.join(versionDir, getBinaryName(platform));
      if (!fs.existsSync(finalPath)) {
        throw new Error(
          `Binary was not successfully downloaded to ${finalPath}\n` +
            `This usually means the GitHub release doesn't have the binary for your platform.`
        );
      }
    }

    console.log('✓ Pinchtab setup complete');
    process.exit(0);
  } catch (err) {
    console.error('\n✗ Failed to setup Pinchtab:');
    console.error(
      `  ${(err instanceof Error ? err.message : String(err)).split('\n').join('\n  ')}`
    );
    console.error('\nTroubleshooting:');
    console.error('  • Check your internet connection');
    console.error('  • Verify the release exists: https://github.com/pinchtab/pinchtab/releases');
    console.error('  • Try again: npm rebuild pinchtab');
    if (process.env.HTTPS_PROXY || process.env.HTTP_PROXY) {
      console.error('  • Check proxy settings (HTTPS_PROXY / HTTP_PROXY)');
    }
    console.error('\nFor Docker or custom binaries:');
    console.error('  export PINCHTAB_BINARY_PATH=/path/to/pinchtab');
    console.error('  npm rebuild pinchtab');
    process.exit(1);
  }
})();
