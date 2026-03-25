/**
 * MCP Wrapper Tests
 *
 * Verifies that the Node.js wrapper correctly detects the mcp subcommand
 * and that the spawnSync path produces valid JSON-RPC responses.
 */

import { test, describe } from 'node:test';
import * as assert from 'node:assert';
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';
import * as fs from 'node:fs';

/**
 * Extracted firstSubcommand logic from bin/pinchtab for isolated testing.
 */
function firstSubcommand(argv: string[]): string | null {
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--server') {
      i += 1;
      continue;
    }
    if (arg.startsWith('--server=')) continue;
    if (!arg.startsWith('-')) return arg;
  }
  return null;
}

describe('firstSubcommand', () => {
  test('simple mcp', () => {
    assert.strictEqual(firstSubcommand(['mcp']), 'mcp');
  });

  test('mcp with flags before', () => {
    assert.strictEqual(firstSubcommand(['--server', 'http://localhost:9867', 'mcp']), 'mcp');
  });

  test('mcp with --server= syntax', () => {
    assert.strictEqual(firstSubcommand(['--server=http://localhost:9867', 'mcp']), 'mcp');
  });

  test('non-mcp subcommand', () => {
    assert.strictEqual(firstSubcommand(['nav', 'https://example.com']), 'nav');
  });

  test('only flags returns null', () => {
    assert.strictEqual(firstSubcommand(['--server', 'http://localhost:9867']), null);
  });

  test('empty args returns null', () => {
    assert.strictEqual(firstSubcommand([]), null);
  });
});

describe('MCP wrapper integration', () => {
  function findBinary(): string | null {
    // Prefer explicit override (CI or local dev with `go build -o`)
    if (process.env.PINCHTAB_BINARY_PATH) {
      return process.env.PINCHTAB_BINARY_PATH;
    }
    // Dev build in repo root (go build -o pinchtab-dev ./cmd/pinchtab)
    const devBinary = path.join(__dirname, '..', '..', 'pinchtab-dev');
    if (fs.existsSync(devBinary)) return devBinary;
    return null;
  }

  const binaryPath = findBinary();
  // __dirname is dist/tests/ after tsc, wrapper lives at bin/ (sibling to dist/)
  const wrapperPath = path.join(__dirname, '..', '..', 'bin', 'pinchtab');

  test('wrapper responds to JSON-RPC initialize via stdin', { skip: !binaryPath }, () => {
    const initMsg = JSON.stringify({
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: {},
        clientInfo: { name: 'test', version: '1.0' },
      },
    });

    const result = spawnSync('node', [wrapperPath, 'mcp'], {
      input: initMsg + '\n',
      timeout: 10000,
      env: {
        ...process.env,
        PINCHTAB_TOKEN: 'test',
        PINCHTAB_BINARY_PATH: binaryPath!,
      },
    });

    const stdout = result.stdout?.toString().trim();
    assert.ok(
      stdout,
      `expected JSON-RPC response on stdout, got nothing (stderr: ${result.stderr?.toString()})`
    );

    const response = JSON.parse(stdout.split('\n')[0]);
    assert.strictEqual(response.jsonrpc, '2.0');
    assert.strictEqual(response.id, 1);
    assert.ok(response.result, 'expected result in response');
    assert.strictEqual(response.result.serverInfo.name, 'PinchTab');
    assert.ok(response.result.capabilities.tools !== undefined, 'expected tools capability');
  });

  test('wrapper with --server flag still detects mcp subcommand', { skip: !binaryPath }, () => {
    const initMsg = JSON.stringify({
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: {},
        clientInfo: { name: 'test', version: '1.0' },
      },
    });

    const result = spawnSync('node', [wrapperPath, '--server', 'http://localhost:9867', 'mcp'], {
      input: initMsg + '\n',
      timeout: 10000,
      env: {
        ...process.env,
        PINCHTAB_TOKEN: 'test',
        PINCHTAB_BINARY_PATH: binaryPath!,
      },
    });

    const stdout = result.stdout?.toString().trim();
    assert.ok(stdout, 'expected JSON-RPC response with --server flag');

    const response = JSON.parse(stdout.split('\n')[0]);
    assert.strictEqual(response.id, 1);
    assert.ok(response.result?.serverInfo, 'expected serverInfo in response');
  });
});
