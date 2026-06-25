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
import * as path from 'node:path';
import {
  detectPlatform,
  findRepoRoot,
  getBinaryName,
  getCheckoutBinaryPath,
  Platform,
} from '../src/platform';

function getTestRepoRoot(): string {
  const repoRoot = findRepoRoot(__dirname);
  if (!repoRoot) {
    throw new Error(`Could not find repo root from ${__dirname}`);
  }
  return repoRoot;
}

describe('Platform Detection', () => {
  describe('source-checkout binary lookup', () => {
    const repoRoot = getTestRepoRoot();

    test('finds repo root from npm/bin', () => {
      const fromDir = path.join(repoRoot, 'npm', 'bin');
      assert.strictEqual(findRepoRoot(fromDir), repoRoot);
      assert.strictEqual(getCheckoutBinaryPath(fromDir), path.join(repoRoot, 'pinchtab-dev'));
    });

    test('finds repo root from built SDK output under npm/dist/src', () => {
      const fromDir = path.join(repoRoot, 'npm', 'dist', 'src');
      assert.strictEqual(findRepoRoot(fromDir), repoRoot);
      assert.strictEqual(getCheckoutBinaryPath(fromDir), path.join(repoRoot, 'pinchtab-dev'));
    });
  });

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
    const cases: Array<[Platform, string]> = [
      [{ os: 'darwin', arch: 'amd64' }, 'pinchtab-darwin-amd64'],
      [{ os: 'darwin', arch: 'arm64' }, 'pinchtab-darwin-arm64'],
      [{ os: 'linux', arch: 'amd64' }, 'pinchtab-linux-amd64'],
      [{ os: 'linux', arch: 'arm64' }, 'pinchtab-linux-arm64'],
      [{ os: 'windows', arch: 'amd64' }, 'pinchtab-windows-amd64.exe'],
      [{ os: 'windows', arch: 'arm64' }, 'pinchtab-windows-arm64.exe'],
    ];

    cases.forEach(([platform, expected]) => {
      test(`${platform.os}-${platform.arch} → ${expected}`, () => {
        assert.strictEqual(getBinaryName(platform), expected);
      });
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
