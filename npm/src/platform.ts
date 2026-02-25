import * as path from 'path';
import * as os from 'os';

export interface Platform {
  os: 'darwin' | 'linux' | 'windows';
  arch: 'x64' | 'arm64';
}

export function detectPlatform(): Platform {
  const platform = process.platform as any;
  const arch = process.arch === 'arm64' ? 'arm64' : 'x64';

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
  const archName = arch === 'arm64' ? 'arm64' : 'x64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

export function getBinDir(): string {
  return path.join(process.env.HOME || process.env.USERPROFILE || '', '.pinchtab', 'bin');
}

export function getBinaryPath(binaryName: string): string {
  // Allow override via environment variable
  if (process.env.PINCHTAB_BINARY_PATH) {
    return process.env.PINCHTAB_BINARY_PATH;
  }

  return path.join(getBinDir(), binaryName);
}
