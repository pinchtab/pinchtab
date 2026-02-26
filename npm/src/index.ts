import { spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
import { detectPlatform, getBinaryName, getBinaryPath } from './platform';
import {
  SnapshotParams,
  SnapshotResponse,
  TabClickParams,
  TabLockParams,
  TabUnlockParams,
  CreateTabParams,
  CreateTabResponse,
  PinchtabOptions,
} from './types';

export * from './types';
export * from './platform';

export class Pinchtab {
  private baseUrl: string;
  private timeout: number;
  private port: number;
  private process: ChildProcess | null = null;
  private binaryPath: string | null = null;

  constructor(options: PinchtabOptions = {}) {
    this.port = options.port || 9867;
    this.baseUrl = options.baseUrl || `http://localhost:${this.port}`;
    this.timeout = options.timeout || 30000;
  }

  /**
   * Start the Pinchtab server process
   */
  async start(binaryPath?: string): Promise<void> {
    if (this.process) {
      throw new Error('Pinchtab process already running');
    }

    if (!binaryPath) {
      binaryPath = await this.getBinaryPathInternal();
    }

    this.binaryPath = binaryPath;

    return new Promise((resolve, reject) => {
      this.process = spawn(binaryPath, ['serve', `--port=${this.port}`], {
        stdio: 'inherit',
      });

      this.process.on('error', (err) => {
        reject(new Error(`Failed to start Pinchtab: ${err.message}`));
      });

      // Give the server a moment to start
      setTimeout(resolve, 500);
    });
  }

  /**
   * Stop the Pinchtab server process
   */
  async stop(): Promise<void> {
    if (this.process) {
      return new Promise((resolve) => {
        this.process?.kill();
        this.process = null;
        resolve();
      });
    }
  }

  /**
   * Take a snapshot of the current tab
   */
  async snapshot(params?: SnapshotParams): Promise<SnapshotResponse> {
    return this.request<SnapshotResponse>('/snapshot', params);
  }

  /**
   * Click on a UI element
   */
  async click(params: TabClickParams): Promise<void> {
    await this.request('/tab/click', params);
  }

  /**
   * Lock a tab
   */
  async lock(params: TabLockParams): Promise<void> {
    await this.request('/tab/lock', params);
  }

  /**
   * Unlock a tab
   */
  async unlock(params: TabUnlockParams): Promise<void> {
    await this.request('/tab/unlock', params);
  }

  /**
   * Create a new tab
   */
  async createTab(params: CreateTabParams): Promise<CreateTabResponse> {
    return this.request<CreateTabResponse>('/tab/create', params);
  }

  /**
   * Make a request to the Pinchtab API
   */
  private async request<T = Record<string, unknown>>(path: string, body?: Record<string, unknown>): Promise<T> {
    const url = `${this.baseUrl}${path}`;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal as AbortSignal,
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(`${response.status}: ${error}`);
      }

      return response.json() as Promise<T>;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Get the path to the Pinchtab binary
   */
  private async getBinaryPathInternal(): Promise<string> {
    const platform = detectPlatform();
    const binaryName = getBinaryName(platform);

    // Try version-specific path first
    const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf-8'));
    const binaryPath = getBinaryPath(binaryName, pkg.version);

    if (!fs.existsSync(binaryPath)) {
      throw new Error(
        `Pinchtab binary not found at ${binaryPath}.\n` +
          `Please run: npm rebuild pinchtab\n` +
          `Or set PINCHTAB_BINARY_PATH=/path/to/binary`
      );
    }

    return binaryPath;
  }
}

export default Pinchtab;
