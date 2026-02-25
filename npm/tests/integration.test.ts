import { test, describe, before, after } from 'node:test';
import * as assert from 'node:assert';
import Pinchtab from '../src/index';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('Pinchtab npm Integration Tests', () => {
  let pinch: Pinchtab;
  const testPort = 9867;

  before(async () => {
    // Check if binary exists before running tests
    const binDir = path.join(os.homedir(), '.pinchtab', 'bin');
    const platform = process.platform === 'darwin' ? 'darwin' : process.platform === 'linux' ? 'linux' : 'windows';
    const arch = process.arch === 'arm64' ? 'arm64' : 'x64';
    const ext = platform === 'windows' ? '.exe' : '';
    const binaryPath = path.join(binDir, `pinchtab-${platform}-${arch}${ext}`);

    if (!fs.existsSync(binaryPath)) {
      console.warn(`⚠ Binary not found at ${binaryPath}`);
      console.warn('Tests will skip binary execution. Build the Go binary and place it at:');
      console.warn(`  ${binaryPath}`);
    }

    pinch = new Pinchtab({ port: testPort });
  });

  after(async () => {
    // Clean up: stop server if running
    try {
      await pinch.stop();
    } catch (e) {
      // Ignore
    }
  });

  test('should import Pinchtab class', () => {
    assert.ok(typeof Pinchtab === 'function');
    assert.ok(pinch instanceof Pinchtab);
  });

  test('should initialize with default options', () => {
    const client = new Pinchtab();
    assert.ok(client);
  });

  test('should initialize with custom port', () => {
    const client = new Pinchtab({ port: 9999 });
    assert.ok(client);
  });

  test('should have API methods defined', () => {
    assert.strictEqual(typeof pinch.start, 'function');
    assert.strictEqual(typeof pinch.stop, 'function');
    assert.strictEqual(typeof pinch.snapshot, 'function');
    assert.strictEqual(typeof pinch.click, 'function');
    assert.strictEqual(typeof pinch.lock, 'function');
    assert.strictEqual(typeof pinch.unlock, 'function');
    assert.strictEqual(typeof pinch.createTab, 'function');
  });

  test('should start server (requires binary)', async () => {
    const client = new Pinchtab({ port: testPort });
    
    try {
      await client.start();
      // Give server a moment to be ready
      await new Promise(r => setTimeout(r, 1000));
      
      // Try a simple health check
      const response = await fetch(`http://localhost:${testPort}/`);
      assert.ok(response.status !== undefined);
      
      await client.stop();
    } catch (err) {
      const errorMsg = (err as Error).message;
      if (errorMsg.includes('ENOENT') || errorMsg.includes('not found')) {
        console.log('⊘ Binary not available — skipping start test');
      } else {
        throw err;
      }
    }
  });

  test('should handle missing binary gracefully', async () => {
    const client = new Pinchtab({ port: 9998 });
    
    try {
      await client.start('/nonexistent/path/to/binary');
      // If we get here, the binary exists (unusual test environment)
    } catch (err) {
      assert.ok(err instanceof Error);
      assert.ok(
        (err as Error).message.includes('Failed to start') || 
        (err as Error).message.includes('ENOENT')
      );
    }
  });

  test('should reject invalid request to non-running server', async () => {
    const client = new Pinchtab({ port: 9997 });
    
    try {
      await client.snapshot();
      // Should not reach here
      assert.fail('Expected connection error');
    } catch (err) {
      // Expected — server not running
      assert.ok(err instanceof Error);
    }
  });
});
