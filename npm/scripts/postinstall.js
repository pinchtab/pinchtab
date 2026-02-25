#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const os = require('os');

const GITHUB_REPO = 'pinchtab/pinchtab';
const VERSION = '0.7.0';

function detectPlatform() {
  const platform = process.platform;
  const arch = process.arch === 'arm64' ? 'arm64' : 'x64';

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
  const archName = arch === 'arm64' ? 'arm64' : 'x64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

function getBinDir() {
  return path.join(os.homedir(), '.pinchtab', 'bin');
}

async function downloadBinary(platform) {
  const binaryName = getBinaryName(platform);
  const binDir = getBinDir();
  const binaryPath = path.join(binDir, binaryName);

  // Skip if already exists
  if (fs.existsSync(binaryPath)) {
    console.log(`✓ Pinchtab binary already present: ${binaryPath}`);
    return;
  }

  const downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${binaryName}`;

  console.log(`Downloading Pinchtab ${VERSION} for ${platform.os}-${platform.arch}...`);

  // Ensure directory exists
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  return new Promise((resolve, reject) => {
    https
      .get(downloadUrl, (response) => {
        if (response.statusCode === 404) {
          console.warn(
            `⚠ Binary not found at ${downloadUrl}. Make sure v${VERSION} is released on GitHub.`
          );
          resolve();
          return;
        }

        if (response.statusCode !== 200) {
          console.warn(`⚠ Download failed with status ${response.statusCode}`);
          resolve();
          return;
        }

        const file = fs.createWriteStream(binaryPath);
        response.pipe(file);

        file.on('finish', () => {
          file.close();
          fs.chmodSync(binaryPath, 0o755);
          console.log(`✓ Downloaded to ${binaryPath}`);
          resolve();
        });

        file.on('error', (err) => {
          fs.unlink(binaryPath, () => {});
          reject(err);
        });
      })
      .on('error', (err) => {
        console.warn(`⚠ Failed to download: ${err.message}`);
        resolve();
      });
  });
}

// Run download
const platform = detectPlatform();
downloadBinary(platform)
  .then(() => {
    console.log('✓ Pinchtab setup complete');
    process.exit(0);
  })
  .catch((err) => {
    console.error('✗ Setup error:', err.message);
    process.exit(1);
  });
