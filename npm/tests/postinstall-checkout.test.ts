/**
 * Postinstall orchestration tests (GAP 2)
 *
 * `runPostinstall` (src/postinstall.ts) is the single npm-install entrypoint. It
 * is exported, but it cannot be driven directly in a unit test without heavy
 * mocking: every terminal branch calls `process.exit()` (which would tear down
 * the test runner), and the non-checkout branch calls `ensureBinary()` which
 * downloads from GitHub over the real network. Those branches are therefore
 * reported as untestable here without production changes / a process harness.
 *
 * What IS deterministically testable without network or process.exit is the FIRST
 * branch of runPostinstall — the source-checkout detection path:
 *
 *     const checkoutBinaryPath = getCheckoutBinaryPath(__dirname);
 *     if (checkoutBinaryPath) {
 *       if (!fs.existsSync(checkoutBinaryPath)) throw "...Build it first...";
 *       console.log("Using source-checkout ...");
 *     }
 *
 * These tests pin down `getCheckoutBinaryPath` (the branch selector, exported
 * from src/platform.ts) plus the existsSync precondition that runPostinstall
 * uses to decide between the happy path and the "build it first" error, driven
 * against synthetic temp checkouts. This is the same isolation strategy used by
 * wrapper-checkout-binary.test.ts.
 */

import { describe, test, afterEach } from 'node:test';
import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import { getCheckoutBinaryPath, findRepoRoot } from '../src/platform';

const tempRoots: string[] = [];

// Builds a synthetic source-checkout layout: <root>/go.mod, <root>/cmd/pinchtab,
// and a nested dir from which postinstall would resolve (mimics dist/src).
function createFakeCheckout(): { root: string; fromDir: string } {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-postinstall-test-'));
  tempRoots.push(root);
  fs.mkdirSync(path.join(root, 'cmd', 'pinchtab'), { recursive: true });
  fs.writeFileSync(path.join(root, 'go.mod'), 'module test/pinchtab\n');
  const fromDir = path.join(root, 'npm', 'dist', 'src');
  fs.mkdirSync(fromDir, { recursive: true });
  return { root, fromDir };
}

// A directory tree with no go.mod / cmd/pinchtab marker (a published-package
// install, not a source checkout).
function createNonCheckoutDir(): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-nocheckout-test-'));
  tempRoots.push(root);
  const fromDir = path.join(root, 'node_modules', 'pinchtab', 'dist', 'src');
  fs.mkdirSync(fromDir, { recursive: true });
  return fromDir;
}

afterEach(() => {
  while (tempRoots.length > 0) {
    fs.rmSync(tempRoots.pop()!, { recursive: true, force: true });
  }
});

describe('runPostinstall source-checkout detection (getCheckoutBinaryPath branch)', () => {
  test('detects a source checkout and points at <root>/pinchtab-dev', () => {
    const { root, fromDir } = createFakeCheckout();
    const checkoutBinaryPath = getCheckoutBinaryPath(fromDir);
    assert.strictEqual(
      checkoutBinaryPath,
      path.join(root, 'pinchtab-dev'),
      'in a source checkout, postinstall must select the local pinchtab-dev binary'
    );
  });

  test('returns null when not inside a source checkout (published-package install)', () => {
    const fromDir = createNonCheckoutDir();
    assert.strictEqual(
      getCheckoutBinaryPath(fromDir),
      null,
      'a published-package install must fall through to the download branch'
    );
  });

  test('"build it first" precondition: checkout detected but pinchtab-dev missing', () => {
    const { fromDir } = createFakeCheckout();
    const checkoutBinaryPath = getCheckoutBinaryPath(fromDir);
    assert.ok(checkoutBinaryPath, 'expected a checkout to be detected');
    // runPostinstall throws "...Build it first..." when this is false.
    assert.strictEqual(
      fs.existsSync(checkoutBinaryPath!),
      false,
      'with no built binary, runPostinstall must hit the "Build it first" error path'
    );
  });

  test('happy path: checkout detected and pinchtab-dev present passes the precondition', () => {
    const { root, fromDir } = createFakeCheckout();
    const checkoutBinaryPath = getCheckoutBinaryPath(fromDir);
    fs.writeFileSync(path.join(root, 'pinchtab-dev'), '#!/bin/sh\n', { mode: 0o755 });
    assert.ok(checkoutBinaryPath);
    assert.strictEqual(
      fs.existsSync(checkoutBinaryPath!),
      true,
      'with the binary built, runPostinstall takes the "Using source-checkout" happy path'
    );
  });

  test('findRepoRoot walks up from a deeply nested dist dir to the checkout root', () => {
    const { root, fromDir } = createFakeCheckout();
    assert.strictEqual(findRepoRoot(fromDir), root);
  });
});
