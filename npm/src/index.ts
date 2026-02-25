import { spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
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
      binaryPath = await this.getBinaryPath();
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
  private async request<T = any>(path: string, body?: any): Promise<T> {
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
        signal: controller.signal as any,
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
  private async getBinaryPath(): Promise<string> {
    const homedir = require('os').homedir();
    const defaultPath = path.join(homedir, '.pinchtab', 'bin', this.getBinaryName());

    if (fs.existsSync(defaultPath)) {
      return defaultPath;
    }

    // Try relative to package
    const relativePath = path.join(__dirname, '..', 'bin', this.getBinaryName());
    if (fs.existsSync(relativePath)) {
      return relativePath;
    }

    throw new Error(
      `Pinchtab binary not found. Please run: npm run postinstall or ensure the binary is in ~/.pinchtab/bin/`
    );
  }

  private getBinaryName(): string {
    const platform = process.platform;
    const arch = process.arch === 'arm64' ? 'arm64' : 'x64';

    if (platform === 'darwin') {
      return `pinchtab-darwin-${arch}`;
    } else if (platform === 'linux') {
      return `pinchtab-linux-${arch}`;
    } else if (platform === 'win32') {
      return `pinchtab-windows-${arch}.exe`;
    }

    throw new Error(`Unsupported platform: ${platform}`);
  }
}

export default Pinchtab;
