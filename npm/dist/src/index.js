"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
var __exportStar = (this && this.__exportStar) || function(m, exports) {
    for (var p in m) if (p !== "default" && !Object.prototype.hasOwnProperty.call(exports, p)) __createBinding(exports, m, p);
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.Pinchtab = void 0;
const child_process_1 = require("child_process");
const path = __importStar(require("path"));
const fs = __importStar(require("fs"));
__exportStar(require("./types"), exports);
class Pinchtab {
    constructor(options = {}) {
        this.process = null;
        this.binaryPath = null;
        this.port = options.port || 9867;
        this.baseUrl = options.baseUrl || `http://localhost:${this.port}`;
        this.timeout = options.timeout || 30000;
    }
    /**
     * Start the Pinchtab server process
     */
    async start(binaryPath) {
        if (this.process) {
            throw new Error('Pinchtab process already running');
        }
        if (!binaryPath) {
            binaryPath = await this.getBinaryPath();
        }
        this.binaryPath = binaryPath;
        return new Promise((resolve, reject) => {
            this.process = (0, child_process_1.spawn)(binaryPath, ['serve', `--port=${this.port}`], {
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
    async stop() {
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
    async snapshot(params) {
        return this.request('/snapshot', params);
    }
    /**
     * Click on a UI element
     */
    async click(params) {
        await this.request('/tab/click', params);
    }
    /**
     * Lock a tab
     */
    async lock(params) {
        await this.request('/tab/lock', params);
    }
    /**
     * Unlock a tab
     */
    async unlock(params) {
        await this.request('/tab/unlock', params);
    }
    /**
     * Create a new tab
     */
    async createTab(params) {
        return this.request('/tab/create', params);
    }
    /**
     * Make a request to the Pinchtab API
     */
    async request(path, body) {
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
                signal: controller.signal,
            });
            if (!response.ok) {
                const error = await response.text();
                throw new Error(`${response.status}: ${error}`);
            }
            return response.json();
        }
        finally {
            clearTimeout(timeoutId);
        }
    }
    /**
     * Get the path to the Pinchtab binary
     */
    async getBinaryPath() {
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
        throw new Error(`Pinchtab binary not found. Please run: npm run postinstall or ensure the binary is in ~/.pinchtab/bin/`);
    }
    getBinaryName() {
        const platform = process.platform;
        const arch = process.arch === 'arm64' ? 'arm64' : 'x64';
        if (platform === 'darwin') {
            return `pinchtab-darwin-${arch}`;
        }
        else if (platform === 'linux') {
            return `pinchtab-linux-${arch}`;
        }
        else if (platform === 'win32') {
            return `pinchtab-windows-${arch}.exe`;
        }
        throw new Error(`Unsupported platform: ${platform}`);
    }
}
exports.Pinchtab = Pinchtab;
exports.default = Pinchtab;
