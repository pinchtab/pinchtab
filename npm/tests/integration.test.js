'use strict';
const __createBinding =
  (this && this.__createBinding) ||
  (Object.create
    ? function (o, m, k, k2) {
        if (k2 === undefined) k2 = k;
        let desc = Object.getOwnPropertyDescriptor(m, k);
        if (!desc || ('get' in desc ? !m.__esModule : desc.writable || desc.configurable)) {
          desc = {
            enumerable: true,
            get: function () {
              return m[k];
            },
          };
        }
        Object.defineProperty(o, k2, desc);
      }
    : function (o, m, k, k2) {
        if (k2 === undefined) k2 = k;
        o[k2] = m[k];
      });
const __setModuleDefault =
  (this && this.__setModuleDefault) ||
  (Object.create
    ? function (o, v) {
        Object.defineProperty(o, 'default', { enumerable: true, value: v });
      }
    : function (o, v) {
        o['default'] = v;
      });
const __importStar =
  (this && this.__importStar) ||
  (function () {
    let ownKeys = function (o) {
      ownKeys =
        Object.getOwnPropertyNames ||
        function (o) {
          const ar = [];
          for (const k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
          return ar;
        };
      return ownKeys(o);
    };
    return function (mod) {
      if (mod && mod.__esModule) return mod;
      const result = {};
      if (mod != null)
        for (let k = ownKeys(mod), i = 0; i < k.length; i++)
          if (k[i] !== 'default') __createBinding(result, mod, k[i]);
      __setModuleDefault(result, mod);
      return result;
    };
  })();
const __importDefault =
  (this && this.__importDefault) ||
  function (mod) {
    return mod && mod.__esModule ? mod : { default: mod };
  };
Object.defineProperty(exports, '__esModule', { value: true });
const node_test_1 = require('node:test');
const assert = __importStar(require('node:assert'));
const index_1 = __importDefault(require('../src/index'));
const fs = __importStar(require('fs'));
const path = __importStar(require('path'));
const os = __importStar(require('os'));
(0, node_test_1.describe)('Pinchtab npm Integration Tests', () => {
  let pinch;
  const testPort = 9867;
  (0, node_test_1.before)(async () => {
    // Check if binary exists before running tests
    const binDir = path.join(os.homedir(), '.pinchtab', 'bin');
    const platform =
      process.platform === 'darwin' ? 'darwin' : process.platform === 'linux' ? 'linux' : 'windows';
    const arch = process.arch === 'arm64' ? 'arm64' : 'x64';
    const ext = platform === 'windows' ? '.exe' : '';
    const binaryPath = path.join(binDir, `pinchtab-${platform}-${arch}${ext}`);
    if (!fs.existsSync(binaryPath)) {
      console.warn(`⚠ Binary not found at ${binaryPath}`);
      console.warn('Tests will skip binary execution. Build the Go binary and place it at:');
      console.warn(`  ${binaryPath}`);
    }
    pinch = new index_1.default({ port: testPort });
  });
  (0, node_test_1.after)(async () => {
    // Clean up: stop server if running
    try {
      await pinch.stop();
    } catch (_e) {
      // Ignore
    }
  });
  (0, node_test_1.test)('should import Pinchtab class', () => {
    assert.ok(typeof index_1.default === 'function');
    assert.ok(pinch instanceof index_1.default);
  });
  (0, node_test_1.test)('should initialize with default options', () => {
    const client = new index_1.default();
    assert.ok(client);
  });
  (0, node_test_1.test)('should initialize with custom port', () => {
    const client = new index_1.default({ port: 9999 });
    assert.ok(client);
  });
  (0, node_test_1.test)('should have API methods defined', () => {
    assert.strictEqual(typeof pinch.start, 'function');
    assert.strictEqual(typeof pinch.stop, 'function');
    assert.strictEqual(typeof pinch.snapshot, 'function');
    assert.strictEqual(typeof pinch.click, 'function');
    assert.strictEqual(typeof pinch.lock, 'function');
    assert.strictEqual(typeof pinch.unlock, 'function');
    assert.strictEqual(typeof pinch.createTab, 'function');
  });
  (0, node_test_1.test)('should start server (requires binary)', async () => {
    const client = new index_1.default({ port: testPort });
    try {
      await client.start();
      // Give server a moment to be ready
      await new Promise((r) => setTimeout(r, 1000));
      // Try a simple health check
      const response = await fetch(`http://localhost:${testPort}/`);
      assert.ok(response.status !== undefined);
      await client.stop();
    } catch (err) {
      const errorMsg = err.message;
      if (errorMsg.includes('ENOENT') || errorMsg.includes('not found')) {
        console.log('⊘ Binary not available — skipping start test');
      } else {
        throw err;
      }
    }
  });
  (0, node_test_1.test)('should handle missing binary gracefully', async () => {
    const client = new index_1.default({ port: 9998 });
    try {
      await client.start('/nonexistent/path/to/binary');
      // If we get here, the binary exists (unusual test environment)
    } catch (err) {
      assert.ok(err instanceof Error);
      assert.ok(err.message.includes('Failed to start') || err.message.includes('ENOENT'));
    }
  });
  (0, node_test_1.test)('should reject invalid request to non-running server', async () => {
    const client = new index_1.default({ port: 9997 });
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
