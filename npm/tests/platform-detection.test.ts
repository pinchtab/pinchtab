/**
 * Platform Detection Tests
 *
 * Verifies that the platform detection logic correctly maps Node.js process.platform/process.arch
 * to the goreleaser binary filenames.
 *
 * Matrix:
 *   process.platform | process.arch | Expected Binary
 *   ───────────────────────────────────────────────────────
 *   darwin          | x64          | pinchtab-darwin-amd64
 *   darwin          | arm64        | pinchtab-darwin-arm64
 *   linux           | x64          | pinchtab-linux-amd64
 *   linux           | arm64        | pinchtab-linux-arm64
 *   win32           | x64          | pinchtab-windows-amd64.exe
 *   win32           | arm64        | pinchtab-windows-arm64.exe
 */

import { test, describe } from 'node:test';
import * as assert from 'node:assert';

/**
 * Extracted detectPlatform logic from postinstall.js
 * (duplicated here for isolated testing)
 */
function detectPlatform(platform: string, arch: string) {
  // Only support x64 and arm64
  let mappedArch: string;
  if (arch === 'x64') {
    mappedArch = 'amd64';
  } else if (arch === 'arm64') {
    mappedArch = 'arm64';
  } else {
    throw new Error(`Unsupported architecture: ${arch}. Only x64 (amd64) and arm64 are supported.`);
  }

  const osMap: Record<string, string> = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const detectedOS = osMap[platform];
  if (!detectedOS) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os: detectedOS, arch: mappedArch };
}

interface PlatformResult {
  os: string;
  arch: string;
}

function getBinaryName(platform: PlatformResult): string {
  const { os, arch } = platform;
  const archName = arch === 'arm64' ? 'arm64' : 'amd64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

describe('Platform Detection', () => {
  describe('detectPlatform', () => {
    test('darwin + x64 → darwin-amd64', () => {
      const platform = detectPlatform('darwin', 'x64');
      assert.strictEqual(platform.os, 'darwin');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('darwin + arm64 → darwin-arm64', () => {
      const platform = detectPlatform('darwin', 'arm64');
      assert.strictEqual(platform.os, 'darwin');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('linux + x64 → linux-amd64', () => {
      const platform = detectPlatform('linux', 'x64');
      assert.strictEqual(platform.os, 'linux');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('linux + arm64 → linux-arm64', () => {
      const platform = detectPlatform('linux', 'arm64');
      assert.strictEqual(platform.os, 'linux');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('win32 + x64 → windows-amd64', () => {
      const platform = detectPlatform('win32', 'x64');
      assert.strictEqual(platform.os, 'windows');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('win32 + arm64 → windows-arm64', () => {
      const platform = detectPlatform('win32', 'arm64');
      assert.strictEqual(platform.os, 'windows');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('unsupported platform → error', () => {
      assert.throws(() => detectPlatform('freebsd', 'x64'), /Unsupported platform: freebsd/);
    });

    test('unsupported arch → error', () => {
      assert.throws(() => detectPlatform('linux', 'ia32'), /Unsupported architecture: ia32/);
    });
  });

  describe('getBinaryName', () => {
    test('darwin-amd64 → pinchtab-darwin-amd64', () => {
      const platform = { os: 'darwin', arch: 'amd64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-darwin-amd64');
    });

    test('darwin-arm64 → pinchtab-darwin-arm64', () => {
      const platform = { os: 'darwin', arch: 'arm64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-darwin-arm64');
    });

    test('linux-amd64 → pinchtab-linux-amd64', () => {
      const platform = { os: 'linux', arch: 'amd64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-linux-amd64');
    });

    test('linux-arm64 → pinchtab-linux-arm64', () => {
      const platform = { os: 'linux', arch: 'arm64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-linux-arm64');
    });

    test('windows-amd64 → pinchtab-windows-amd64.exe', () => {
      const platform = { os: 'windows', arch: 'amd64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-windows-amd64.exe');
    });

    test('windows-arm64 → pinchtab-windows-arm64.exe', () => {
      const platform = { os: 'windows', arch: 'arm64' };
      const name = getBinaryName(platform);
      assert.strictEqual(name, 'pinchtab-windows-arm64.exe');
    });
  });

  describe('Full Matrix (detectPlatform + getBinaryName)', () => {
    interface MatrixEntry {
      nodejs_platform: string;
      nodejs_arch: string;
      expected_binary: string;
    }

    const matrix: MatrixEntry[] = [
      { nodejs_platform: 'darwin', nodejs_arch: 'x64', expected_binary: 'pinchtab-darwin-amd64' },
      { nodejs_platform: 'darwin', nodejs_arch: 'arm64', expected_binary: 'pinchtab-darwin-arm64' },
      { nodejs_platform: 'linux', nodejs_arch: 'x64', expected_binary: 'pinchtab-linux-amd64' },
      { nodejs_platform: 'linux', nodejs_arch: 'arm64', expected_binary: 'pinchtab-linux-arm64' },
      {
        nodejs_platform: 'win32',
        nodejs_arch: 'x64',
        expected_binary: 'pinchtab-windows-amd64.exe',
      },
      {
        nodejs_platform: 'win32',
        nodejs_arch: 'arm64',
        expected_binary: 'pinchtab-windows-arm64.exe',
      },
    ];

    matrix.forEach(({ nodejs_platform, nodejs_arch, expected_binary }) => {
      test(`${nodejs_platform}/${nodejs_arch} → ${expected_binary}`, () => {
        const platform = detectPlatform(nodejs_platform, nodejs_arch);
        const binary = getBinaryName(platform);
        assert.strictEqual(binary, expected_binary);
      });
    });
  });
});
