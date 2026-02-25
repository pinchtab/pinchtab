import * as path from 'path';
import * as os from 'os';

export interface Platform {
  os: 'darwin' | 'linux' | 'windows';
  arch: 'amd64' | 'arm64';
}

export function detectPlatform(): Platform {
  const platform = process.platform as any;
  const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';

  const osMap: Record<string, 'darwin' | 'linux' | 'windows'> = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const os_name = osMap[platform];
  if (!os_name) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os: os_name, arch };
}

export function getBinaryName(platform: Platform): string {
  const { os, arch } = platform;
  const archName = arch === 'arm64' ? 'arm64' : 'amd64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

export function getBinDir(): string {
  return path.join(process.env.HOME || process.env.USERPROFILE || '', '.pinchtab', 'bin');
}

export function getBinaryPath(binaryName: string, version?: string): string {
  // Allow override via environment variable (for Docker, custom builds, etc.)
  if (process.env.PINCHTAB_BINARY_PATH) {
    return process.env.PINCHTAB_BINARY_PATH;
  }

  // Version-specific path: ~/.pinchtab/bin/0.7.0/pinchtab-darwin-arm64
  // This allows multiple versions to coexist and prevents silent overwrites
  if (version) {
    return path.join(getBinDir(), version, binaryName);
  }

  // Fallback to version-less for backwards compat
  return path.join(getBinDir(), binaryName);
}
