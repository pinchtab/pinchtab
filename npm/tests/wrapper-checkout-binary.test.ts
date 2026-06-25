import { afterEach, describe, test } from 'node:test';
import * as assert from 'node:assert';
import { spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import { findRepoRoot } from '../src/platform';

function getTestRepoRoot(): string {
  const root = findRepoRoot(__dirname);
  if (!root) {
    throw new Error(`Could not find repo root from ${__dirname}`);
  }
  return root;
}

const repoRoot = getTestRepoRoot();
const sourceWrapperPath = path.join(repoRoot, 'npm', 'bin', 'pinchtab');
// The wrapper requires the compiled shared helpers (dist/src/platform.js), so a
// fake checkout must stage them too — published packages ship dist/ alongside bin/.
const compiledPlatformPath = path.join(repoRoot, 'npm', 'dist', 'src', 'platform.js');
const tempRoots: string[] = [];

function createFakeCheckout(stdoutText: string, recordPath: string) {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-wrapper-test-'));
  tempRoots.push(tempRoot);

  fs.mkdirSync(path.join(tempRoot, 'cmd', 'pinchtab'), { recursive: true });
  fs.mkdirSync(path.join(tempRoot, 'npm', 'bin'), { recursive: true });
  fs.mkdirSync(path.join(tempRoot, 'npm', 'dist', 'src'), { recursive: true });
  fs.writeFileSync(path.join(tempRoot, 'go.mod'), 'module test/pinchtab\n');
  fs.copyFileSync(sourceWrapperPath, path.join(tempRoot, 'npm', 'bin', 'pinchtab'));
  fs.copyFileSync(compiledPlatformPath, path.join(tempRoot, 'npm', 'dist', 'src', 'platform.js'));

  const contents = `#!/usr/bin/env node
const fs = require('fs');
fs.writeFileSync(${JSON.stringify(recordPath)}, JSON.stringify({
  argv: process.argv.slice(2),
}, null, 2));
process.stdout.write(${JSON.stringify(stdoutText)});
`;
  fs.writeFileSync(path.join(tempRoot, 'pinchtab-dev'), contents, { mode: 0o755 });
  fs.chmodSync(path.join(tempRoot, 'npm', 'bin', 'pinchtab'), 0o755);

  return {
    repoRoot: tempRoot,
    wrapperPath: path.join(tempRoot, 'npm', 'bin', 'pinchtab'),
  };
}

afterEach(() => {
  while (tempRoots.length > 0) {
    fs.rmSync(tempRoots.pop()!, { recursive: true, force: true });
  }
});

describe('wrapper source-checkout binary path', () => {
  test('uses pinchtab-dev for standard subcommands', () => {
    const recordPath = path.join(repoRoot, 'npm', 'wrapper-standard.json');
    const checkout = createFakeCheckout('standard-ok\n', recordPath);

    const result = spawnSync('node', [checkout.wrapperPath, '--version'], {
      cwd: checkout.repoRoot,
      encoding: 'utf-8',
    });

    assert.strictEqual(result.status, 0, result.stderr);
    assert.match(result.stdout, /standard-ok/);

    const recorded = JSON.parse(fs.readFileSync(recordPath, 'utf-8'));
    fs.rmSync(recordPath, { force: true });
    assert.deepStrictEqual(recorded.argv, ['--version']);
  });

  test('uses pinchtab-dev for the mcp subcommand path', () => {
    const recordPath = path.join(repoRoot, 'npm', 'wrapper-mcp.json');
    const checkout = createFakeCheckout('mcp-ok\n', recordPath);

    const result = spawnSync('node', [checkout.wrapperPath, 'mcp'], {
      cwd: checkout.repoRoot,
      encoding: 'utf-8',
    });

    assert.strictEqual(result.status, 0, result.stderr);
    assert.match(result.stdout, /mcp-ok/);

    const recorded = JSON.parse(fs.readFileSync(recordPath, 'utf-8'));
    fs.rmSync(recordPath, { force: true });
    assert.deepStrictEqual(recorded.argv, ['mcp']);
  });
});
