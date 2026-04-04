import { spawn, ChildProcess } from 'child_process';
import { randomBytes } from 'crypto';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';
import { detectPlatform, getBinaryName, getBinaryPath, getCheckoutBinaryPath } from './platform';
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
  private tempConfigDir: string | null = null;
  private token: string | null;
  private readonly configuredToken: string | null;

  constructor(options: PinchtabOptions = {}) {
    this.port = options.port || 9867;
    this.baseUrl = options.baseUrl || `http://localhost:${this.port}`;
    this.timeout = options.timeout || 30000;
    this.configuredToken = options.token?.trim() || null;
    this.token = this.configuredToken;
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
    const tempConfigPath = this.createTempConfig();

    return new Promise((resolve, reject) => {
      let settled = false;
      const fail = (message: string) => {
        if (settled) {
          return;
        }
        settled = true;
        this.cleanupTempConfig();
        reject(new Error(message));
      };

      this.process = spawn(binaryPath, ['server'], {
        stdio: 'inherit',
        env: {
          ...process.env,
          PINCHTAB_CONFIG: tempConfigPath,
        },
      });

      this.process.on('error', (err) => {
        fail(`Failed to start Pinchtab: ${err.message}`);
      });

      this.process.on('exit', (code, signal) => {
        this.cleanupTempConfig();
        if (!settled) {
          const reason = signal ? `signal ${signal}` : `exit code ${code ?? 0}`;
          reject(new Error(`Pinchtab exited before becoming ready (${reason})`));
        }
      });

      void this.waitForServerReady()
        .then(() => {
          if (settled) {
            return;
          }
          settled = true;
          resolve();
        })
        .catch((err: Error) => {
          if (this.process) {
            this.process.kill();
            this.process = null;
          }
          fail(err.message);
        });
    });
  }

  /**
   * Stop the Pinchtab server process
   */
  async stop(): Promise<void> {
    if (this.process) {
      return new Promise((resolve) => {
        const proc = this.process;
        this.process = null;
        if (!proc) {
          this.cleanupTempConfig();
          resolve();
          return;
        }
        proc.once('exit', () => {
          this.cleanupTempConfig();
          resolve();
        });
        proc.kill();
      });
    }
    this.cleanupTempConfig();
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
    await this.request('/lock', params);
  }

  /**
   * Unlock a tab
   */
  async unlock(params: TabUnlockParams): Promise<void> {
    await this.request('/unlock', params);
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
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private async request<T = any>(path: string, body?: any): Promise<T> {
    const url = `${this.baseUrl}${path}`;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      if (this.token) {
        headers.Authorization = `Bearer ${this.token}`;
      }

      const response = await fetch(url, {
        method: 'POST',
        headers,
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
    const checkoutBinaryPath = getCheckoutBinaryPath(__dirname);
    if (checkoutBinaryPath) {
      if (!fs.existsSync(checkoutBinaryPath)) {
        throw new Error(
          `Pinchtab source-checkout binary not found at ${checkoutBinaryPath}.\n` +
            `Build it first with: bash scripts/npm-dev-binary.sh`
        );
      }
      return checkoutBinaryPath;
    }

    const platform = detectPlatform();
    const binaryName = getBinaryName(platform);

    // Try version-specific path first
    let version: string | undefined;
    try {
      version = this.readPackageVersion();
    } catch (err) {
      console.warn(
        `Could not read version from package.json, falling back to unversioned binary. (${(err as Error).message})`
      );
    }

    const binaryPath = getBinaryPath(binaryName, version);
    if (!fs.existsSync(binaryPath)) {
      throw new Error(
        `Pinchtab binary not found at ${binaryPath}.\n` +
          `Please run: npm rebuild pinchtab\n` +
          `Or pass an explicit path to pinch.start('/path/to/pinchtab')`
      );
    }

    return binaryPath;
  }

  private createTempConfig(): string {
    this.cleanupTempConfig();

    const configDir = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-npm-'));
    const configPath = path.join(configDir, 'config.json');
    const stateDir = path.join(configDir, 'state');
    const token = this.configuredToken || `npm-${randomBytes(16).toString('hex')}`;
    this.token = token;

    fs.writeFileSync(
      configPath,
      JSON.stringify(
        {
          server: {
            bind: '127.0.0.1',
            port: String(this.port),
            stateDir,
            token,
          },
        },
        null,
        2
      )
    );

    this.tempConfigDir = configDir;
    return configPath;
  }

  private cleanupTempConfig(): void {
    if (!this.tempConfigDir) {
      return;
    }
    fs.rmSync(this.tempConfigDir, { recursive: true, force: true });
    this.tempConfigDir = null;
  }

  private async waitForServerReady(): Promise<void> {
    const deadline = Date.now() + 10000;

    while (Date.now() < deadline) {
      try {
        const response = await fetch(this.baseUrl);
        if (response.status > 0) {
          return;
        }
      } catch {
        // Server not ready yet.
      }

      await new Promise((resolve) => setTimeout(resolve, 250));
    }

    throw new Error(`Pinchtab did not become ready at ${this.baseUrl} within 10s`);
  }

  private readPackageVersion(): string {
    let dir = path.resolve(__dirname);

    while (dir) {
      const pkgPath = path.join(dir, 'package.json');
      if (fs.existsSync(pkgPath)) {
        const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
        if (typeof pkg.version === 'string' && pkg.version.trim() !== '') {
          return pkg.version;
        }
      }

      const parent = path.dirname(dir);
      if (parent === dir) {
        break;
      }
      dir = parent;
    }

    throw new Error(`ENOENT: package.json not found above ${__dirname}`);
  }
}

export default Pinchtab;
