import * as fs from 'fs';
import * as path from 'path';
import * as https from 'https';

const GITHUB_REPO = 'pinchtab/pinchtab';
const VERSION = '0.7.0';

interface Platform {
  os: 'darwin' | 'linux' | 'windows';
  arch: 'x64' | 'arm64';
}

function detectPlatform(): Platform {
  const platform = process.platform as any;
  const arch = process.arch === 'arm64' ? 'arm64' : 'x64';

  const osMap: Record<string, 'darwin' | 'linux' | 'windows'> = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const os = osMap[platform];
  if (!os) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os, arch };
}

function getBinaryName(platform: Platform): string {
  const { os, arch } = platform;
  const archName = arch === 'arm64' ? 'arm64' : 'x64';
  
  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

function getBinDir(): string {
  return path.join(process.env.HOME || process.env.USERPROFILE || '', '.pinchtab', 'bin');
}

async function downloadBinary(platform: Platform): Promise<void> {
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
  console.log(`URL: ${downloadUrl}`);

  // Ensure directory exists
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  return new Promise((resolve, reject) => {
    https
      .get(downloadUrl, (response) => {
        if (response.statusCode === 404) {
          reject(
            new Error(
              `Binary not found: ${downloadUrl}. Make sure v${VERSION} is released on GitHub.`
            )
          );
          return;
        }

        if (response.statusCode !== 200) {
          reject(new Error(`Download failed with status ${response.statusCode}`));
          return;
        }

        const file = fs.createWriteStream(binaryPath);
        response.pipe(file);

        file.on('finish', () => {
          file.close();
          // Make executable
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
        reject(err);
      });
  });
}

export async function ensureBinary(): Promise<string> {
  const platform = detectPlatform();
  await downloadBinary(platform);
  const binDir = getBinDir();
  const binaryName = getBinaryName(platform);
  return path.join(binDir, binaryName);
}
